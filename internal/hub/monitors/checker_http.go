package monitors

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults and bounds for the HTTP/keyword checker (spec §6.1).
const (
	defaultAcceptedStatusCodes = "200-299"
	defaultMaxRedirects        = 10
	minMaxRedirects            = 1
	maxMaxRedirects            = 20
	maxHeaderEntries           = 20
	maxHeaderKeyLen            = 100
	maxRequestBodyBytes        = 1 << 20
	maxResponseBodyBytes       = 2 << 20
	defaultUserAgent           = "Beszel-Monitor"
	certSkippedNote            = "tls checker pending (task 4)"
	defaultCheckTimeout        = 10 * time.Second
	tlsHandshakeTimeout        = 5 * time.Second
)

// allowedHTTPMethods is the enum of supported request methods.
var allowedHTTPMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodPatch:   true,
	http.MethodDelete:  true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// CheckHTTP performs an HTTP (or keyword) uptime check from the hub.
// It never applies upside_down: Task 7 applies that inversion. Secrets from
// the config are never included in messages or details.
func CheckHTTP(ctx context.Context, m Monitor) CheckResult {
	details := map[string]any{}
	down := func(msg string) CheckResult {
		return CheckResult{Status: StatusDown, Message: msg, Details: details}
	}

	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}

	target, err := parseMonitorURL(m.Target)
	if err != nil {
		return down(err.Error())
	}

	method, err := parseHTTPMethod(m.Config["method"])
	if err != nil {
		return down(err.Error())
	}

	accept, err := parseAcceptedStatusCodes(configString(m.Config["accepted_status_codes"], defaultAcceptedStatusCodes))
	if err != nil {
		return down(err.Error())
	}

	followRedirects := configBool(m.Config["follow_redirects"], true)
	maxRedirects := configInt(m.Config["max_redirects"], defaultMaxRedirects)
	if maxRedirects < minMaxRedirects {
		maxRedirects = minMaxRedirects
	}
	if maxRedirects > maxMaxRedirects {
		maxRedirects = maxMaxRedirects
	}

	headers, hasUserAgent, err := parseHTTPHeaders(m.Config["headers"])
	if err != nil {
		return down(err.Error())
	}

	body := configString(m.Config["body"], "")
	if len(body) > maxRequestBodyBytes {
		return down(fmt.Sprintf("request body exceeds 1 MB limit (%d bytes)", len(body)))
	}
	contentType := configString(m.Config["content_type"], "")

	auth, err := parseHTTPAuth(m.Config)
	if err != nil {
		return down(err.Error())
	}

	ignoreTLS := configBool(m.Config["ignore_tls_errors"], false)
	if ignoreTLS {
		details["tls_insecure"] = true
	}
	if configBool(m.Config["check_cert_expiry"], false) {
		// check_cert_expiry is wired by Task 4 (CheckTLS). Note it without
		// failing the check.
		details["cert_skipped"] = certSkippedNote
	}

	keyword := configString(m.Config["keyword"], "")
	invertKeyword := configBool(m.Config["invert_keyword"], false)
	if m.Type == TypeKeyword && keyword == "" {
		return down("keyword is required for keyword monitors")
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, target.String(), bodyReader)
	if err != nil {
		return down(fmt.Sprintf("cannot build request: %v", err))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if !hasUserAgent {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	switch auth.scheme {
	case "basic":
		req.SetBasicAuth(auth.username, auth.password)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+auth.token)
	}

	transport := &http.Transport{
		DialContext:           GuardDialContext,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          0,
	}
	defer transport.CloseIdleConnections()
	if ignoreTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !followRedirects {
				return http.ErrUseLastResponse
			}
			// via holds previous requests: len(via)==maxRedirects means
			// maxRedirects follows already happened, so stop here.
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects: redirect limit exceeded", maxRedirects)
			}
			if err := redirectHostAllowed(req.Context(), req.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		msg := err.Error()
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Timeout() {
			msg = fmt.Sprintf("request timed out after %s", timeout)
		}
		res := CheckResult{Status: StatusDown, LatencyMs: elapsedMs(start), Message: msg, Details: details}
		if resp != nil {
			if resp.Body != nil {
				resp.Body.Close()
			}
			res.Code = intPtr(resp.StatusCode)
			if res.Details["final_url"] == nil && resp.Request != nil && resp.Request.URL != nil {
				res.Details["final_url"] = resp.Request.URL.String()
			}
		}
		return res
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return CheckResult{Status: StatusDown, LatencyMs: elapsedMs(start), Code: intPtr(resp.StatusCode), Message: fmt.Sprintf("error reading response body: %v", err), Details: details}
	}
	if len(data) > maxResponseBodyBytes {
		data = data[:maxResponseBodyBytes]
		details["truncated"] = true
	}
	latencyMs := elapsedMs(start)
	details["final_url"] = resp.Request.URL.String()

	if !accept(resp.StatusCode) {
		return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Code: intPtr(resp.StatusCode), Message: fmt.Sprintf("status %d not accepted", resp.StatusCode), Details: details}
	}

	if keyword != "" {
		found := strings.Contains(string(data), keyword)
		details["keyword_found"] = found
		switch {
		case found && invertKeyword:
			return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Code: intPtr(resp.StatusCode), Message: "keyword found (inverted)", Details: details}
		case !found && invertKeyword:
			return CheckResult{Status: StatusUp, LatencyMs: latencyMs, Code: intPtr(resp.StatusCode), Message: "keyword not found (inverted)", Details: details}
		case found:
			return CheckResult{Status: StatusUp, LatencyMs: latencyMs, Code: intPtr(resp.StatusCode), Message: "keyword found", Details: details}
		default:
			return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Code: intPtr(resp.StatusCode), Message: "keyword not found", Details: details}
		}
	}

	return CheckResult{Status: StatusUp, LatencyMs: latencyMs, Code: intPtr(resp.StatusCode), Message: fmt.Sprintf("status %d accepted", resp.StatusCode), Details: details}
}

