package checks

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/henrygd/beszel/internal/entities/monitor"
)

// Checker defines the interface for monitor check implementations
type Checker interface {
	Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult
}

// CheckerRegistry holds all monitor type checkers
type CheckerRegistry struct {
	checkers map[string]Checker
}

// StubChecker returns a placeholder result for monitor types without full implementation
type StubChecker struct {
	Name string
}

func (s *StubChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	return &monitor.CheckResult{
		Status: monitor.StatusUp,
		Ping:   0,
		Msg:    fmt.Sprintf("%s monitoring is not fully implemented yet", s.Name),
	}
}

// PortChecker checks TCP connectivity to a specific service port
type PortChecker struct {
	Name        string
	DefaultPort int
}

func (c *PortChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	host := m.Hostname
	if host == "" {
		host = m.URL
	}
	if host == "" {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: "hostname or URL is required"}
	}
	port := m.Port
	if port == 0 {
		port = c.DefaultPort
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Duration(m.Timeout)*time.Second)
	if err != nil {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: err.Error()}
	}
	defer conn.Close()
	ping := int(time.Since(start).Milliseconds())
	return &monitor.CheckResult{Status: monitor.StatusUp, Ping: ping, Msg: fmt.Sprintf("%s port %d reachable", c.Name, port)}
}

// MySQLChecker checks MySQL/MariaDB connectivity via TCP
type MySQLChecker struct{}

func (c *MySQLChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	host := m.Hostname
	if host == "" {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: "hostname is required"}
	}
	port := m.Port
	if port == 0 {
		port = 3306
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Duration(m.Timeout)*time.Second)
	if err != nil {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: err.Error()}
	}
	defer conn.Close()
	ping := int(time.Since(start).Milliseconds())
	return &monitor.CheckResult{Status: monitor.StatusUp, Ping: ping, Msg: "MySQL port reachable"}
}

// WebSocketChecker checks WebSocket upgrade connectivity
type WebSocketChecker struct{}

func (c *WebSocketChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	urlStr := m.URL
	if urlStr == "" {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: "URL is required"}
	}
	start := time.Now()
	dialer := websocket.Dialer{HandshakeTimeout: time.Duration(m.Timeout) * time.Second}
	conn, _, err := dialer.Dial(urlStr, nil)
	if err != nil {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: err.Error()}
	}
	defer conn.Close()
	ping := int(time.Since(start).Milliseconds())
	return &monitor.CheckResult{Status: monitor.StatusUp, Ping: ping, Msg: "WebSocket connected"}
}

// SMTPChecker checks SMTP server connectivity
type SMTPChecker struct{}

func (c *SMTPChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	host := m.Hostname
	if host == "" {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: "hostname is required"}
	}
	port := m.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Duration(m.Timeout)*time.Second)
	if err != nil {
		return &monitor.CheckResult{Status: monitor.StatusDown, Msg: err.Error()}
	}
	defer conn.Close()
	ping := int(time.Since(start).Milliseconds())
	return &monitor.CheckResult{Status: monitor.StatusUp, Ping: ping, Msg: "SMTP port reachable"}
}

