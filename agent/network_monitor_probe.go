package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/entities/monitor"
)

const networkMonitorUserAgent = "Beszel-Agent/" + beszel.Version + " (+https://beszel.dev)"

// maxMonitorBodyBytes caps how much of an HTTP response body is read for content checks.
const maxMonitorBodyBytes = 1 << 20

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
			return monitorDNS(ctx, config)
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

// monitorDNS measures DNS resolution response time in microseconds. If
// config.Server is non-empty, the lookup is sent to that DNS server (host or
// host:port, default port 53) instead of the system resolver. config.RecordType
// selects the record type to query, and a non-empty config.Keyword must match
// one of the returned records. Returns -1 and an error on failure.
func monitorDNS(ctx context.Context, config monitor.Config) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resolver := net.DefaultResolver
	if config.Server != "" {
		resolver = dnsResolverForServer(config.Server)
	}

	start := time.Now()
	records, err := lookupDNSRecords(ctx, resolver, config.Target, config.RecordType)
	if err != nil {
		return -1, err
	}
	if len(records) == 0 {
		return -1, errors.New("no DNS records found")
	}
	responseUs := time.Since(start).Microseconds()
	if config.Keyword != "" && !dnsRecordsMatch(records, config.RecordType, config.Keyword) {
		return -1, fmt.Errorf("DNS records %q do not match %q", truncateMonitorValue(strings.Join(records, ", ")), config.Keyword)
	}
	return responseUs, nil
}

// lookupDNSRecords returns the target's records of the given type as strings.
// An empty record type resolves the host's A and AAAA records.
func lookupDNSRecords(ctx context.Context, resolver *net.Resolver, target, recordType string) ([]string, error) {
	switch recordType {
	case "":
		return resolver.LookupHost(ctx, target)
	case "A", "AAAA":
		network := "ip4"
		if recordType == "AAAA" {
			network = "ip6"
		}
		ips, err := resolver.LookupIP(ctx, network, target)
		records := make([]string, 0, len(ips))
		for _, ip := range ips {
			records = append(records, ip.String())
		}
		return records, err
	case "CNAME":
		cname, err := resolver.LookupCNAME(ctx, target)
		if err != nil {
			return nil, err
		}
		// LookupCNAME returns the queried name itself when it has no CNAME record.
		if normalizeDNSValue(cname) == normalizeDNSValue(target) {
			return nil, errors.New("no CNAME record found")
		}
		return []string{cname}, nil
	case "MX":
		// Lookups can return valid records together with an error for malformed ones.
		mxs, err := resolver.LookupMX(ctx, target)
		records := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			records = append(records, mx.Host)
		}
		return records, ignoreErrorWithRecords(records, err)
	case "NS":
		nss, err := resolver.LookupNS(ctx, target)
		records := make([]string, 0, len(nss))
		for _, ns := range nss {
			records = append(records, ns.Host)
		}
		return records, ignoreErrorWithRecords(records, err)
	case "TXT":
		return resolver.LookupTXT(ctx, target)
	default:
		return nil, fmt.Errorf("unknown DNS record type: %s", recordType)
	}
}

func ignoreErrorWithRecords(records []string, err error) error {
	if len(records) > 0 {
		return nil
	}
	return err
}

// dnsRecordsMatch reports whether any record matches keyword. TXT records only
// need to contain it; other records must equal it, ignoring case, a trailing
// dot, and IP address formatting.
func dnsRecordsMatch(records []string, recordType, keyword string) bool {
	expected := normalizeDNSValue(keyword)
	for _, record := range records {
		if recordType == "TXT" {
			if strings.Contains(record, keyword) {
				return true
			}
			continue
		}
		if normalizeDNSValue(record) == expected {
			return true
		}
	}
	return false
}

// normalizeDNSValue returns a canonical form of an IP address or host name for comparison.
func normalizeDNSValue(value string) string {
	value = strings.TrimSpace(value)
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.Unmap().String()
	}
	return strings.ToLower(strings.TrimSuffix(value, "."))
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

// monitorHTTP measures HTTP GET request response in microseconds. A non-empty
// config.Keyword or config.JSONPath also checks the response body; see
// checkHTTPBody. Returns -1 and an error on failure.
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
	if resp.StatusCode >= 400 {
		return -1, fmt.Errorf("HTTP error: %s", resp.Status)
	}
	// Reading the body for a content check is not part of the response time.
	responseUs := time.Since(start).Microseconds()
	if config.Keyword == "" && config.JSONPath == "" {
		return responseUs, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMonitorBodyBytes))
	if err != nil {
		return -1, err
	}
	if err := checkHTTPBody(body, config.JSONPath, config.Keyword); err != nil {
		return -1, err
	}
	return responseUs, nil
}

// checkHTTPBody verifies the response body. Without a JSON path the body must
// contain keyword. With one, the body must be JSON holding a single value at
// that path, which must equal keyword unless keyword is empty.
func checkHTTPBody(body []byte, jsonPath, keyword string) error {
	if jsonPath == "" {
		if !bytes.Contains(body, []byte(keyword)) {
			return fmt.Errorf("keyword %q not found in response", keyword)
		}
		return nil
	}
	value, err := jsonPathValue(body, jsonPath)
	if err != nil {
		return err
	}
	if keyword != "" && value != keyword {
		return fmt.Errorf("JSON value at %q is %q, expected %q", jsonPath, truncateMonitorValue(value), keyword)
	}
	return nil
}

// truncateMonitorValue shortens response content so a failed check cannot flood the log.
func truncateMonitorValue(value string) string {
	const maxLen = 200
	if len(value) <= maxLen {
		return value
	}
	return strings.ToValidUTF8(value[:maxLen], "") + "..."
}

// jsonPathValue returns the string, number, boolean, or null at a dot-separated
// path in a JSON document, formatted as text. Numeric path segments index arrays.
func jsonPathValue(body []byte, path string) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var current any
	if err := decoder.Decode(&current); err != nil {
		return "", fmt.Errorf("response is not valid JSON: %w", err)
	}
	for key := range strings.SplitSeq(path, ".") {
		found := false
		switch node := current.(type) {
		case map[string]any:
			current, found = node[key]
		case []any:
			if i, err := strconv.Atoi(key); err == nil && i >= 0 && i < len(node) {
				current, found = node[i], true
			}
		}
		if !found {
			return "", fmt.Errorf("JSON path %q not found in response", path)
		}
	}
	switch value := current.(type) {
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	case bool:
		return strconv.FormatBool(value), nil
	case nil:
		return "null", nil
	default:
		return "", fmt.Errorf("JSON path %q is not a single value", path)
	}
}