// parseMonitorURL validates that target is an http(s) URL with a host.
func parseMonitorURL(target string) (*url.URL, error) {
	if strings.TrimSpace(target) == "" {
		return nil, fmt.Errorf("invalid URL %q: target is required", target)
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %v", target, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("invalid URL %q: scheme must be http or https", target)
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL %q: missing host", target)
	}
	return u, nil
}

// parseHTTPMethod validates the configured method (default GET).
func parseHTTPMethod(v any) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(configString(v, http.MethodGet)))
	if !allowedHTTPMethods[s] {
		return "", fmt.Errorf("invalid method %q: must be GET, POST, PUT, PATCH, DELETE, HEAD or OPTIONS", configString(v, ""))
	}
	return s, nil
}

// parseAcceptedStatusCodes parses a Kuma-style spec: comma-separated codes
// and ranges (e.g. "200,201,204" or "200-299"), tolerating spaces.
func parseAcceptedStatusCodes(spec string) (func(int) bool, error) {
	type codeRange struct{ lo, hi int }
	var ranges []codeRange
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid accepted_status_codes %q: empty entry", spec)
		}
		if loStr, hiStr, ok := strings.Cut(part, "-"); ok {
			lo, errLo := parseStatusCode(strings.TrimSpace(loStr))
			hi, errHi := parseStatusCode(strings.TrimSpace(hiStr))
			if errLo != nil || errHi != nil || lo > hi {
				return nil, fmt.Errorf("invalid accepted_status_codes %q: bad range %q", spec, part)
			}
			ranges = append(ranges, codeRange{lo, hi})
			continue
		}
		n, err := parseStatusCode(part)
		if err != nil {
			return nil, fmt.Errorf("invalid accepted_status_codes %q: bad code %q", spec, part)
		}
		ranges = append(ranges, codeRange{n, n})
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("invalid accepted_status_codes %q: empty spec", spec)
	}
	return func(code int) bool {
		for _, r := range ranges {
			if code >= r.lo && code <= r.hi {
				return true
			}
		}
		return false
	}, nil
}

func parseStatusCode(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 100 || n > 599 {
		return 0, fmt.Errorf("invalid status code %q", s)
	}
	return n, nil
}