// NewCheckerRegistry creates a new registry with all checkers registered
func NewCheckerRegistry() *CheckerRegistry {
	registry := &CheckerRegistry{
		checkers: make(map[string]Checker),
	}

	// Register all checkers
	registry.Register(monitor.TypeHTTP, &HTTPChecker{})
	registry.Register(monitor.TypeHTTPS, &HTTPChecker{IsHTTPS: true})
	registry.Register(monitor.TypeTCP, &TCPChecker{})
	registry.Register(monitor.TypePing, &PingChecker{})
	registry.Register(monitor.TypeDNS, &DNSChecker{})
	registry.Register(monitor.TypeKeyword, &KeywordChecker{})
	registry.Register(monitor.TypeJSONQuery, &JSONQueryChecker{})
	registry.Register(monitor.TypeWebSocket, &WebSocketChecker{})
	registry.Register(monitor.TypeMySQL, &MySQLChecker{})
	registry.Register(monitor.TypeSMTP, &SMTPChecker{})
	// TCP-based connectivity checkers for database / protocol types
	registry.Register(monitor.TypePostgreSQL, &PortChecker{Name: "PostgreSQL", DefaultPort: 5432})
	registry.Register(monitor.TypeRedis, &PortChecker{Name: "Redis", DefaultPort: 6379})
	registry.Register(monitor.TypeMongoDB, &PortChecker{Name: "MongoDB", DefaultPort: 27017})
	registry.Register(monitor.TypeSQLServer, &PortChecker{Name: "SQL Server", DefaultPort: 1433})
	registry.Register(monitor.TypeOracleDB, &PortChecker{Name: "Oracle", DefaultPort: 1521})
	registry.Register(monitor.TypeRADIUS, &PortChecker{Name: "RADIUS", DefaultPort: 1812})
	registry.Register(monitor.TypeMQTT, &PortChecker{Name: "MQTT", DefaultPort: 1883})
	registry.Register(monitor.TypeRabbitMQ, &PortChecker{Name: "RabbitMQ", DefaultPort: 5672})
	registry.Register(monitor.TypeKafka, &PortChecker{Name: "Kafka", DefaultPort: 9092})
	registry.Register(monitor.TypeSIP, &PortChecker{Name: "SIP", DefaultPort: 5060})
	registry.Register(monitor.TypeTailscalePing, &PortChecker{Name: "Tailscale", DefaultPort: 80})

	// Stub checkers for types requiring special libraries or APIs
	registry.Register(monitor.TypeDocker, &StubChecker{Name: "Docker"})
	registry.Register(monitor.TypePush, &StubChecker{Name: "Push"})
	registry.Register(monitor.TypeManual, &StubChecker{Name: "Manual"})
	registry.Register(monitor.TypeSystemService, &StubChecker{Name: "System Service"})
	registry.Register(monitor.TypeRealBrowser, &StubChecker{Name: "Browser Engine"})
	registry.Register(monitor.TypeGRPCKeyword, &StubChecker{Name: "gRPC"})
	registry.Register(monitor.TypeSNMP, &StubChecker{Name: "SNMP"})
	registry.Register(monitor.TypeGlobalping, &StubChecker{Name: "Globalping"})
	registry.Register(monitor.TypeGameDig, &StubChecker{Name: "GameDig"})
	registry.Register(monitor.TypeSteam, &StubChecker{Name: "Steam"})

	return registry
}

// Register adds a checker for a monitor type
func (r *CheckerRegistry) Register(monitorType string, checker Checker) {
	r.checkers[monitorType] = checker
}

// Get returns the checker for a monitor type
func (r *CheckerRegistry) Get(monitorType string) (Checker, bool) {
	checker, ok := r.checkers[monitorType]
	return checker, ok
}

// HTTPChecker performs HTTP/HTTPS checks
type HTTPChecker struct {
	IsHTTPS bool
}

// Check performs an HTTP/HTTPS check
func (c *HTTPChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	start := time.Now()

	// Parse URL
	checkURL := m.URL
	if checkURL == "" {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Msg:    "URL is empty",
			Error:  fmt.Errorf("URL is empty"),
		}
	}

	// Ensure URL has scheme
	if !strings.HasPrefix(checkURL, "http://") && !strings.HasPrefix(checkURL, "https://") {
		if c.IsHTTPS {
			checkURL = "https://" + checkURL
		} else {
			checkURL = "http://" + checkURL
		}
	}

	// Create request
	method := m.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, checkURL, strings.NewReader(m.Body))
	if err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Msg:    fmt.Sprintf("Failed to create request: %v", err),
			Error:  err,
		}
	}

	// Add headers
	if m.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(m.Headers), &headers); err == nil {
			for key, value := range headers {
				req.Header.Set(key, value)
			}
		}
	}

	// Create client with timeout and TLS config
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			maxRedirects := m.MaxRedirects
			if maxRedirects == 0 {
				maxRedirects = 10
			}
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Configure TLS
	if c.IsHTTPS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: m.IgnoreTLSError,
		}
		client.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Msg:    fmt.Sprintf("Request failed: %v", err),
			Error:  err,
		}
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)
	ping := int(elapsed.Milliseconds())

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Ping:   ping,
			Msg:    fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	// Check certificate if HTTPS and cert expiry notification enabled
	var certExpiry int
	var certValid bool
	if c.IsHTTPS && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		certValid = true
		certExpiry = int(time.Until(cert.NotAfter).Hours() / 24)
	}

	return &monitor.CheckResult{
		Status:     monitor.StatusUp,
		Ping:       ping,
		Msg:        fmt.Sprintf("HTTP %d", resp.StatusCode),
		CertExpiry: certExpiry,
		CertValid:  certValid,
	}
}

// TCPChecker performs TCP port checks
type TCPChecker struct{}

