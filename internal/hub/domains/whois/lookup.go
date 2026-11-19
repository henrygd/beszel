package whois

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/domain"
	"github.com/henrygd/beszel/internal/hub/domains/detect"
)

// LookupService handles WHOIS lookups with multiple fallback methods
type LookupService struct {
	whoisXMLAPIKey string
	rdapCache      map[string]string
}

// NewLookupService creates a new WHOIS lookup service
func NewLookupService(apiKey string) *LookupService {
	return &LookupService{
		whoisXMLAPIKey: apiKey,
		rdapCache:      make(map[string]string),
	}
}

// LookupDomain performs a comprehensive domain lookup (WHOIS, DNS, SSL, Host, Headers, SEO)
func (s *LookupService) LookupDomain(ctx context.Context, domainName string) (*domain.Domain, error) {
	// Clean domain name
	domainName = cleanDomain(domainName)

	// Extract TLD
	parts := strings.Split(domainName, ".")
	tld := ""
	if len(parts) >= 2 {
		tld = strings.ToLower(parts[len(parts)-1])
	}

	// Initialize domain struct
	d := &domain.Domain{
		DomainName:      domainName,
		TLD:             tld,
		Active:          true,
		AlertDaysBefore: 30, // Default: alert 30 days before expiry
		Tags:            []string{},
		NameServers:     []string{},
		MXRecords:       []string{},
		TXTRecords:      []string{},
		IPv4Addresses:   []string{},
		IPv6Addresses:   []string{},
		Headers:         []domain.Header{},
		Certificates:    []domain.Certificate{},
		DomainStatuses:  []string{},
	}

	// Perform WHOIS lookup
	whoisData, rawWhois, err := s.LookupWHOIS(ctx, domainName)
	if err == nil && whoisData != nil {
		s.applyWHOISData(d, whoisData)
		d.WHOISRaw = rawWhois
	}

	// Perform DNS lookups
	s.lookupDNS(ctx, domainName, d)

	// Perform SSL lookup (certificate chain)
	s.lookupCertificateChain(ctx, domainName, d)

	// Perform host lookup (using first IPv4)
	if len(d.IPv4Addresses) > 0 {
		s.lookupHost(d.IPv4Addresses[0], d)
	}

	// Fetch HTTP headers for provider detection
	s.lookupHeaders(ctx, domainName, d)

	// Fetch SEO metadata
	s.lookupSEO(ctx, domainName, d)

	// Detect providers from gathered data
	s.detectProviders(d)

	// Fetch favicon
	d.FaviconURL = fmt.Sprintf("https://www.google.com/s2/favicons?domain=%s&sz=128", domainName)

	d.LastChecked = time.Now()
	return d, nil
}

// LookupWHOIS performs WHOIS lookup with multiple fallback methods.
// Expiry-bearing data wins; the first partially valid result is kept as
// last resort so GDPR-redacted responses (e.g. .eu) still return something.
func (s *LookupService) LookupWHOIS(ctx context.Context, domainName string) (*domain.WHOISData, string, error) {
	var lastErr error
	var partial *domain.WHOISData
	var partialRaw string

	consider := func(data *domain.WHOISData, raw string, err error) bool {
		if err != nil {
			lastErr = err
			return false
		}
		if data == nil {
			return false
		}
		if hasExpiry(data) {
			partial, partialRaw = data, raw
			return true
		}
		if partial == nil && hasValidData(data) {
			partial, partialRaw = data, raw
		}
		return false
	}

	// Try RDAP first
	data, err := s.tryRDAP(ctx, domainName)
	if consider(data, "", err) {
		return partial, partialRaw, nil
	}

	// Try TCP WHOIS (this should work for .eu domains)
	data, raw, err := s.tryTCPWHOIS(ctx, domainName)
	if consider(data, raw, err) {
		return partial, partialRaw, nil
	}

	// Try native whois command (often works when TCP fails)
	data, raw, err = s.tryNativeWHOIS(ctx, domainName)
	if consider(data, raw, err) {
		return partial, partialRaw, nil
	}

	// Try EURid web scraping for .eu domains to get expiry dates
	parts := strings.Split(domainName, ".")
	if len(parts) >= 2 && strings.ToLower(parts[len(parts)-1]) == "eu" {
		data, err = s.tryEURidWebScraping(ctx, domainName)
		if consider(data, "", err) {
			return partial, partialRaw, nil
		}

		// Try alternative WHOIS services for .eu domains
		data, err = s.tryAlternativeWHOIS(ctx, domainName)
		if consider(data, "", err) {
			return partial, partialRaw, nil
		}
	}

	// Try WhoisXML API if key is configured (this can provide expiry dates for .eu domains)
	if s.whoisXMLAPIKey != "" {
		data, err = s.tryWhoisXML(ctx, domainName)
		if consider(data, "", err) {
			return partial, partialRaw, nil
		}
	}

	if partial != nil {
		return partial, partialRaw, nil
	}
	return nil, "", fmt.Errorf("all WHOIS lookup methods failed for %s: %w", domainName, lastErr)
}

