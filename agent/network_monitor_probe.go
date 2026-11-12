package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/entities/monitor"
)

const networkMonitorUserAgent = "Beszel-Agent/" + beszel.Version + " (+https://beszel.dev)"

// monitorProbe performs one check. Errors are recorded as loss by the task runner.
// Implementations must honor cancellation and bound their execution time.
type monitorProbe func(context.Context, monitor.Config) (int64, error)

// networkMonitorProbe returns a probe that uses client for HTTP monitors, or
// insecureClient for HTTP monitors that skip TLS certificate verification.
func networkMonitorProbe(client, insecureClient *http.Client) monitorProbe {
	return func(ctx context.Context, config monitor.Config) (int64, error) {
		switch config.Protocol {
		case "icmp":
			return monitorICMP(ctx, config.Target)
		case "tcp":
			return monitorTCP(ctx, config.Target, config.Port)
		case "http":
			if config.SkipTLSVerify {
				return monitorHTTP(ctx, insecureClient, config)
			}
			return monitorHTTP(ctx, client, config)
		case "dns":
			return monitorDNS(ctx, config.Target, config.Server)
		default:
			return -1, fmt.Errorf("unknown monitor protocol: %s", config.Protocol)
		}
	}
}

// monitorTCP measures connection establishment time, including address fallback
// but excluding DNS resolution.
// Returns -1 and an error on failure.
func monitorTCP(ctx context.Context, target string, port uint16) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Resolve DNS first, outside the timing window but within the probe deadline.
	ips, err := net.DefaultResolver.LookupHost(ctx, target)
	if err != nil {
		return -1, err
	}
	if len(ips) == 0 {
		return -1, errors.New("no addresses resolved for TCP monitor")
	}
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
		conn, err = dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, portString))
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
// port 53) instead of the system resolver. Returns -1 and an error on failure.
func monitorDNS(ctx context.Context, target, server string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resolver := net.DefaultResolver
	if server != "" {
		resolver = dnsResolverForServer(server)
	}

	start := time.Now()
	ips, err := resolver.LookupHost(ctx, target)
	if err != nil || len(ips) == 0 {
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

// maxKeywordBodyBytes limits how much of the response body is searched for a keyword.
const maxKeywordBodyBytes = 1 << 20

// monitorHTTP measures HTTP GET request response in microseconds. Returns -1 and an error on failure.
// The response time covers the time until the response headers are received. Without an expected
// status, any status below 400 is accepted. If a keyword is set, the body must contain it (or must
// not contain it when InvertKeyword is set).
func monitorHTTP(ctx context.Context, client *http.Client, config monitor.Config) (int64, error) {
	if client == nil {
		client = http.DefaultClient
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.Target, nil)
	if err != nil {
		return -1, err
	}
	req.Header.Set("User-Agent", networkMonitorUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	responseUs := time.Since(start).Microseconds()
	if config.ExpectedStatus != "" {
		ok, err := statusMatches(config.ExpectedStatus, resp.StatusCode)
		if err != nil {
			return -1, err
		}
		if !ok {
			return -1, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
		}
	} else if resp.StatusCode >= 400 {
		return -1, fmt.Errorf("HTTP error: %s", resp.Status)
	}
	if config.Keyword != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxKeywordBodyBytes))
		if err != nil {
			return -1, err
		}
		found := strings.Contains(string(body), config.Keyword)
		if found == config.InvertKeyword {
			if config.InvertKeyword {
				return -1, fmt.Errorf("keyword %q found in response", config.Keyword)
			}
			return -1, fmt.Errorf("keyword %q not found in response", config.Keyword)
		}
	}
	return responseUs, nil
}

// statusMatches reports whether code is covered by a comma-separated list of
// status codes and inclusive ranges, e.g. "200-299,301".
func statusMatches(expected string, code int) (bool, error) {
	matched := false
	for part := range strings.SplitSeq(expected, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		low, err := strconv.Atoi(strings.TrimSpace(lo))
		if err != nil {
			return false, fmt.Errorf("invalid expected status: %q", part)
		}
		high := low
		if isRange {
			if high, err = strconv.Atoi(strings.TrimSpace(hi)); err != nil || high < low {
				return false, fmt.Errorf("invalid expected status: %q", part)
			}
		}
		if code >= low && code <= high {
			matched = true
		}
	}
	return matched, nil
}
