package monitors

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// allowPrivateNetworkEnv is the admin-only override permitting checks
// against private network addresses (lab use). It is read on every call
// so tests can toggle it with t.Setenv.
const allowPrivateNetworkEnv = "MONITORS_ALLOW_PRIVATE_NETWORK"

// privateCIDRs lists the address ranges blocked by default.
var privateCIDRs = []string{
	"127.0.0.0/8",    // loopback
	"10.0.0.0/8",     // RFC 1918
	"172.16.0.0/12",  // RFC 1918
	"192.168.0.0/16", // RFC 1918
	"169.254.0.0/16", // link-local / cloud metadata
	"::1/128",        // loopback
	"fe80::/10",      // link-local
	"fc00::/7",       // unique-local
	"0.0.0.0/32",     // unspecified
	"::/128",         // unspecified
}

var privateNets []*net.IPNet

func init() {
	for _, cidr := range privateCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("monitors: invalid private CIDR %q: %v", cidr, err))
		}
		privateNets = append(privateNets, n)
	}
}

// IsPrivateIP reports whether ip falls in a range blocked by default.
// The classifier is pure: the env override only relaxes GuardDialContext.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return true
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ParsePortOrDefault splits host[:port], defaulting the port when absent.
func ParsePortOrDefault(hostport string, def int) (string, int, error) {
	if def <= 0 || def > 65535 {
		return "", 0, fmt.Errorf("invalid default port %d", def)
	}
	if hostport == "" {
		return "", 0, fmt.Errorf("empty address")
	}
	if host, portStr, err := net.SplitHostPort(hostport); err == nil {
		if host == "" {
			return "", 0, fmt.Errorf("empty host in %q", hostport)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return "", 0, fmt.Errorf("invalid port in %q", hostport)
		}
		return strings.Trim(host, "[]"), port, nil
	}
	// No explicit port: hostport must be a bare host, not malformed input.
	trimmed := strings.Trim(hostport, "[]")
	if strings.HasPrefix(hostport, "[") || strings.ContainsAny(trimmed, " /") {
		return "", 0, fmt.Errorf("invalid address %q", hostport)
	}
	if ip := net.ParseIP(trimmed); ip != nil && strings.Contains(trimmed, ":") {
		// Bare IPv6 literal without brackets and without port.
		return trimmed, def, nil
	}
	if strings.Count(trimmed, ":") > 1 {
		return "", 0, fmt.Errorf("invalid address %q", hostport)
	}
	return trimmed, def, nil
}

// GuardDialContext dials network/address after verifying every resolved IP
// against the private-network blocklist. It pins the first allowed IP for
// the attempt, mitigating DNS-rebinding races within a single check.
func GuardDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := ParsePortOrDefault(address, 0)
	if err != nil || port == 0 {
		// ParsePortOrDefault needs a default; re-split strictly requiring a port.
		h, pStr, splitErr := net.SplitHostPort(address)
		if splitErr != nil || h == "" {
			return nil, fmt.Errorf("invalid address %q", address)
		}
		p, convErr := strconv.Atoi(pStr)
		if convErr != nil || p <= 0 || p > 65535 {
			return nil, fmt.Errorf("invalid port in %q", address)
		}
		host, port = h, p
	}
	_ = host

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", stripBrackets(host))
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("cannot resolve %q: %v", host, err)
	}
	allowPrivate := os.Getenv(allowPrivateNetworkEnv) == "true"
	var first *net.IP
	for _, ip := range ips {
		if !allowPrivate && IsPrivateIP(ip) {
			continue
		}
		ipCopy := ip
		first = &ipCopy
		break
	}
	if first == nil {
		return nil, fmt.Errorf("all resolved addresses for %q are private network (set %s=true to allow)", host, allowPrivateNetworkEnv)
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(first.String(), strconv.Itoa(port)))
}

func stripBrackets(h string) string {
	return strings.Trim(h, "[]")
}