// tryRDAP attempts RDAP lookup
func (s *LookupService) tryRDAP(ctx context.Context, domainName string) (*domain.WHOISData, error) {
	// Get TLD
	parts := strings.Split(domainName, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid domain format")
	}
	tld := parts[len(parts)-1]

	// Get RDAP base URL
	baseURL, err := s.getRDAPBaseURL(ctx, tld)
	if err != nil {
		return nil, err
	}

	// Make RDAP request
	url := fmt.Sprintf("%s/domain/%s", baseURL, domainName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rdap+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RDAP returned status %d", resp.StatusCode)
	}

	var rdapResp struct {
		LdhName string   `json:"ldhName"`
		Handle  string   `json:"handle"`
		Status  []string `json:"status"`
		Events  []struct {
			EventAction string `json:"eventAction"`
			EventDate   string `json:"eventDate"`
		} `json:"events"`
		Entities []struct {
			Roles     []string `json:"roles"`
			PublicIds []struct {
				Type       string `json:"type"`
				Identifier string `json:"identifier"`
			} `json:"publicIds"`
			VCardArray []interface{} `json:"vcardArray"`
		} `json:"entities"`
		SecureDNS struct {
			ZoneSigned bool `json:"zoneSigned"`
		} `json:"secureDNS"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rdapResp); err != nil {
		return nil, err
	}

	// Parse events
	var creationDate, expiryDate, updatedDate *time.Time
	for _, event := range rdapResp.Events {
		t, err := time.Parse(time.RFC3339, event.EventDate)
		if err != nil || t.IsZero() {
			continue
		}
		switch event.EventAction {
		case "registration":
			creationDate = &t
		case "expiration":
			expiryDate = &t
		case "last changed":
			updatedDate = &t
		}
	}

	// Find registrar
	var registrarName, registrarID string
	for _, entity := range rdapResp.Entities {
		for _, role := range entity.Roles {
			if role == "registrar" {
				// Try to get name from vCard
				if len(entity.VCardArray) > 1 {
					if vcard, ok := entity.VCardArray[1].([]interface{}); ok {
						for _, item := range vcard {
							if arr, ok := item.([]interface{}); ok && len(arr) >= 4 {
								if arr[0] == "fn" {
									if name, ok := arr[3].(string); ok {
										registrarName = name
									}
								}
							}
						}
					}
				}
				// Get IANA ID
				for _, pid := range entity.PublicIds {
					if pid.Type == "IANA Registrar ID" {
						registrarID = pid.Identifier
					}
				}
			}
		}
	}

	dnssec := ""
	if rdapResp.SecureDNS.ZoneSigned {
		dnssec = "signed"
	}

	return &domain.WHOISData{
		DomainName: rdapResp.LdhName,
		Status:     rdapResp.Status,
		DNSSEC:     dnssec,
		Dates: domain.WHOISDates{
			ExpiryDate:   expiryDate,
			CreationDate: creationDate,
			UpdatedDate:  updatedDate,
		},
		Registrar: domain.WHOISRegistrar{
			Name:             registrarName,
			ID:               registrarID,
			URL:              "",
			RegistryDomainID: rdapResp.Handle,
		},
	}, nil
}

// tryNativeWHOIS tries the native whois command
func (s *LookupService) tryNativeWHOIS(ctx context.Context, domainName string) (*domain.WHOISData, string, error) {
	// Check if whois command exists
	_, err := exec.LookPath("whois")
	if err != nil {
		return nil, "", fmt.Errorf("whois command not found")
	}

	// Use longer timeout for .eu domains
	timeout := 10 * time.Second
	parts := strings.Split(domainName, ".")
	if len(parts) >= 2 && strings.ToLower(parts[len(parts)-1]) == "eu" {
		timeout = 20 * time.Second
	}

	// Execute whois with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "whois", domainName)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", err
	}

	outStr := string(output)
	data, err := s.parseWHOISOutput(outStr, domainName)
	return data, outStr, err
}

// whoisServers maps common TLDs to their WHOIS servers
var whoisServers = map[string]string{
	"com":    "whois.verisign-grs.com",
	"net":    "whois.verisign-grs.com",
	"org":    "whois.pir.org",
	"io":     "whois.nic.io",
	"co":     "whois.nic.co",
	"dev":    "whois.nic.google",
	"app":    "whois.nic.google",
	"xyz":    "whois.nic.xyz",
	"info":   "whois.afilias.net",
	"biz":    "whois.biz",
	"us":     "whois.nic.us",
	"uk":     "whois.nic.uk",
	"de":     "whois.denic.de",
	"fr":     "whois.nic.fr",
	"eu":     "whois.eu",
	"nl":     "whois.domain-registry.nl",
	"ca":     "whois.cira.ca",
	"au":     "whois.auda.org.au",
	"me":     "whois.nic.me",
	"tv":     "whois.nic.tv",
	"cc":     "whois.nic.cc",
	"ws":     "whois.website.ws",
	"name":   "whois.nic.name",
	"mobi":   "whois.dotmobiregistry.net",
	"asia":   "whois.nic.asia",
	"pro":    "whois.nic.pro",
	"jobs":   "whois.nic.jobs",
	"travel": "whois.nic.travel",
}

// tryTCPWHOIS performs WHOIS lookup via direct TCP connection (port 43)
func (s *LookupService) tryTCPWHOIS(ctx context.Context, domainName string) (*domain.WHOISData, string, error) {
	parts := strings.Split(domainName, ".")
	if len(parts) < 2 {
		return nil, "", fmt.Errorf("invalid domain format")
	}
	tld := strings.ToLower(parts[len(parts)-1])

	server, ok := whoisServers[tld]
	if !ok {
		// Fallback to IANA for unknown TLDs
		server = "whois.iana.org"
	}

	addr := net.JoinHostPort(server, "43")

	// Use longer timeout for .eu domains as they can be slow
	timeout := 10 * time.Second
	if tld == "eu" {
		timeout = 20 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, "", fmt.Errorf("tcp whois dial failed: %w", err)
	}
	defer conn.Close()

	// Some servers require the domain followed by \r\n
	query := domainName + "\r\n"
	if _, err := conn.Write([]byte(query)); err != nil {
		return nil, "", fmt.Errorf("tcp whois write failed: %w", err)
	}

	// Read response with deadline
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, "", err
	}

	var output strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	data, err := s.parseWHOISOutput(output.String(), domainName)
	return data, output.String(), err
}

// tryWhoisXML tries the WhoisXML API
func (s *LookupService) tryWhoisXML(ctx context.Context, domainName string) (*domain.WHOISData, error) {
	if s.whoisXMLAPIKey == "" {
		return nil, fmt.Errorf("no API key configured")
	}

	url := fmt.Sprintf(
		"https://www.whoisxmlapi.com/whoisserver/WhoisService?apiKey=%s&outputFormat=json&domainName=%s",
		s.whoisXMLAPIKey, domainName,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("WhoisXML API returned %d", resp.StatusCode)
	}

	var result struct {
		WhoisRecord struct {
			DomainName      string `json:"domainName"`
			RegistrarName   string `json:"registrarName"`
			RegistrarIANAID string `json:"registrarIANAID"`
			RegistryData    struct {
				Status                string `json:"status"`
				CreatedDateNormalized string `json:"createdDateNormalized"`
				ExpiresDateNormalized string `json:"expiresDateNormalized"`
				UpdatedDateNormalized string `json:"updatedDateNormalized"`
				WhoisServer           string `json:"whoisServer"`
			} `json:"registryData"`
		} `json:"WhoisRecord"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	record := result.WhoisRecord
	registry := record.RegistryData

	return &domain.WHOISData{
		DomainName: record.DomainName,
		Status:     strings.Split(registry.Status, ", "),
		Dates: domain.WHOISDates{
			ExpiryDate:   parseDatePtr(registry.ExpiresDateNormalized),
			CreationDate: parseDatePtr(registry.CreatedDateNormalized),
			UpdatedDate:  parseDatePtr(registry.UpdatedDateNormalized),
		},
		Registrar: domain.WHOISRegistrar{
			Name: record.RegistrarName,
			ID:   record.RegistrarIANAID,
			URL:  fmt.Sprintf("https://%s", registry.WhoisServer),
		},
	}, nil
}

// parseEUWHOIS parses .eu domain WHOIS output which has a unique format
func (s *LookupService) parseEUWHOIS(output, domainName string) (*domain.WHOISData, error) {
	lines := strings.Split(output, "\n")

	var registrarName, organization string
	var statuses []string

	// Parse the .eu specific format
	currentSection := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}

		// Track sections
		if strings.HasPrefix(line, "Registrant:") {
			currentSection = "registrant"
			continue
		}
		if strings.HasPrefix(line, "Technical:") {
			currentSection = "technical"
			continue
		}
		if strings.HasPrefix(line, "Registrar:") {
			currentSection = "registrar"
			continue
		}
		if strings.HasPrefix(line, "Name servers:") {
			currentSection = "nameservers"
			continue
		}

		// Parse based on current section
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])

			switch currentSection {
			case "registrar":
				if strings.TrimSpace(key) == "Name" {
					registrarName = value
				}
				if strings.TrimSpace(key) == "Website" {
					// Could extract website URL if needed
				}
			case "technical":
				if strings.TrimSpace(key) == "Organisation" {
					organization = value
				}
			}
		}
	}

	// For .eu domains, we often don't get expiry dates, so we'll return what we have
	return &domain.WHOISData{
		DomainName: domainName,
		Status:     statuses,
		DNSSEC:     "", // .eu WHOIS doesn't provide DNSSEC info
		Dates: domain.WHOISDates{
			ExpiryDate:   nil, // .eu domains don't show expiry in TCP WHOIS
			CreationDate: nil,
			UpdatedDate:  nil,
		},
		Registrar: domain.WHOISRegistrar{
			Name: registrarName,
			ID:   "",
			URL:  "",
		},
		Registrant: domain.WHOISContact{
			Name:         "NOT DISCLOSED",
			Organization: organization,
		},
	}, nil
}

