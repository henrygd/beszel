package domains

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// SubdomainDiscovery handles advanced subdomain discovery
type SubdomainDiscovery struct {
	app      core.App
	client   *http.Client
	wordlist []string
}

// NewSubdomainDiscovery creates a new subdomain discovery service
func NewSubdomainDiscovery(app core.App) *SubdomainDiscovery {
	return &SubdomainDiscovery{
		app: app,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		wordlist: getEnhancedWordlist(),
	}
}

// DiscoveryResult represents a discovered subdomain
type DiscoveryResult struct {
	Subdomain   string    `json:"subdomain"`
	FullDomain  string    `json:"full_domain"`
	IPAddresses []string  `json:"ip_addresses"`
	StatusCode  int       `json:"status_code,omitempty"`
	Server      string    `json:"server,omitempty"`
	Source      string    `json:"source"` // dns, certificate, http, etc.
	FoundAt     time.Time `json:"found_at"`
}

// Discover performs comprehensive subdomain discovery
func (sd *SubdomainDiscovery) Discover(ctx context.Context, domainName string) ([]DiscoveryResult, error) {
	var results []DiscoveryResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a channel for results
	resultChan := make(chan DiscoveryResult, 100)

	// Start multiple discovery methods concurrently
	wg.Add(4)

	// 1. DNS brute force with enhanced wordlist
	go func() {
		defer wg.Done()
		sd.dnsBruteForce(ctx, domainName, resultChan)
	}()

	// 2. Certificate transparency log search
	go func() {
		defer wg.Done()
		sd.ctLogSearch(ctx, domainName, resultChan)
	}()

	// 3. DNS enumeration via common patterns
	go func() {
		defer wg.Done()
		sd.patternEnumeration(ctx, domainName, resultChan)
	}()

	// 4. HTTP probe for common subdomains
	go func() {
		defer wg.Done()
		sd.httpProbe(ctx, domainName, resultChan)
	}()

	// Collect results in a separate goroutine
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	seen := make(map[string]bool)
	for result := range resultChan {
		if seen[result.Subdomain] {
			continue
		}
		seen[result.Subdomain] = true

		mu.Lock()
		results = append(results, result)
		mu.Unlock()
	}

	return results, nil
}

// dnsBruteForce performs DNS brute forcing with wordlist
func (sd *SubdomainDiscovery) dnsBruteForce(ctx context.Context, domainName string, results chan<- DiscoveryResult) {
	semaphore := make(chan struct{}, 20) // Limit concurrency
	var wg sync.WaitGroup

	for _, word := range sd.wordlist {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(word string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			subdomain := word + "." + domainName
			ips, err := net.LookupHost(subdomain)
			if err != nil || len(ips) == 0 {
				return
			}

			results <- DiscoveryResult{
				Subdomain:   word,
				FullDomain:  subdomain,
				IPAddresses: ips,
				Source:      "dns",
				FoundAt:     time.Now(),
			}
		}(word)
	}

	wg.Wait()
}