// httpAuth carries validated credentials. Secrets must never be logged.
type httpAuth struct {
	scheme   string
	username string
	password string
	token    string
}

// parseHTTPAuth validates auth_type/username/password/token.
func parseHTTPAuth(cfg map[string]any) (httpAuth, error) {
	t := strings.ToLower(strings.TrimSpace(configString(cfg["auth_type"], "none")))
	switch t {
	case "", "none":
		return httpAuth{}, nil
	case "basic":
		u := configString(cfg["username"], "")
		p := configString(cfg["password"], "")
		if u == "" || p == "" {
			return httpAuth{}, fmt.Errorf("basic auth requires username and password")
		}
		return httpAuth{scheme: "basic", username: u, password: p}, nil
	case "bearer":
		tok := configString(cfg["token"], "")
		if tok == "" {
			return httpAuth{}, fmt.Errorf("bearer auth requires token")
		}
		return httpAuth{scheme: "bearer", token: tok}, nil
	default:
		return httpAuth{}, fmt.Errorf("unsupported auth_type %q: must be none, basic or bearer", t)
	}
}

// parseHTTPHeaders validates the custom headers map (<=20 entries, Host and
// Content-Length forbidden). It also reports whether User-Agent is overridden.
func parseHTTPHeaders(v any) (map[string]string, bool, error) {
	headers := map[string]string{}
	if v == nil {
		return headers, false, nil
	}
	var entries map[string]any
	switch t := v.(type) {
	case map[string]any:
		entries = t
	case map[string]string:
		entries = make(map[string]any, len(t))
		for k, val := range t {
			entries[k] = val
		}
	default:
		return nil, false, fmt.Errorf("invalid headers: must be an object")
	}
	if len(entries) > maxHeaderEntries {
		return nil, false, fmt.Errorf("too many headers: max %d entries", maxHeaderEntries)
	}
	hasUserAgent := false
	for k, val := range entries {
		if strings.EqualFold(k, "host") || strings.EqualFold(k, "content-length") {
			return nil, false, fmt.Errorf("forbidden header %q: must not be set manually", k)
		}
		if strings.TrimSpace(k) == "" {
			return nil, false, fmt.Errorf("invalid headers: empty header name")
		}
		if len(k) > maxHeaderKeyLen {
			return nil, false, fmt.Errorf("header name too long: max %d characters", maxHeaderKeyLen)
		}
		s, ok := val.(string)
		if !ok {
			s = fmt.Sprint(val)
		}
		headers[k] = s
		if strings.EqualFold(k, "user-agent") {
			hasUserAgent = true
		}
	}
	return headers, hasUserAgent, nil
}

// redirectHostAllowed re-validates each redirect hop against the SSRF
// blocklist. The admin-only env override relaxes the private-IP check but
// unresolvable hosts are still rejected.
func redirectHostAllowed(ctx context.Context, host string) error {
	h := stripBrackets(strings.TrimSpace(host))
	if h == "" {
		return fmt.Errorf("redirect to empty host blocked")
	}
	allowPrivate := os.Getenv(allowPrivateNetworkEnv) == "true"
	if ip := net.ParseIP(h); ip != nil {
		if !allowPrivate && IsPrivateIP(ip) {
			return fmt.Errorf("redirect to private address %q blocked", host)
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", h)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("cannot resolve redirect host %q", host)
	}
	if allowPrivate {
		return nil
	}
	for _, ip := range ips {
		if !IsPrivateIP(ip) {
			return nil
		}
	}
	return fmt.Errorf("redirect to private address %q blocked", host)
}

func configString(v any, def string) string {
	switch t := v.(type) {
	case nil:
		return def
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func configBool(v any, def bool) bool {
	switch t := v.(type) {
	case nil:
		return def
	case bool:
		return t
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(t)); err == nil {
			return b
		}
		return def
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	default:
		return def
	}
}

func configInt(v any, def int) int {
	switch t := v.(type) {
	case nil:
		return def
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n
		}
		return def
	default:
		return def
	}
}

func elapsedMs(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}

func intPtr(n int) *int {
	return &n
}