// tryEURidWebScraping attempts to scrape EURid's web WHOIS for .eu domains
func (s *LookupService) tryEURidWebScraping(ctx context.Context, domainName string) (*domain.WHOISData, error) {
	// Try multiple EURid endpoints
	endpoints := []string{
		fmt.Sprintf("https://whois.eurid.eu/en/?q=%s", domainName),
		fmt.Sprintf("https://whois.eurid.eu/en/search?q=%s", domainName),
		fmt.Sprintf("https://www.eurid.eu/en/whois/?domain=%s", domainName),
	}

	for _, url := range endpoints {
		data, err := s.tryEURidEndpoint(ctx, url, domainName)
		if err == nil && data != nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("all EURid web scraping attempts failed for %s", domainName)
}

// tryEURidEndpoint attempts to scrape a specific EURid endpoint
func (s *LookupService) tryEURidEndpoint(ctx context.Context, url, domainName string) (*domain.WHOISData, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use more realistic browser headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Ch-Ua", "\"Not_A Brand\";v=\"8\", \"Chromium\";v=\"120\", \"Google Chrome\";v=\"120\"")
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", "\"Windows\"")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch EURid web page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("EURid web page returned status %d", resp.StatusCode)
	}

	// Read the HTML response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read EURid response: %w", err)
	}

	// Parse the HTML to extract expiry date
	return s.parseEURidWebHTML(string(body), domainName)
}

// tryAlternativeWHOIS tries alternative WHOIS services for .eu domains
func (s *LookupService) tryAlternativeWHOIS(ctx context.Context, domainName string) (*domain.WHOISData, error) {
	// Try multiple alternative WHOIS services
	services := []struct {
		name string
		url  string
	}{
		{"whois.com", fmt.Sprintf("https://www.whois.com/whois/%s", domainName)},
		{"who.is", fmt.Sprintf("https://who.is/whois/%s", domainName)},
		{"ip2location.com", fmt.Sprintf("https://www.ip2location.com/whois/%s", domainName)},
	}

	for _, service := range services {
		data, err := s.tryAlternativeWHOISService(ctx, service.name, service.url, domainName)
		if err == nil && data != nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("all alternative WHOIS services failed for %s", domainName)
}

// tryAlternativeWHOISService attempts to fetch WHOIS data from an alternative service
func (s *LookupService) tryAlternativeWHOISService(ctx context.Context, serviceName, url, domainName string) (*domain.WHOISData, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", serviceName, err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", serviceName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s returned status %d", serviceName, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s response: %w", serviceName, err)
	}

	return s.parseAlternativeWHOISHTML(string(body), domainName, serviceName)
}