// ctLogSearch searches certificate transparency logs via crt.sh
func (sd *SubdomainDiscovery) ctLogSearch(ctx context.Context, domainName string, results chan<- DiscoveryResult) {
	// Query crt.sh for certificates
	url := fmt.Sprintf("https://crt.sh/?q=%%.%s&output=json", domainName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("[subdomain-discovery] CT log search failed for %s: %v", domainName, err)
		return
	}

	resp, err := sd.client.Do(req)
	if err != nil {
		log.Printf("[subdomain-discovery] CT log search failed for %s: %v", domainName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[subdomain-discovery] CT log search returned status %d for %s", resp.StatusCode, domainName)
		return
	}

	// Parse crt.sh JSON response
	var entries []struct {
		NameValue string `json:"name_value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		log.Printf("[subdomain-discovery] Failed to parse CT log response for %s: %v", domainName, err)
		return
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		// crt.sh returns one name_value per line, may contain wildcards or multiple names
		names := strings.Split(entry.NameValue, "\n")
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" || name == domainName {
				continue
			}
			// Remove wildcard prefix
			name = strings.TrimPrefix(name, "*.")
			// Only include subdomains of the target domain
			if !strings.HasSuffix(name, "."+domainName) {
				continue
			}
			subdomain := strings.TrimSuffix(name, "."+domainName)
			if subdomain == "" || seen[subdomain] {
				continue
			}
			seen[subdomain] = true

			// Try to resolve IPs
			ips, _ := net.LookupHost(name)

			results <- DiscoveryResult{
				Subdomain:   subdomain,
				FullDomain:  name,
				IPAddresses: ips,
				Source:      "certificate",
				FoundAt:     time.Now(),
			}
		}
	}

	log.Printf("[subdomain-discovery] CT log search found %d unique subdomains for %s", len(seen), domainName)
}

// patternEnumeration enumerates common subdomain patterns
func (sd *SubdomainDiscovery) patternEnumeration(ctx context.Context, domainName string, results chan<- DiscoveryResult) {
	patterns := []string{
		"api-v1", "api-v2", "api-v3", "api-dev", "api-staging", "api-prod",
		"app-dev", "app-staging", "app-prod", "web-dev", "web-staging",
		"admin-dev", "admin-staging", "admin-prod",
		"portal-dev", "portal-staging", "portal-prod",
		"dashboard-dev", "dashboard-staging", "dashboard-prod",
		"service-1", "service-2", "service-3",
		"node-1", "node-2", "node-3",
		"server-1", "server-2", "server-3",
		"web-01", "web-02", "web-03",
		"app-01", "app-02", "app-03",
		"us-east", "us-west", "eu-west", "eu-central", "ap-south", "ap-northeast",
	}

	semaphore := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, pattern := range patterns {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(pattern string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			subdomain := pattern + "." + domainName
			ips, err := net.LookupHost(subdomain)
			if err != nil || len(ips) == 0 {
				return
			}

			results <- DiscoveryResult{
				Subdomain:   pattern,
				FullDomain:  subdomain,
				IPAddresses: ips,
				Source:      "pattern",
				FoundAt:     time.Now(),
			}
		}(pattern)
	}

	wg.Wait()
}

// httpProbe probes common subdomains via HTTP
func (sd *SubdomainDiscovery) httpProbe(ctx context.Context, domainName string, results chan<- DiscoveryResult) {
	commonWebSubdomains := []string{"www", "app", "api", "admin", "dashboard", "portal", "web"}

	semaphore := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, sub := range commonWebSubdomains {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(sub string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Try HTTPS first
			url := fmt.Sprintf("https://%s.%s", sub, domainName)
			resp, err := sd.client.Get(url)
			if err != nil {
				// Try HTTP
				url = fmt.Sprintf("http://%s.%s", sub, domainName)
				resp, err = sd.client.Get(url)
				if err != nil {
					return
				}
			}
			defer resp.Body.Close()

			// Get IP addresses
			subdomain := sub + "." + domainName
			ips, _ := net.LookupHost(subdomain)

			results <- DiscoveryResult{
				Subdomain:   sub,
				FullDomain:  subdomain,
				IPAddresses: ips,
				StatusCode:  resp.StatusCode,
				Server:      resp.Header.Get("Server"),
				Source:      "http",
				FoundAt:     time.Now(),
			}
		}(sub)
	}

	wg.Wait()
}

// SaveSubdomains saves discovered subdomains to the database
func (sd *SubdomainDiscovery) SaveSubdomains(domainRecord *core.Record, results []DiscoveryResult, userID string) error {
	collection, err := sd.app.FindCollectionByNameOrId("subdomains")
	if err != nil {
		return err
	}

	// Get existing subdomains to avoid duplicates
	existing, _ := sd.app.FindAllRecords("subdomains",
		dbx.NewExp("domain = {:domain}", dbx.Params{"domain": domainRecord.Id}),
	)
	existingMap := make(map[string]bool)
	for _, sub := range existing {
		existingMap[sub.GetString("subdomain_name")] = true
	}

	for _, result := range results {
		if existingMap[result.Subdomain] {
			continue
		}

		record := core.NewRecord(collection)
		record.Set("domain", domainRecord.Id)
		record.Set("subdomain_name", result.Subdomain)
		record.Set("full_domain", result.FullDomain)
		record.Set("status", "active")
		record.Set("ip_addresses", strings.Join(result.IPAddresses, ","))
		record.Set("discovery_source", result.Source)
		record.Set("last_checked", time.Now())
		record.Set("user", userID)

		if result.StatusCode > 0 {
			record.Set("http_status", result.StatusCode)
		}
		if result.Server != "" {
			record.Set("server_header", result.Server)
		}

		if err := sd.app.Save(record); err != nil {
			log.Printf("[subdomain-discovery] Failed to save subdomain %s: %v", result.FullDomain, err)
		} else {
			log.Printf("[subdomain-discovery] Saved subdomain: %s (source: %s)", result.FullDomain, result.Source)
		}
	}

	return nil
}

// getEnhancedWordlist returns an enhanced wordlist for subdomain discovery
func getEnhancedWordlist() []string {
	// Comprehensive wordlist including common subdomains
	wordlist := []string{
		// Common web
		"www", "mail", "ftp", "localhost", "admin", "dashboard", "portal",
		"api", "app", "mobile", "dev", "test", "staging", "demo", "beta",
		"prod", "production", "live", "www2", "www1", "blog", "shop", "store",
		"support", "help", "docs", "documentation", "wiki", "forum", "community",
		"news", "media", "cdn", "static", "assets", "files", "download", "upload",
		"images", "img", "css", "js", "scripts", "resources", "public", "private",

		// Services
		"api-v1", "api-v2", "api-v3", "api-dev", "api-staging", "api-prod",
		"graphql", "rest", "grpc", "websocket", "socket", "ws", "wss",
		"oauth", "auth", "login", "signin", "signup", "register", "sso",
		"saml", "oidc", "openid", "keycloak", "auth0", "cognito", "okta",

		// Infrastructure
		"server", "server1", "server2", "server3", "srv", "srv1", "srv2",
		"node", "node1", "node2", "node3", "worker", "worker1", "worker2",
		"web", "web1", "web2", "web3", "app1", "app2", "app3",
		"db", "database", "mysql", "postgres", "mongodb", "redis", "elasticsearch",
		"cache", "queue", "rabbitmq", "kafka", "zookeeper", "etcd",

		// Cloud & DevOps
		"k8s", "kubernetes", "docker", "swarm", "nomad", "consul", "vault",
		"terraform", "ansible", "puppet", "chef", "salt", "jenkins", "gitlab",
		"github", "bitbucket", "travis", "circleci", "drone", "argo", "spinnaker",
		"prometheus", "grafana", "alertmanager", "thanos", "loki", "tempo",
		"jaeger", "zipkin", "kibana", "elk", "efk", "splunk", "datadog",

		// Security
		"vpn", "ipsec", "openvpn", "wireguard", "fortinet", "paloalto",
		"firewall", "waf", "ids", "ips", "siem", "soc", "nids",
		"scan", "scanner", "security", "sec", "pentest", "vulnerability",

		// Monitoring & Logging
		"monitor", "monitoring", "status", "health", "healthcheck", "ping",
		"uptime", "metrics", "logs", "logging", "audit", "trace", "apm",
		"sentry", "bugsnag", "raygun", "rollbar", "airbrake", "honeybadger",
		"newrelic", "appdynamics", "dynatrace", "instana", "scout", "skylight",

		// Communication
		"mail", "email", "smtp", "pop", "imap", "exchange", "webmail",
		"mailgun", "sendgrid", "mailchimp", " SES", "postmark", "sparkpost",
		"chat", "slack", "teams", "discord", "zoom", "meet", "webex",
		"jitsi", "mattermost", "rocket", "element", "matrix",

		// Regions
		"us", "us-east", "us-west", "us-central", "us-south",
		"eu", "eu-west", "eu-central", "eu-east", "eu-north", "eu-south",
		"ap", "ap-south", "ap-northeast", "ap-southeast", "ap-east",
		"sa", "sa-east", "af", "af-south", "me", "me-south",

		// Environments
		"sandbox", "playground", "lab", "labs", "experiment", "canary",
		"blue", "green", "a", "b", "alpha", "beta", "gamma", "delta",
		"rc", "release", "nightly", "stable", "latest", "edge", "lts",

		// Corporate
		"corp", "corporate", "enterprise", "business", "company", "org",
		"intranet", "extranet", "partners", "vendors", "suppliers",
		"hr", "finance", "accounting", "legal", "compliance", "it",
		"sales", "marketing", "crm", "erp", "scm", "plm", "hrm",

		// Development tools
		"git", "svn", "cvs", "mercurial", "hg", "bitkeeper",
		"repo", "repository", "repos", "source", "code", "src",
		"build", "ci", "cd", "deploy", "deployment", "release",
		"artifact", "artifacts", "package", "packages", "registry",

		// Testing
		"qa", "qc", "test", "testing", "tests", "spec", "specs",
		"unit", "integration", "e2e", "acceptance", "regression",
		"mock", "stub", "fake", "dummy", "sandbox", "fixture",

		// Archive & Backup
		"archive", "archives", "backup", "backups", "snapshot", "snapshots",
		"old", "legacy", "deprecated", "retired", "historical",

		// Miscellaneous
		"search", "find", "lookup", "query", "browse", "explorer",
		"api-docs", "swagger", "openapi", "redoc", "postman", "graphql-playground",
		"status-page", "statuspage", "trust", "security", "compliance",
		"terms", "privacy", "legal", "about", "contact", "feedback",
		"careers", "jobs", "team", "people", "staff", "employees",
		"press", "media-kit", "brand", "assets", "styleguide", "design",
		"sitemap", "robots", "humans", "security", "pgp", "key",
		"invite", "join", "apply", "subscribe", "newsletter", "rss", "atom",
		"mobile", "m", "touch", "app", "ios", "android", "windows", "mac",
		"download", "downloads", "dl", "installer", "setup", "update", "upgrade",
		"payment", "pay", "billing", "invoice", "subscription", "checkout",
		"cart", "basket", "shop", "store", "buy", "purchase", "order",
		"account", "my", "profile", "user", "users", "member", "members",
		"session", "token", "auth", "verify", "validation", "confirm",
		"reset", "recovery", "forgot", "password", "pass", "pwd",
		"2fa", "mfa", "totp", "otp", "authenticator", "security-key",
		"webhook", "callback", "hook", "integration", "connector", "adapter",
		"plugin", "extension", "addon", "module", "component", "widget",
		"embed", "iframe", "frame", "proxy", "gateway", "ingress", "egress",
		"loadbalancer", "lb", "vip", "floating", "virtual", "ha", "failover",
		"primary", "secondary", "master", "slave", "leader", "follower",
		"active", "standby", "hot", "warm", "cold", "mirror", "replica",
		"shard", "shards", "partition", "partitions", "segment", "segments",
	}

	return wordlist
}