// Check performs a TCP port check
func (c *TCPChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	start := time.Now()

	hostname := m.Hostname
	if hostname == "" {
		hostname = m.URL
	}

	port := m.Port
	if port == 0 {
		port = 80
	}

	address := fmt.Sprintf("%s:%d", hostname, port)

	// Create dialer with timeout
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Msg:    fmt.Sprintf("Connection failed: %v", err),
			Error:  err,
		}
	}
	defer conn.Close()

	elapsed := time.Since(start)
	ping := int(elapsed.Milliseconds())

	return &monitor.CheckResult{
		Status: monitor.StatusUp,
		Ping:   ping,
		Msg:    fmt.Sprintf("Connected in %dms", ping),
	}
}

// PingChecker performs ICMP ping checks
type PingChecker struct{}

// Check performs an ICMP ping check
func (c *PingChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	start := time.Now()

	hostname := m.Hostname
	if hostname == "" {
		hostname = m.URL
	}

	// Parse hostname to remove any scheme
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimSuffix(hostname, "/")

	// Resolve the address
	resolver := &net.Resolver{}
	addrs, err := resolver.LookupHost(ctx, hostname)
	if err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Msg:    fmt.Sprintf("DNS lookup failed: %v", err),
			Error:  err,
		}
	}

	if len(addrs) == 0 {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Msg:    "No IP addresses found",
			Error:  fmt.Errorf("no IP addresses found"),
		}
	}

	// Try to connect to port 7 (echo) or just check if host is reachable
	// Since raw ICMP requires root, we'll do a TCP connection to a common port
	address := net.JoinHostPort(addrs[0], "80")

	timeout := time.Duration(m.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		// Try port 443
		address = net.JoinHostPort(addrs[0], "443")
		conn, err = net.DialTimeout("tcp", address, timeout)
		if err != nil {
			return &monitor.CheckResult{
				Status: monitor.StatusDown,
				Msg:    fmt.Sprintf("Host unreachable: %v", err),
				Error:  err,
			}
		}
	}
	defer conn.Close()

	elapsed := time.Since(start)
	ping := int(elapsed.Milliseconds())

	return &monitor.CheckResult{
		Status: monitor.StatusUp,
		Ping:   ping,
		Msg:    fmt.Sprintf("Ping: %dms", ping),
	}
}

// DNSChecker performs DNS resolution checks
type DNSChecker struct{}

// Check performs a DNS resolution check
func (c *DNSChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	start := time.Now()

	hostname := m.Hostname
	if hostname == "" {
		hostname = m.URL
	}

	// Remove scheme if present
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimSuffix(hostname, "/")

	// Use custom DNS server if specified
	resolver := &net.Resolver{}
	if m.DNSResolveServer != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, network, m.DNSResolveServer+":53")
			},
		}
	}

	var err error
	var results []string

	// Perform DNS lookup based on record type
	recordType := m.DNSResolverMode
	if recordType == "" {
		recordType = "A"
	}

	switch recordType {
	case "A", "AAAA":
		results, err = resolver.LookupHost(ctx, hostname)
	case "CNAME":
		var cname string
		cname, err = resolver.LookupCNAME(ctx, hostname)
		if err == nil && cname != "" {
			results = []string{cname}
		}
	case "MX":
		var mxRecords []*net.MX
		mxRecords, err = resolver.LookupMX(ctx, hostname)
		if err == nil {
			for _, mx := range mxRecords {
				results = append(results, fmt.Sprintf("%s (priority: %d)", mx.Host, mx.Pref))
			}
		}
	case "NS":
		var nsRecords []*net.NS
		nsRecords, err = resolver.LookupNS(ctx, hostname)
		if err == nil {
			for _, ns := range nsRecords {
				results = append(results, ns.Host)
			}
		}
	case "TXT":
		results, err = resolver.LookupTXT(ctx, hostname)
	case "SRV":
		// SRV requires service and protocol
		_, srvRecords, err := resolver.LookupSRV(ctx, "", "", hostname)
		if err == nil {
			for _, srv := range srvRecords {
				results = append(results, fmt.Sprintf("%s:%d (priority: %d, weight: %d)",
					srv.Target, srv.Port, srv.Priority, srv.Weight))
			}
		}
	default:
		results, err = resolver.LookupHost(ctx, hostname)
	}

	elapsed := time.Since(start)
	ping := int(elapsed.Milliseconds())

	if err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Msg:    fmt.Sprintf("DNS lookup failed: %v", err),
			Error:  err,
		}
	}

	return &monitor.CheckResult{
		Status: monitor.StatusUp,
		Ping:   ping,
		Msg:    fmt.Sprintf("Resolved %d records in %dms", len(results), ping),
	}
}

// KeywordChecker performs HTTP checks with keyword validation
type KeywordChecker struct{}