// parseAlternativeWHOISHTML parses HTML from alternative WHOIS services
func (s *LookupService) parseAlternativeWHOISHTML(html, domainName, serviceName string) (*domain.WHOISData, error) {
	var expiryDate, registrarName, status string

	// Look for expiry date patterns (common across WHOIS services)
	expiryPatterns := []string{
		`Expiry Date:\s*</[^>]*>\s*([^<\n]+)`,
		`Expiry Date:</[^>]*>\s*([^<\n]+)`,
		`Expires on:\s*</[^>]*>\s*([^<\n]+)`,
		`Expires:\s*</[^>]*>\s*([^<\n]+)`,
		`"expiry":"([^"]+)"`,
		`"expires":"([^"]+)"`,
		`data-expiry="([^"]+)"`,
		`\d{4}-\d{2}-\d{2}`, // ISO date pattern
		`\d{2}/\d{2}/\d{4}`, // DD/MM/YYYY pattern
	}

	for _, pattern := range expiryPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			expiryDate = strings.TrimSpace(matches[1])
			break
		}
	}

	// Look for registrar name
	registrarPatterns := []string{
		`Registrar:\s*</[^>]*>\s*([^<\n]+)`,
		`Registrar:</[^>]*>\s*([^<\n]+)`,
		`Registered through:\s*</[^>]*>\s*([^<\n]+)`,
		`"registrar":"([^"]+)"`,
		`data-registrar="([^"]+)"`,
	}

	for _, pattern := range registrarPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			registrarName = strings.TrimSpace(matches[1])
			break
		}
	}

	// Look for status
	statusPatterns := []string{
		`Status:\s*</[^>]*>\s*([^<\n]+)`,
		`Status:</[^>]*>\s*([^<\n]+)`,
		`"status":"([^"]+)"`,
		`data-status="([^"]+)"`,
	}

	for _, pattern := range statusPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			status = strings.TrimSpace(matches[1])
			break
		}
	}

	// Parse expiry date if found
	var parsedExpiry *time.Time
	if expiryDate != "" {
		// Try different date formats
		dateFormats := []string{
			"2006-01-02",           // ISO
			"02/01/2006",           // DD/MM/YYYY
			"01/02/2006",           // MM/DD/YYYY
			"2006-01-02T15:04:05Z", // ISO with time
		}

		for _, format := range dateFormats {
			if parsed, err := time.Parse(format, expiryDate); err == nil {
				parsedExpiry = &parsed
				break
			}
		}
	}

	// Create WHOIS data structure
	var statuses []string
	if status != "" {
		statuses = []string{status}
	}

	return &domain.WHOISData{
		DomainName: domainName,
		Status:     statuses,
		DNSSEC:     "",
		Dates: domain.WHOISDates{
			ExpiryDate:   parsedExpiry,
			CreationDate: nil,
			UpdatedDate:  nil,
		},
		Registrar: domain.WHOISRegistrar{
			Name: registrarName,
			ID:   "",
			URL:  "",
		},
		Registrant: domain.WHOISContact{
			Name:         "NOT DISCLOSED",
			Organization: "",
		},
	}, nil
}

// parseEURidWebHTML parses EURid's web WHOIS HTML to extract domain information
func (s *LookupService) parseEURidWebHTML(html, domainName string) (*domain.WHOISData, error) {
	// This is a simplified HTML parser - in production, you'd want to use a proper HTML parser
	// For now, we'll use regex to find key information

	var expiryDate, registrarName, status string

	// Look for expiry date patterns in EURid's HTML
	expiryPatterns := []string{
		`Expiry date:\s*</strong>\s*(\d{2}/\d{2}/\d{4})`,
		`Expiry date:</strong>\s*(\d{2}/\d{2}/\d{4})`,
		`Expiry date</strong>\s*(\d{2}/\d{2}/\d{4})`,
		`"expiryDate":"([^"]+)"`,
		`data-expiry="([^"]+)"`,
	}

	for _, pattern := range expiryPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			expiryDate = matches[1]
			break
		}
	}

	// Look for registrar name
	registrarPatterns := []string{
		`Registrar:\s*</strong>\s*([^<\n]+)`,
		`Registrar:</strong>\s*([^<\n]+)`,
		`Registrar</strong>\s*([^<\n]+)`,
		`"registrar":"([^"]+)"`,
		`data-registrar="([^"]+)"`,
	}

	for _, pattern := range registrarPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			registrarName = strings.TrimSpace(matches[1])
			break
		}
	}

	// Look for status
	statusPatterns := []string{
		`Status:\s*</strong>\s*([^<\n]+)`,
		`Status:</strong>\s*([^<\n]+)`,
		`Status</strong>\s*([^<\n]+)`,
		`"status":"([^"]+)"`,
		`data-status="([^"]+)"`,
	}

	for _, pattern := range statusPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			status = strings.TrimSpace(matches[1])
			break
		}
	}

	// Parse expiry date if found
	var parsedExpiry *time.Time
	if expiryDate != "" {
		// Try DD/MM/YYYY format first
		if parsed, err := time.Parse("02/01/2006", expiryDate); err == nil {
			parsedExpiry = &parsed
		} else if parsed, err := time.Parse("2006-01-02", expiryDate); err == nil {
			parsedExpiry = &parsed
		}
	}

	// Create WHOIS data structure
	var statuses []string
	if status != "" {
		statuses = []string{status}
	}

	return &domain.WHOISData{
		DomainName: domainName,
		Status:     statuses,
		DNSSEC:     "",
		Dates: domain.WHOISDates{
			ExpiryDate:   parsedExpiry,
			CreationDate: nil,
			UpdatedDate:  nil,
		},
		Registrar: domain.WHOISRegistrar{
			Name: registrarName,
			ID:   "",
			URL:  "",
		},
		Registrant: domain.WHOISContact{
			Name:         "NOT DISCLOSED",
			Organization: "",
		},
	}, nil
}

