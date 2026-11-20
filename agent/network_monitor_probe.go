package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/entities/monitor"
)

const networkMonitorUserAgent = "Beszel-Agent/" + beszel.Version + " (+https://beszel.dev)"

// monitorProbe performs one check. Errors are recorded as loss by the task runner.
// Implementations must honor cancellation and bound their execution time.
type monitorProbe func(context.Context, monitor.Config) (int64, error)

// monitorHTTPClients returns the HTTP client for an IP version and TLS mode.
type monitorHTTPClients func(ipVersion uint8, skipTLSVerify bool) *http.Client

type monitorHTTPClientKey struct {
	ipVersion     uint8
	skipTLSVerify bool
}

// newMonitorHTTPClients builds one client per IP version and TLS mode. Each has its
// own transport, so an idle connection of one family is never reused by another.
func newMonitorHTTPClients() monitorHTTPClients {
	clients := make(map[monitorHTTPClientKey]*http.Client, 6)
	for _, ipVersion := range []uint8{monitor.IPVersionAuto, monitor.IPVersion4, monitor.IPVersion6} {
		for _, skipTLSVerify := range []bool{false, true} {
			clients[monitorHTTPClientKey{ipVersion, skipTLSVerify}] = newMonitorHTTPClient(ipVersion, skipTLSVerify)
		}
	}
	return func(ipVersion uint8, skipTLSVerify bool) *http.Client {
		return clients[monitorHTTPClientKey{ipVersion, skipTLSVerify}]
	}
}

func newMonitorHTTPClient(ipVersion uint8, skipTLSVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	network := monitor.Network("tcp", ipVersion)
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, _, address string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			host, _, _ := net.SplitHostPort(address)
			return nil, lookupError(err, ipVersion, host)
		}
		return conn, nil
	}
	return &http.Client{Timeout: monitor.MaxProbeTimeout, Transport: transport}
}

// networkMonitorProbe returns a probe that picks the HTTP client for each
// monitor's IP version and TLS mode from clients.
func networkMonitorProbe(clients monitorHTTPClients) monitorProbe {
	return func(ctx context.Context, config monitor.Config) (int64, error) {
		if !monitor.ValidIPVersion(config.IPVersion) {
			return -1, fmt.Errorf("unknown monitor IP version: %d", config.IPVersion)
		}
		switch config.Protocol {
		case "icmp":
			return monitorICMP(ctx, config.Target, config.IPVersion)
		case "tcp":
			return monitorTCP(ctx, config.Target, config.Port, config.IPVersion)
		case "http":
			return monitorHTTP(ctx, clients(config.IPVersion, config.SkipTLSVerify), config.Target)
		case "dns":
			return monitorDNS(ctx, config.Target, config.Server, config.IPVersion)
		default:
			return -1, fmt.Errorf("unknown monitor protocol: %s", config.Protocol)
		}
	}
}

// lookupMonitorIPs resolves host to addresses of the given IP version. It never
// returns an empty slice without an error.
func lookupMonitorIPs(ctx context.Context, resolver *net.Resolver, host string, ipVersion uint8) ([]net.IP, error) {
	ips, err := resolver.LookupIP(ctx, monitor.Network("ip", ipVersion), host)
	if err != nil {
		return nil, lookupError(err, ipVersion, host)
	}
	if len(ips) == 0 {
		return nil, noAddressError(ipVersion, host)
	}
	return ips, nil
}

// lookupError replaces Go's resolver errors, such as "lookup example.com on
// 127.0.0.53:53: no such host", with shorter messages naming the host. Go also
// reports "no such host" when only the other family's records exist.
func lookupError(err error, ipVersion uint8, host string) error {
	var dnsErr *net.DNSError
	var addrErr *net.AddrError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound, errors.As(err, &addrErr):
		return noAddressError(ipVersion, host)
	case dnsErr != nil && dnsErr.IsTimeout:
		return fmt.Errorf("DNS lookup for %s timed out", host)
	case dnsErr != nil:
		return fmt.Errorf("DNS lookup for %s failed: %s", host, dnsErr.Err)
	default:
		return err
	}
}

func noAddressError(ipVersion uint8, host string) error {
	if ipVersion == monitor.IPVersionAuto {
		return fmt.Errorf("no address found for %s", host)
	}
	return fmt.Errorf("no IPv%d address for %s", ipVersion, host)
}

// monitorTCP measures connection establishment time, including address fallback
// but excluding DNS resolution.
// Returns -1 and an error on failure.
func monitorTCP(ctx context.Context, target string, port uint16, ipVersion uint8) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Resolve DNS first, outside the timing window but within the probe deadline.
	ips, err := lookupMonitorIPs(ctx, net.DefaultResolver, target, ipVersion)
	if err != nil {
		return -1, err
	}
	network := monitor.Network("tcp", ipVersion)
	portString := fmt.Sprintf("%d", port)
	deadline, _ := ctx.Deadline()

	// Share the remaining probe budget across addresses so an unresponsive
	// first address cannot consume all the time available for alternatives.
	start := time.Now()
	for i, ip := range ips {
		if err := ctx.Err(); err != nil {
			return -1, err
		}
		dialer := net.Dialer{Timeout: time.Until(deadline) / time.Duration(len(ips)-i)}
		var conn net.Conn
		conn, err = dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), portString))
		if err != nil {
			continue
		}
		responseUs := time.Since(start).Microseconds()
		conn.Close()
		return responseUs, nil
	}
	return -1, err
}

// monitorDNS measures DNS resolution response time in microseconds. If server is
// non-empty, the lookup is sent to that DNS server (host or host:port, default
// port 53) instead of the system resolver. IPv4 and IPv6 query only the A or AAAA
// record. Returns -1 and an error on failure.
func monitorDNS(ctx context.Context, target, server string, ipVersion uint8) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resolver := net.DefaultResolver
	if server != "" {
		resolver = dnsResolverForServer(server)
	}

	start := time.Now()
	if _, err := lookupMonitorIPs(ctx, resolver, target, ipVersion); err != nil {
		return -1, err
	}
	return time.Since(start).Microseconds(), nil
}

// dnsResolverForServer builds a resolver that sends lookups to the given DNS
// server address instead of the system resolver. server may be a bare host or
// host:port; when no port is given, the standard DNS port 53 is used.
func dnsResolverForServer(server string) *net.Resolver {
	address := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		address = net.JoinHostPort(server, "53")
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, address)
		},
	}
}

// monitorHTTP measures HTTP GET request response in microseconds. Returns -1 and an error on failure.
func monitorHTTP(ctx context.Context, client *http.Client, url string) (int64, error) {
	if client == nil {
		client = http.DefaultClient
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return -1, err
	}
	req.Header.Set("User-Agent", networkMonitorUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return -1, fmt.Errorf("HTTP error: %s", resp.Status)
	}
	return time.Since(start).Microseconds(), nil
}
