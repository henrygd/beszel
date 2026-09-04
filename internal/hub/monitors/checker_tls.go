package monitors

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Defaults and bounds for the TLS certificate checker (spec §6.2).
const (
	defaultTLSPort  = 443
	defaultWarnDays = 21
	defaultCritDays = 7
	maxDNSNames     = 5
	// emptyChainDays is returned by CertDaysLeft for an empty chain: a very
	// negative value so callers bucket it as down.
	emptyChainDays = -1e9
)

// tlsTestRoots is a test-only seam: when non-nil it replaces the system root
// pool for CheckTLS so tests can trust a throwaway CA without network access.
// Production code leaves it nil (system roots are used).
var tlsTestRoots *x509.CertPool

// CertDaysLeft returns the remaining lifetime of the leaf certificate
// (chain[0].NotAfter - now) in fractional days. It is pure and needs no
// network. An empty chain yields a very negative value which the caller
// treats as down.
func CertDaysLeft(chain []*x509.Certificate, now time.Time) float64 {
	if len(chain) == 0 || chain[0] == nil {
		return emptyChainDays
	}
	return chain[0].NotAfter.Sub(now).Hours() / 24
}

// CheckTLS performs a TLS certificate check from the hub: it opens a guarded
// TCP connection (SSRF blocklist enforced), completes a TLS handshake with
// full hostname verification, and evaluates certificate expiry against the
// warn_days / crit_days thresholds. It never applies upside_down: Task 7
// applies that inversion.
func CheckTLS(ctx context.Context, m Monitor) CheckResult {
	details := map[string]any{}
	down := func(msg string) CheckResult {
		return CheckResult{Status: StatusDown, Message: msg, Details: details}
	}

	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}

	host, port, err := parseTLSTarget(m.Target)
	if err != nil {
		return down(err.Error())
	}

	warnDays := configInt(m.Config["warn_days"], defaultWarnDays)
	critDays := configInt(m.Config["crit_days"], defaultCritDays)
	if critDays >= warnDays {
		return down(fmt.Sprintf("invalid thresholds: crit_days (%d) must be less than warn_days (%d)", critDays, warnDays))
	}

	serverName := strings.TrimSpace(configString(m.Config["server_name"], ""))
	verifyHost := host
	if serverName != "" {
		verifyHost = serverName
	}

	ignoreTLS := configBool(m.Config["ignore_tls_errors"], false)
	if ignoreTLS {
		details["tls_insecure"] = true
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// TCP must go through the SSRF guard: every resolved IP is vetted and
	// the admin-only env override is honored. GuardDialContext dials with a
	// 5 s timeout internally; the check context adds the global timeout.
	start := time.Now()
	rawConn, err := GuardDialContext(reqCtx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		// The private-network block surfaces here; the guard message names
		// it explicitly ("... are private network ...").
		msg := fmt.Sprintf("connection failed: %v", err)
		details["error"] = msg
		return CheckResult{Status: StatusDown, LatencyMs: elapsedMs(start), Message: msg, Details: details}
	}
	defer rawConn.Close()

	// tls.Client on the guarded connection + HandshakeContext honors both
	// the guard and the check timeout. (tls.DialWithDialer pins
	// context.Background internally, which would ignore m.TimeoutSeconds.)
	tlsConfig := &tls.Config{
		ServerName:         verifyHost,
		InsecureSkipVerify: ignoreTLS,
	}
	if tlsTestRoots != nil {
		tlsConfig.RootCAs = tlsTestRoots
	}
	tlsConn := tls.Client(rawConn, tlsConfig)
	if err := tlsConn.HandshakeContext(reqCtx); err != nil {
		msg := classifyTLSHandshakeError(err)
		details["error"] = err.Error()
		return CheckResult{Status: StatusDown, LatencyMs: elapsedMs(start), Message: msg, Details: details}
	}
	defer tlsConn.Close()

	chain := tlsConn.ConnectionState().PeerCertificates
	latency := elapsedMs(start)
	if len(chain) == 0 || chain[0] == nil {
		msg := "invalid certificate chain"
		details["error"] = "server presented no certificates"
		return CheckResult{Status: StatusDown, LatencyMs: latency, Message: msg, Details: details}
	}
	leaf := chain[0]
	now := time.Now()
	days := CertDaysLeft(chain, now)
	rounded := math.Round(days*100) / 100

	details["not_after"] = leaf.NotAfter.UTC().Format(time.RFC3339)
	issuer := leaf.Issuer.CommonName
	if issuer == "" {
		issuer = leaf.Issuer.String()
	}
	details["issuer"] = issuer
	names := leaf.DNSNames
	if len(names) > maxDNSNames {
		names = names[:maxDNSNames]
	}
	dnsNames := make([]string, len(names))
	copy(dnsNames, names)
	details["dns_names"] = dnsNames

	res := CheckResult{LatencyMs: latency, Details: details, CertDays: &rounded}
	switch {
	case !now.Before(leaf.NotAfter):
		res.Status = StatusDown
		res.Message = "certificate expired"
	case now.Before(leaf.NotBefore):
		res.Status = StatusDown
		res.Message = "invalid certificate chain"
		details["error"] = fmt.Sprintf("certificate is not valid before %s", leaf.NotBefore.UTC().Format(time.RFC3339))
	case days <= float64(critDays):
		res.Status = StatusDown
		res.Message = fmt.Sprintf("certificate expires in %.2f days", rounded)
	case days <= float64(warnDays):
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("certificate expires in %.2f days (warning)", rounded)
	default:
		res.Status = StatusUp
		res.Message = fmt.Sprintf("certificate expires in %.2f days", rounded)
	}
	return res
}

// parseTLSTarget accepts a URL (https://host[:port]/...), host[:port], or a
// bare host. The port defaults to 443 and must be 1-65535. An empty host is a
// validation failure, never a panic.
func parseTLSTarget(target string) (string, int, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", 0, fmt.Errorf("invalid target %q: target is required", target)
	}
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", 0, fmt.Errorf("invalid target %q: %v", target, err)
		}
		host := u.Hostname()
		if host == "" {
			return "", 0, fmt.Errorf("invalid target %q: missing host", target)
		}
		port := defaultTLSPort
		if u.Port() != "" {
			p, err := strconv.Atoi(u.Port())
			if err != nil || p <= 0 || p > 65535 {
				return "", 0, fmt.Errorf("invalid target %q: invalid port", target)
			}
			port = p
		}
		return stripBrackets(host), port, nil
	}
	host, port, err := ParsePortOrDefault(trimmed, defaultTLSPort)
	if err != nil {
		return "", 0, fmt.Errorf("invalid target %q: %v", target, err)
	}
	if host == "" {
		return "", 0, fmt.Errorf("invalid target %q: missing host", target)
	}
	return host, port, nil
}

// classifyTLSHandshakeError maps handshake failures to stable messages.
// Messages stay fixed for tests and dashboards; the raw error lands in
// Details["error"] for debugging.
func classifyTLSHandshakeError(err error) string {
	var verErr *tls.CertificateVerificationError
	if errors.As(err, &verErr) {
		return classifyTLSVerifyError(verErr.Err)
	}
	return classifyTLSVerifyError(err)
}

func classifyTLSVerifyError(err error) string {
	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return "hostname mismatch"
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		if invalid.Reason == x509.Expired {
			return "certificate expired"
		}
		return "invalid certificate chain"
	}
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return "invalid certificate chain"
	}
	return fmt.Sprintf("TLS handshake failed: %v", err)
}