// parseWHOISOutput parses the raw WHOIS text output
func (s *LookupService) parseWHOISOutput(output, domainName string) (*domain.WHOISData, error) {
	lines := strings.Split(output, "\n")
	data := make(map[string]string)

	// Special handling for .eu domains which have a different format
	parts := strings.Split(domainName, ".")
	isEUDomain := len(parts) >= 2 && strings.ToLower(parts[len(parts)-1]) == "eu"

	if isEUDomain {
		return s.parseEUWHOIS(output, domainName)
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}

		// Parse "Key: Value" format
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.ToLower(strings.TrimSpace(line[:idx]))
			value := strings.TrimSpace(line[idx+1:])
			// Normalize key
			key = strings.ReplaceAll(key, " ", "_")
			key = strings.ReplaceAll(key, "/", "_")
			if value != "" && !strings.HasPrefix(value, "REDACTED") {
				data[key] = value
			}
		}
	}

	// Extract dates - try many field name variations used by different registries
	expiryDate := s.parseDate(
		data["registry_expiry_date"],
		data["registrar_registration_expiration_date"],
		data["expiry_date"],
		data["expiration_time"],
		data["expire"],
		data["paid_until"],
		data["expire_date"],
		data["renewal_date"],
		data["valid_until"],
	)
	creationDate := s.parseDate(
		data["creation_date"],
		data["created_date"],
		data["registration_time"],
		data["registered_on"],
		data["domain_registered"],
	)
	updatedDate := s.parseDate(
		data["updated_date"],
		data["last_updated"],
		data["last_modified"],
		data["modified_date"],
	)

	// Extract registrar - try multiple field names used by different WHOIS servers
	// .eu domains use "Name:" under Registrar section
	registrarName := data["registrar"]
	if registrarName == "" {
		registrarName = data["registrar_name"]
	}
	if registrarName == "" {
		registrarName = data["sponsoring_registrar"]
	}
	if registrarName == "" {
		registrarName = data["registrar_organization"]
	}
	if registrarName == "" {
		registrarName = data["registrant_organization"]
	}
	if registrarName == "" {
		registrarName = data["registrar_url"]
	}
	if registrarName == "" {
		registrarName = data["registrar_abuse_contact_email"]
	}
	if registrarName == "" {
		registrarName = "Unknown"
	}

	// Parse status
	statusStr := data["domain_status"]
	var statuses []string
	if statusStr != "" {
		statuses = s.parseStatus(statusStr)
	}

	// Extract registrant contact info
	registrant := domain.WHOISContact{
		Name:         data["registrant_name"],
		Organization: data["registrant_organization"],
		Street:       data["registrant_street"],
		City:         data["registrant_city"],
		State:        data["registrant_state_province"],
		Country:      data["registrant_country"],
		PostalCode:   data["registrant_postal_code"],
	}

	// Try alternate field names for registrant (.eu uses "holder", other variations)
	if registrant.Name == "" {
		registrant.Name = data["registrant"]
	}
	if registrant.Name == "" {
		registrant.Name = data["holder"]
	}
	if registrant.Name == "" {
		registrant.Name = data["domain_holder"]
	}
	if registrant.Organization == "" {
		registrant.Organization = data["org"]
	}
	if registrant.Organization == "" {
		registrant.Organization = data["organization"]
	}
	if registrant.Organization == "" {
		registrant.Organization = data["holder_org"]
	}
	if registrant.Country == "" {
		registrant.Country = data["country"]
	}
	if registrant.Country == "" {
		registrant.Country = data["holder_country"]
	}

	// Parse DNSSEC more thoroughly
	dnssec := data["dnssec"]
	if dnssec == "" {
		// Try alternate field names
		dnssec = data["dnssec_signed"]
	}
	if dnssec == "" {
		dnssec = data["signed_dnssec"]
	}
	// Normalize DNSSEC value
	dnssec = strings.ToLower(strings.TrimSpace(dnssec))
	if dnssec == "signed" || dnssec == "yes" || dnssec == "true" {
		dnssec = "signed"
	} else if dnssec == "unsigned" || dnssec == "no" || dnssec == "false" {
		dnssec = "unsigned"
	}

	return &domain.WHOISData{
		DomainName: domainName,
		Status:     statuses,
		DNSSEC:     dnssec,
		Dates: domain.WHOISDates{
			ExpiryDate:   expiryDate,
			CreationDate: creationDate,
			UpdatedDate:  updatedDate,
		},
		Registrar: domain.WHOISRegistrar{
			Name:             registrarName,
			ID:               data["registrar_iana_id"],
			URL:              data["registrar_url"],
			RegistryDomainID: data["registry_domain_id"],
		},
		Registrant: registrant,
		Abuse: domain.WHOISAbuse{
			Email: data["registrar_abuse_contact_email"],
			Phone: data["registrar_abuse_contact_phone"],
		},
	}, nil
}

// parseDate attempts to parse a date from multiple possible formats
func (s *LookupService) parseDate(dates ...string) *time.Time {
	formats := []string{
		// Standard ISO formats
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.0",
		// US formats
		"01/02/2006",
		"01/02/2006 15:04:05",
		// European formats
		"02/01/2006",
		"02.01.2006",
		// Verbal formats
		"Jan 2 2006",
		"January 2 2006",
		"2 Jan 2006",
		"2 January 2006",
		"Jan 02 2006",
		"02-Jan-2006",
		"2-Jan-2006",
		// With timezone names
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05 UTC",
		// Common registrar formats
		"Monday, January 2, 2006",
		"Mon, 02 Jan 2006 15:04:05 MST",
		"Mon, 2 Jan 2006 15:04:05 MST",
		// Additional formats
		"20060102",
		"20060102150405",
	}

	for _, dateStr := range dates {
		if dateStr == "" || dateStr == "REDACTED" || strings.Contains(dateStr, "REDACTED") {
			continue
		}
		dateStr = strings.TrimSpace(dateStr)
		// Remove common prefixes/suffixes that don't help
		dateStr = strings.TrimPrefix(dateStr, "before ")
		dateStr = strings.TrimPrefix(dateStr, "after ")

		for _, format := range formats {
			if t, err := time.Parse(format, dateStr); err == nil {
				return &t
			}
		}
	}
	return nil
}

// parseStatus parses domain status from WHOIS output
func (s *LookupService) parseStatus(statusStr string) []string {
	knownStatuses := []string{
		"clientDeleteProhibited", "clientHold", "clientRenewProhibited",
		"clientTransferProhibited", "clientUpdateProhibited",
		"serverDeleteProhibited", "serverHold", "serverRenewProhibited",
		"serverTransferProhibited", "serverUpdateProhibited",
		"inactive", "ok", "pendingCreate", "pendingDelete", "pendingRenew",
		"pendingRestore", "pendingTransfer", "pendingUpdate",
		"addPeriod", "autoRenewPeriod", "renewPeriod", "transferPeriod",
	}

	statusStr = strings.ToLower(statusStr)
	var matches []string
	for _, status := range knownStatuses {
		if strings.Contains(statusStr, strings.ToLower(status)) {
			matches = append(matches, status)
		}
	}
	return matches
}