// Check performs an HTTP check with keyword validation
func (c *KeywordChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	// First do HTTP check
	httpChecker := &HTTPChecker{}
	result := httpChecker.Check(ctx, m)

	if result.Status != monitor.StatusUp {
		return result
	}

	// Now we need to fetch the body and check for keyword
	// Re-fetch the body since we closed it in HTTPChecker
	checkURL := m.URL
	if !strings.HasPrefix(checkURL, "http://") && !strings.HasPrefix(checkURL, "https://") {
		checkURL = "https://" + checkURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
	if err != nil {
		return result
	}

	client := &http.Client{
		Timeout: time.Duration(m.Timeout) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return result
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Ping:   result.Ping,
			Msg:    fmt.Sprintf("Failed to read body: %v", err),
			Error:  err,
		}
	}

	bodyStr := string(body)
	keyword := m.Keyword
	found := strings.Contains(bodyStr, keyword)

	// Handle invert keyword option
	if m.InvertKeyword {
		found = !found
	}

	if !found {
		status := "not found"
		if m.InvertKeyword {
			status = "found (inverted)"
		}
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Ping:   result.Ping,
			Msg:    fmt.Sprintf("Keyword '%s' %s", keyword, status),
			Error:  fmt.Errorf("keyword check failed"),
		}
	}

	return result
}

// JSONQueryChecker performs HTTP checks with JSON path validation
type JSONQueryChecker struct{}

// Check performs an HTTP check with JSON path validation
func (c *JSONQueryChecker) Check(ctx context.Context, m *monitor.Monitor) *monitor.CheckResult {
	// First do HTTP check
	httpChecker := &HTTPChecker{}
	result := httpChecker.Check(ctx, m)

	if result.Status != monitor.StatusUp {
		return result
	}

	// Re-fetch the body for JSON parsing
	checkURL := m.URL
	if !strings.HasPrefix(checkURL, "http://") && !strings.HasPrefix(checkURL, "https://") {
		checkURL = "https://" + checkURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
	if err != nil {
		return result
	}

	client := &http.Client{
		Timeout: time.Duration(m.Timeout) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return result
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Ping:   result.Ping,
			Msg:    fmt.Sprintf("Failed to read body: %v", err),
			Error:  err,
		}
	}

	// Parse JSON
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Ping:   result.Ping,
			Msg:    fmt.Sprintf("Invalid JSON: %v", err),
			Error:  err,
		}
	}

	// Simple path evaluation (supports dot notation like "data.status")
	path := m.JSONQuery
	expectedValue := m.ExpectedValue

	value := evaluateJSONPath(data, path)

	if expectedValue != "" && value != expectedValue {
		return &monitor.CheckResult{
			Status: monitor.StatusDown,
			Ping:   result.Ping,
			Msg:    fmt.Sprintf("Expected '%s' but got '%s'", expectedValue, value),
			Error:  fmt.Errorf("JSON value mismatch"),
		}
	}

	return result
}

// evaluateJSONPath extracts a value from JSON using dot notation path
func evaluateJSONPath(data interface{}, path string) string {
	if path == "" {
		return ""
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			if val, ok := v[part]; ok {
				current = val
			} else {
				return ""
			}
		case []interface{}:
			// Try to parse as index
			if idx, err := strconv.Atoi(part); err == nil && idx >= 0 && idx < len(v) {
				current = v[idx]
			} else {
				return ""
			}
		default:
			return ""
		}
	}

	// Convert result to string
	switch v := current.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// URLValidator validates URL format
func URLValidator(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return err
	}
	if u.Host == "" {
		return fmt.Errorf("missing host in URL")
	}
	return nil
}

// IsValidStatusCode checks if HTTP status code is valid for UP status
func IsValidStatusCode(code int, validCodes []int) bool {
	if len(validCodes) == 0 {
		return code >= 200 && code < 300
	}
	for _, validCode := range validCodes {
		if code == validCode {
			return true
		}
	}
	return false
}

// ExtractDomain extracts domain from URL or hostname
func ExtractDomain(urlStr string) string {
	if urlStr == "" {
		return ""
	}

	// Try to parse as URL first
	if u, err := url.Parse(urlStr); err == nil && u.Host != "" {
		return u.Hostname()
	}

	// Remove scheme if present
	domain := urlStr
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimSuffix(domain, "/")

	// Remove port if present
	if idx := strings.LastIndex(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	return domain
}

// ValidateRegex validates a regex pattern
func ValidateRegex(pattern string) error {
	_, err := regexp.Compile(pattern)
	return err
}