// getRDAPBaseURL gets the RDAP base URL for a TLD
func (s *LookupService) getRDAPBaseURL(ctx context.Context, tld string) (string, error) {
	// Check cache
	if url, ok := s.rdapCache[tld]; ok {
		return url, nil
	}

	// Fetch IANA RDAP bootstrap
	url := "https://data.iana.org/rdap/dns.json"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var bootstrap struct {
		Services [][]interface{} `json:"services"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bootstrap); err != nil {
		return "", err
	}

	// Populate cache and find URL for this TLD
	for _, service := range bootstrap.Services {
		if len(service) >= 2 {
			tlds, ok1 := service[0].([]interface{})
			urls, ok2 := service[1].([]interface{})
			if ok1 && ok2 && len(urls) > 0 {
				if urlStr, ok := urls[0].(string); ok {
					for _, t := range tlds {
						if tldStr, ok := t.(string); ok {
							s.rdapCache[tldStr] = strings.TrimSuffix(urlStr, "/")
						}
					}
				}
			}
		}
	}

	if url, ok := s.rdapCache[tld]; ok {
		return url, nil
	}
	return "", fmt.Errorf("no RDAP server found for TLD .%s", tld)
}

// lookupDNS performs DNS lookups
func (s *LookupService) lookupDNS(ctx context.Context, domainName string, d *domain.Domain) {
	// NS records
	nsRecords, _ := net.LookupNS(domainName)
	for _, ns := range nsRecords {
		d.NameServers = append(d.NameServers, ns.Host)
	}

	// MX records
	mxRecords, _ := net.LookupMX(domainName)
	for _, mx := range mxRecords {
		d.MXRecords = append(d.MXRecords, fmt.Sprintf("%s (priority: %d)", mx.Host, mx.Pref))
	}

	// TXT records
	txtRecords, _ := net.LookupTXT(domainName)
	d.TXTRecords = txtRecords

	// CNAME record
	cname, err := net.LookupCNAME(domainName)
	if err == nil && cname != domainName && cname != "" {
		d.CNAMERecord = cname
	}

	// SRV records (common services)
	srvServices := []string{"sip", "xmpp-server", "ldap", "autodiscover", "imap", "smtp", "caldavs", "carddavs"}
	srvProtos := []string{"tcp", "udp", "tls"}
	for _, service := range srvServices {
		for _, proto := range srvProtos {
			_, addrs, err := net.LookupSRV(service, proto, domainName)
			if err == nil {
				for _, addr := range addrs {
					d.SRVRecords = append(d.SRVRecords, fmt.Sprintf("_%s._%s %s:%d (priority: %d, weight: %d)", service, proto, addr.Target, addr.Port, addr.Priority, addr.Weight))
				}
			}
		}
	}

	// IPv4
	ipv4Addrs, _ := net.LookupHost(domainName)
	for _, ip := range ipv4Addrs {
		if strings.Contains(ip, ".") {
			d.IPv4Addresses = append(d.IPv4Addresses, ip)
		}
	}

	// IPv6
	ipv6Addrs, _ := net.LookupIP(domainName)
	for _, ip := range ipv6Addrs {
		if ip.To4() == nil {
			d.IPv6Addresses = append(d.IPv6Addresses, ip.String())
		}
	}
}

// lookupSSL fetches SSL certificate info
func (s *LookupService) lookupSSL(ctx context.Context, domainName string, d *domain.Domain) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", domainName+":443", &tls.Config{
		ServerName:         domainName,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close()

	cert := conn.ConnectionState().PeerCertificates[0]
	if cert != nil {
		if len(cert.Issuer.Organization) > 0 {
			d.SSLIssuer = cert.Issuer.Organization[0]
		}
		if len(cert.Issuer.Country) > 0 {
			d.SSLIssuerCountry = cert.Issuer.Country[0]
		}
		d.SSLValidFrom = &cert.NotBefore
		d.SSLValidTo = &cert.NotAfter
		d.SSLSubject = cert.Subject.CommonName

		fingerprint := sha256.Sum256(cert.Raw)
		d.SSLFingerprint = strings.ToUpper(strings.Join(splitHex(hex.EncodeToString(fingerprint[:])), ":"))

		d.SSLSignatureAlgo = cert.SignatureAlgorithm.String()

		switch key := cert.PublicKey.(type) {
		case *rsa.PublicKey:
			d.SSLKeySize = key.N.BitLen()
		case *ecdsa.PublicKey:
			d.SSLKeySize = key.Curve.Params().BitSize
		default:
			d.SSLKeySize = 0
		}
	}
}

func splitHex(value string) []string {
	parts := make([]string, 0, len(value)/2)
	for i := 0; i < len(value); i += 2 {
		end := i + 2
		if end > len(value) {
			end = len(value)
		}
		parts = append(parts, value[i:end])
	}
	return parts
}

// lookupHost fetches host/geolocation info
func (s *LookupService) lookupHost(ip string, d *domain.Domain) {
	// Use ip-api.com (free, no auth required for non-commercial use)
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,regionName,city,lat,lon,isp,org,as", ip)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result struct {
		Status      string  `json:"status"`
		Message     string  `json:"message"`
		Country     string  `json:"country"`
		CountryCode string  `json:"countryCode"`
		Region      string  `json:"regionName"`
		City        string  `json:"city"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		ISP         string  `json:"isp"`
		Org         string  `json:"org"`
		AS          string  `json:"as"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	if result.Status == "success" {
		d.HostCountry = result.Country
		d.HostCountryCode = result.CountryCode
		d.HostRegion = result.Region
		d.HostCity = result.City
		d.HostLat = result.Lat
		d.HostLon = result.Lon
		d.HostISP = result.ISP
		d.HostOrg = result.Org
		d.HostAS = result.AS
	}
}

// applyWHOISData applies WHOIS data to domain struct
func (s *LookupService) applyWHOISData(d *domain.Domain, whois *domain.WHOISData) {
	d.DomainName = whois.DomainName
	d.Status = strings.Join(whois.Status, ", ")
	d.DNSSEC = whois.DNSSEC
	d.ExpiryDate = whois.Dates.ExpiryDate
	d.CreationDate = whois.Dates.CreationDate
	d.UpdatedDate = whois.Dates.UpdatedDate
	d.RegistrarName = whois.Registrar.Name
	d.RegistrarID = whois.Registrar.ID
	d.RegistrarURL = whois.Registrar.URL
	d.RegistryDomainID = whois.Registrar.RegistryDomainID
	d.DomainStatuses = whois.Status

	// Detect privacy protection from registrant name
	registrantLower := strings.ToLower(whois.Registrant.Name + " " + whois.Registrant.Organization)
	d.PrivacyEnabled = strings.Contains(registrantLower, "redacted") ||
		strings.Contains(registrantLower, "privacy") ||
		strings.Contains(registrantLower, "whoisguard") ||
		strings.Contains(registrantLower, "not disclosed") ||
		strings.Contains(registrantLower, "hidden") ||
		strings.Contains(registrantLower, "data protected") ||
		strings.Contains(registrantLower, "gdpr") ||
		strings.Contains(registrantLower, "data redacted")

	// Detect transfer lock from statuses
	for _, status := range whois.Status {
		statusLower := strings.ToLower(status)
		if strings.Contains(statusLower, "clienttransferprohibited") || strings.Contains(statusLower, "servertransferprohibited") {
			d.TransferLock = true
			break
		}
	}

	// Apply registrant contact info if available
	if whois.Registrant.Name != "" || whois.Registrant.Organization != "" {
		d.RegistrantName = whois.Registrant.Name
		d.RegistrantOrg = whois.Registrant.Organization
		d.RegistrantStreet = whois.Registrant.Street
		d.RegistrantCity = whois.Registrant.City
		d.RegistrantState = whois.Registrant.State
		d.RegistrantCountry = whois.Registrant.Country
		d.RegistrantPostal = whois.Registrant.PostalCode
	}

	// Apply abuse contact info
	if whois.Abuse.Email != "" || whois.Abuse.Phone != "" {
		d.AbuseEmail = whois.Abuse.Email
		d.AbusePhone = whois.Abuse.Phone
	}
}

// cleanDomain cleans and normalizes a domain name
func cleanDomain(domain string) string {
	// Remove protocol
	domain = regexp.MustCompile(`^https?://`).ReplaceAllString(domain, "")
	// Remove www prefix
	domain = regexp.MustCompile(`^www\.`).ReplaceAllString(domain, "")
	// Remove path and query
	if idx := strings.IndexAny(domain, "/?#"); idx != -1 {
		domain = domain[:idx]
	}
	// Remove port
	if idx := strings.Index(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}
	return strings.ToLower(strings.TrimSpace(domain))
}

// lookupCertificateChain fetches the full TLS certificate chain
func (s *LookupService) lookupCertificateChain(ctx context.Context, domainName string, d *domain.Domain) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", domainName+":443", &tls.Config{
		ServerName:         domainName,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	for i, cert := range certs {
		issuer := ""
		if len(cert.Issuer.Organization) > 0 {
			issuer = cert.Issuer.Organization[0]
		} else if cert.Issuer.CommonName != "" {
			issuer = cert.Issuer.CommonName
		}

		altNames := make([]string, 0, len(cert.DNSNames)+len(cert.IPAddresses)+len(cert.EmailAddresses))
		altNames = append(altNames, cert.DNSNames...)
		for _, ip := range cert.IPAddresses {
			altNames = append(altNames, ip.String())
		}
		for _, email := range cert.EmailAddresses {
			altNames = append(altNames, email)
		}

		// For leaf cert, also set legacy SSL fields
		if i == 0 {
			if len(cert.Issuer.Organization) > 0 {
				d.SSLIssuer = cert.Issuer.Organization[0]
			}
			if len(cert.Issuer.Country) > 0 {
				d.SSLIssuerCountry = cert.Issuer.Country[0]
			}
			d.SSLValidFrom = &cert.NotBefore
			d.SSLValidTo = &cert.NotAfter
			d.SSLSubject = cert.Subject.CommonName

			fingerprint := sha256.Sum256(cert.Raw)
			d.SSLFingerprint = strings.ToUpper(strings.Join(splitHex(hex.EncodeToString(fingerprint[:])), ":"))

			d.SSLSignatureAlgo = cert.SignatureAlgorithm.String()

			switch key := cert.PublicKey.(type) {
			case *rsa.PublicKey:
				d.SSLKeySize = key.N.BitLen()
			case *ecdsa.PublicKey:
				d.SSLKeySize = key.Curve.Params().BitSize
			default:
				d.SSLKeySize = 0
			}
		}

		caProvider := detect.DetectCertificateAuthority(issuer)
		d.Certificates = append(d.Certificates, domain.Certificate{
			Issuer:     issuer,
			Subject:    cert.Subject.CommonName,
			AltNames:   altNames,
			ValidFrom:  cert.NotBefore,
			ValidTo:    cert.NotAfter,
			CAProvider: caProvider,
		})
	}

	// Set top-level CA provider from the chain
	if len(d.Certificates) > 0 {
		d.CAProvider = d.Certificates[len(d.Certificates)-1].CAProvider
	}
}

// lookupHeaders fetches HTTP response headers
func (s *LookupService) lookupHeaders(ctx context.Context, domainName string, d *domain.Domain) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	url := "https://" + domainName
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Beszel/1.0; +https://github.com/henrygd/beszel)")

	resp, err := client.Do(req)
	if err != nil {
		// Try HTTP fallback
		req, err = http.NewRequestWithContext(ctx, "HEAD", "http://"+domainName, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Beszel/1.0; +https://github.com/henrygd/beszel)")
		resp, err = client.Do(req)
		if err != nil {
			return
		}
	}
	defer resp.Body.Close()

	for name, values := range resp.Header {
		for _, value := range values {
			d.Headers = append(d.Headers, domain.Header{
				Name:  strings.ToLower(name),
				Value: value,
			})
		}
	}
}

// lookupSEO fetches and parses SEO metadata
func (s *LookupService) lookupSEO(ctx context.Context, domainName string, d *domain.Domain) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Fetch HTML
	url := "https://" + domainName
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Limit reading to avoid large responses
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return
	}

	html := string(body)
	seo := &domain.SEOMeta{
		OpenGraph: domain.OpenGraphMeta{},
		Twitter:   domain.TwitterMeta{},
		General:   domain.GeneralMeta{},
		Robots:    domain.RobotsTxt{Fetched: false, Groups: []domain.RobotsGroup{}, Sitemaps: []string{}},
	}

	// Parse general meta tags
	seo.General.Title = extractMetaTag(html, "title")
	seo.General.Description = extractMetaTag(html, "description")
	seo.General.Author = extractMetaTag(html, "author")
	seo.General.Robots = extractMetaTag(html, "robots")
	seo.General.Keywords = extractMetaTag(html, "keywords")
	seo.General.Canonical = extractLinkRel(html, "canonical")

	// Parse Open Graph
	seo.OpenGraph.URL = extractMetaProperty(html, "og:url")
	seo.OpenGraph.Type = extractMetaProperty(html, "og:type")
	seo.OpenGraph.Title = extractMetaProperty(html, "og:title")
	seo.OpenGraph.Description = extractMetaProperty(html, "og:description")
	seo.OpenGraph.Images = extractMetaProperties(html, "og:image")

	// Parse Twitter
	seo.Twitter.Title = extractMetaProperty(html, "twitter:title")
	seo.Twitter.Description = extractMetaProperty(html, "twitter:description")
	seo.Twitter.Image = extractMetaProperty(html, "twitter:image")
	seo.Twitter.Card = extractMetaProperty(html, "twitter:card")

	// Fetch robots.txt
	robotsURL := url + "/robots.txt"
	robotsReq, err := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	if err == nil {
		robotsResp, err := client.Do(robotsReq)
		if err == nil && robotsResp.StatusCode >= 200 && robotsResp.StatusCode < 300 {
			robotsBody, err := io.ReadAll(io.LimitReader(robotsResp.Body, 256*1024))
			robotsResp.Body.Close()
			if err == nil {
				seo.Robots = parseRobotsTxt(string(robotsBody))
			}
		} else if robotsResp != nil {
			robotsResp.Body.Close()
		}
	}

	d.SEOMeta = seo
}

// extractMetaTag extracts a meta tag by name attribute
func extractMetaTag(html, name string) string {
	// Match <meta name="xxx" content="yyy"> or <meta name='xxx' content='yyy'>
	re := regexp.MustCompile(`(?i)<meta\s+name=["']` + regexp.QuoteMeta(name) + `["']\s+content=["']([^"']*)["']`)
	match := re.FindStringSubmatch(html)
	if len(match) > 1 {
		return match[1]
	}
	// Try reverse order
	re = regexp.MustCompile(`(?i)<meta\s+content=["']([^"']*)["']\s+name=["']` + regexp.QuoteMeta(name) + `["']`)
	match = re.FindStringSubmatch(html)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// extractMetaProperty extracts a meta tag by property attribute
func extractMetaProperty(html, prop string) string {
	re := regexp.MustCompile(`(?i)<meta\s+property=["']` + regexp.QuoteMeta(prop) + `["']\s+content=["']([^"']*)["']`)
	match := re.FindStringSubmatch(html)
	if len(match) > 1 {
		return match[1]
	}
	// Try reverse order
	re = regexp.MustCompile(`(?i)<meta\s+content=["']([^"']*)["']\s+property=["']` + regexp.QuoteMeta(prop) + `["']`)
	match = re.FindStringSubmatch(html)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// extractMetaProperties extracts all meta tags matching a property prefix
func extractMetaProperties(html, prop string) []string {
	re := regexp.MustCompile(`(?i)<meta\s+property=["']` + regexp.QuoteMeta(prop) + `["']\s+content=["']([^"']*)["']`)
	matches := re.FindAllStringSubmatch(html, -1)
	var results []string
	for _, match := range matches {
		if len(match) > 1 {
			results = append(results, match[1])
		}
	}
	return results
}

// extractLinkRel extracts a link rel href value
func extractLinkRel(html, rel string) string {
	re := regexp.MustCompile(`(?i)<link\s+rel=["']` + regexp.QuoteMeta(rel) + `["']\s+href=["']([^"']*)["']`)
	match := re.FindStringSubmatch(html)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// parseRobotsTxt parses robots.txt content
func parseRobotsTxt(content string) domain.RobotsTxt {
	result := domain.RobotsTxt{
		Fetched:  true,
		Groups:   []domain.RobotsGroup{},
		Sitemaps: []string{},
	}

	lines := strings.Split(content, "\n")
	var currentGroup *domain.RobotsGroup

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "user-agent":
			if currentGroup != nil {
				result.Groups = append(result.Groups, *currentGroup)
			}
			currentGroup = &domain.RobotsGroup{
				UserAgents: []string{value},
				Rules:      []domain.RobotsRule{},
			}
		case "allow", "disallow":
			if currentGroup == nil {
				currentGroup = &domain.RobotsGroup{
					UserAgents: []string{"*"},
					Rules:      []domain.RobotsRule{},
				}
			}
			currentGroup.Rules = append(currentGroup.Rules, domain.RobotsRule{
				Type:  key,
				Value: value,
			})
		case "sitemap":
			result.Sitemaps = append(result.Sitemaps, value)
		}
	}

	if currentGroup != nil {
		result.Groups = append(result.Groups, *currentGroup)
	}

	return result
}

// detectProviders detects DNS, hosting, email, and CA providers
func (s *LookupService) detectProviders(d *domain.Domain) {
	d.DNSProvider = detect.DetectDNSProvider(d.NameServers)
	d.EmailProvider = detect.DetectEmailProvider(d.MXRecords)

	if len(d.Headers) > 0 {
		headerMap := make(http.Header)
		for _, h := range d.Headers {
			headerMap.Add(h.Name, h.Value)
		}
		d.HostingProvider = detect.DetectHostingProvider(headerMap)
	}
}

// parseDatePtr parses "2006-01-02" into a *time.Time, nil on failure or empty
func parseDatePtr(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.IsZero() {
		return nil
	}
	return &t
}

// hasExpiry checks if WHOIS data contains a non-zero expiry date
func hasExpiry(data *domain.WHOISData) bool {
	return data.Dates.ExpiryDate != nil && !data.Dates.ExpiryDate.IsZero()
}

// hasValidData checks if WHOIS data has useful parsed fields
func hasValidData(data *domain.WHOISData) bool {
	if data == nil {
		return false
	}
	// Accept if we got any meaningful date (non-nil and not zero)
	if data.Dates.ExpiryDate != nil && !data.Dates.ExpiryDate.IsZero() {
		return true
	}
	if data.Dates.CreationDate != nil && !data.Dates.CreationDate.IsZero() {
		return true
	}
	if data.Registrar.Name != "" && data.Registrar.Name != "Unknown" {
		return true
	}
	if len(data.Status) > 0 {
		return true
	}
	return false
}
