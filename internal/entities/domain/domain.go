package domain

import "time"

// Domain represents a tracked domain with WHOIS and DNS information
type Domain struct {
	ID         string `json:"id" db:"id"`
	DomainName string `json:"domain_name" db:"domain_name"`
	Status     string `json:"status" db:"status"`
	Active     bool   `json:"active" db:"active"`
	// Availability is "available" | "registered" | "" (unknown) — set from WHOIS/RDAP
	Availability string `json:"availability" db:"availability"`

	// Dates
	ExpiryDate   *time.Time `json:"expiry_date" db:"expiry_date"`
	CreationDate *time.Time `json:"creation_date" db:"creation_date"`
	UpdatedDate  *time.Time `json:"updated_date" db:"updated_date"`

	// Registrar
	RegistrarName    string `json:"registrar_name" db:"registrar_name"`
	RegistrarID      string `json:"registrar_id" db:"registrar_id"`
	RegistrarURL     string `json:"registrar_url" db:"registrar_url"`
	RegistryDomainID string `json:"registry_domain_id" db:"registry_domain_id"`

	// DNS
	DNSSEC      string   `json:"dnssec" db:"dnssec"`
	NameServers []string `json:"name_servers" db:"name_servers"`
	MXRecords   []string `json:"mx_records" db:"mx_records"`
	TXTRecords  []string `json:"txt_records" db:"txt_records"`
	CNAMERecord string   `json:"cname_record" db:"cname_record"`
	SRVRecords  []string `json:"srv_records" db:"srv_records"`

	// IP Addresses
	IPv4Addresses []string `json:"ipv4_addresses" db:"ipv4_addresses"`
	IPv6Addresses []string `json:"ipv6_addresses" db:"ipv6_addresses"`

	// SSL Certificate
	SSLIssuer        string     `json:"ssl_issuer" db:"ssl_issuer"`
	SSLIssuerCountry string     `json:"ssl_issuer_country" db:"ssl_issuer_country"`
	SSLValidFrom     *time.Time `json:"ssl_valid_from" db:"ssl_valid_from"`
	SSLValidTo       *time.Time `json:"ssl_valid_to" db:"ssl_valid_to"`
	SSLSubject       string     `json:"ssl_subject" db:"ssl_subject"`
	SSLFingerprint   string     `json:"ssl_fingerprint" db:"ssl_fingerprint"`
	SSLKeySize       int        `json:"ssl_key_size" db:"ssl_key_size"`
	SSLSignatureAlgo string     `json:"ssl_signature_algo" db:"ssl_signature_algo"`

	// Host Info
	HostCountry string  `json:"host_country" db:"host_country"`
	HostRegion  string  `json:"host_region" db:"host_region"`
	HostCity    string  `json:"host_city" db:"host_city"`
	HostISP     string  `json:"host_isp" db:"host_isp"`
	HostOrg     string  `json:"host_org" db:"host_org"`
	HostAS      string  `json:"host_as" db:"host_as"`
	HostLat     float64 `json:"host_lat" db:"host_lat"`
	HostLon     float64 `json:"host_lon" db:"host_lon"`

	// Valuation
	PurchasePrice float64 `json:"purchase_price" db:"purchase_price"`
	CurrentValue  float64 `json:"current_value" db:"current_value"`
	RenewalCost   float64 `json:"renewal_cost" db:"renewal_cost"`
	AutoRenew     bool    `json:"auto_renew" db:"auto_renew"`

	// Registrant Contact (from WHOIS)
	RegistrantName    string `json:"registrant_name" db:"registrant_name"`
	RegistrantOrg     string `json:"registrant_org" db:"registrant_org"`
	RegistrantStreet  string `json:"registrant_street" db:"registrant_street"`
	RegistrantCity    string `json:"registrant_city" db:"registrant_city"`
	RegistrantState   string `json:"registrant_state" db:"registrant_state"`
	RegistrantCountry string `json:"registrant_country" db:"registrant_country"`
	RegistrantPostal  string `json:"registrant_postal" db:"registrant_postal"`

	// Abuse Contact (from WHOIS)
	AbuseEmail string `json:"abuse_email" db:"abuse_email"`
	AbusePhone string `json:"abuse_phone" db:"abuse_phone"`

	// Provider Detection
	DNSProvider     string `json:"dns_provider" db:"dns_provider"`
	HostingProvider string `json:"hosting_provider" db:"hosting_provider"`
	EmailProvider   string `json:"email_provider" db:"email_provider"`
	CAProvider      string `json:"ca_provider" db:"ca_provider"`

	// HTTP Headers
	Headers []Header `json:"headers" db:"headers"`

	// Certificate Chain
	Certificates []Certificate `json:"certificates" db:"certificates"`

	// SEO Metadata
	SEOMeta *SEOMeta `json:"seo_meta" db:"seo_meta"`

	// Raw WHOIS Response
	WHOISRaw string `json:"whois_raw" db:"whois_raw"`

	// Registration Details
	PrivacyEnabled bool     `json:"privacy_enabled" db:"privacy_enabled"`
	TransferLock   bool     `json:"transfer_lock" db:"transfer_lock"`
	TLD            string   `json:"tld" db:"tld"`
	DomainStatuses []string `json:"domain_statuses" db:"domain_statuses"`

	// Enhanced Geo
	HostCountryCode string `json:"host_country_code" db:"host_country_code"`

	// Metadata
	Tags       []string `json:"tags" db:"tags"`
	Notes      string   `json:"notes" db:"notes"`
	FaviconURL string   `json:"favicon_url" db:"favicon_url"`

	// Ownership
	UserID      string    `json:"user" db:"user"`
	Created     time.Time `json:"created" db:"created"`
	Updated     time.Time `json:"updated" db:"updated"`
	LastChecked time.Time `json:"last_checked" db:"last_checked"`

	// Alert settings
	AlertDaysBefore int  `json:"alert_days_before" db:"alert_days_before"` // Days before expiry to alert
	SSLAlertEnabled bool `json:"ssl_alert_enabled" db:"ssl_alert_enabled"`
}

// DomainHistory tracks changes to a domain over time
type DomainHistory struct {
	ID         string    `json:"id" db:"id"`
	DomainID   string    `json:"domain" db:"domain"`
	ChangeType string    `json:"change_type" db:"change_type"` // expiry, ssl, dns, registrar, ip, etc.
	FieldName  string    `json:"field_name" db:"field_name"`
	OldValue   string    `json:"old_value" db:"old_value"`
	NewValue   string    `json:"new_value" db:"new_value"`
	UserID     string    `json:"user" db:"user"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// WHOISData represents parsed WHOIS information
type WHOISData struct {
	DomainName string         `json:"domain_name"`
	Available  bool           `json:"available"`
	Status     []string       `json:"status"`
	DNSSEC     string         `json:"dnssec"`
	Dates      WHOISDates     `json:"dates"`
	Registrar  WHOISRegistrar `json:"registrar"`
	Registrant WHOISContact   `json:"registrant"`
	Abuse      WHOISAbuse     `json:"abuse"`
}

type WHOISDates struct {
	ExpiryDate   *time.Time `json:"expiry_date"`
	CreationDate *time.Time `json:"creation_date"`
	UpdatedDate  *time.Time `json:"updated_date"`
}

type WHOISRegistrar struct {
	Name             string `json:"name"`
	ID               string `json:"id"`
	URL              string `json:"url"`
	RegistryDomainID string `json:"registry_domain_id"`
}

type WHOISContact struct {
	Name         string `json:"name"`
	Organization string `json:"organization"`
	Street       string `json:"street"`
	City         string `json:"city"`
	State        string `json:"state"`
	Country      string `json:"country"`
	PostalCode   string `json:"postal_code"`
}

type WHOISAbuse struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// SSLInfo represents SSL certificate information
type SSLInfo struct {
	Issuer        string    `json:"issuer"`
	IssuerCountry string    `json:"issuer_country"`
	ValidFrom     time.Time `json:"valid_from"`
	ValidTo       time.Time `json:"valid_to"`
	Subject       string    `json:"subject"`
	Fingerprint   string    `json:"fingerprint"`
	KeySize       int       `json:"key_size"`
	SignatureAlgo string    `json:"signature_algo"`
}

// DNSInfo represents DNS records
type DNSInfo struct {
	NameServers []string `json:"name_servers"`
	MXRecords   []string `json:"mx_records"`
	TXTRecords  []string `json:"txt_records"`
	DNSSEC      string   `json:"dnssec"`
}

// HostInfo represents host/geolocation information
type HostInfo struct {
	Country  string  `json:"country"`
	Region   string  `json:"region"`
	City     string  `json:"city"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	ISP      string  `json:"isp"`
	Org      string  `json:"org"`
	ASNumber string  `json:"as_number"`
}

// IPInfo represents IP address information
type IPInfo struct {
	IPv4 []string `json:"ipv4"`
	IPv6 []string `json:"ipv6"`
}

// Header represents an HTTP response header
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Certificate represents a TLS certificate in the chain
type Certificate struct {
	Issuer     string    `json:"issuer"`
	Subject    string    `json:"subject"`
	AltNames   []string  `json:"alt_names"`
	ValidFrom  time.Time `json:"valid_from"`
	ValidTo    time.Time `json:"valid_to"`
	CAProvider string    `json:"ca_provider"`
}

// OpenGraphMeta represents Open Graph metadata
type OpenGraphMeta struct {
	URL         string   `json:"url"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Images      []string `json:"images"`
	Description string   `json:"description"`
}

// TwitterMeta represents Twitter card metadata
type TwitterMeta struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Card        string `json:"card"`
}

// GeneralMeta represents general HTML meta tags
type GeneralMeta struct {
	Title       string `json:"title"`
	Author      string `json:"author"`
	Robots      string `json:"robots"`
	Keywords    string `json:"keywords"`
	Canonical   string `json:"canonical"`
	Description string `json:"description"`
}

// RobotsTxt represents parsed robots.txt data
type RobotsTxt struct {
	Fetched  bool          `json:"fetched"`
	Groups   []RobotsGroup `json:"groups"`
	Sitemaps []string      `json:"sitemaps"`
}

// RobotsGroup represents a user-agent group in robots.txt
type RobotsGroup struct {
	UserAgents []string     `json:"userAgents"`
	Rules      []RobotsRule `json:"rules"`
}

// RobotsRule represents a single rule in robots.txt
type RobotsRule struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// SEOMeta represents all SEO-related metadata
type SEOMeta struct {
	OpenGraph OpenGraphMeta `json:"openGraph"`
	Twitter   TwitterMeta   `json:"twitter"`
	General   GeneralMeta   `json:"general"`
	Robots    RobotsTxt     `json:"robots"`
}

// ChangeType constants for domain history
const (
	ChangeTypeExpiry    = "expiry"
	ChangeTypeSSL       = "ssl"
	ChangeTypeDNS       = "dns"
	ChangeTypeRegistrar = "registrar"
	ChangeTypeIP        = "ip"
	ChangeTypeHost      = "host"
	ChangeTypeStatus    = "status"
	ChangeTypeProvider  = "provider"
	ChangeTypeSecurity  = "security"
	ChangeTypeSEO       = "seo"
)

// Domain status constants
const (
	DomainStatusActive   = "active"
	DomainStatusExpiring = "expiring" // Within alert_days_before
	DomainStatusExpired  = "expired"
	DomainStatusUnknown  = "unknown"
	DomainStatusPaused   = "paused"
)

// IsExpiring returns true if the domain is expiring within the alert window
func (d *Domain) IsExpiring() bool {
	if d.ExpiryDate == nil || d.AlertDaysBefore <= 0 {
		return false
	}
	alertDate := time.Now().AddDate(0, 0, d.AlertDaysBefore)
	return d.ExpiryDate.Before(alertDate) && d.ExpiryDate.After(time.Now())
}

// IsExpired returns true if the domain has expired
func (d *Domain) IsExpired() bool {
	if d.ExpiryDate == nil {
		return false
	}
	return d.ExpiryDate.Before(time.Now())
}

// DaysUntilExpiry returns the number of days until expiry
func (d *Domain) DaysUntilExpiry() int {
	if d.ExpiryDate == nil {
		return -1
	}
	return int(time.Until(*d.ExpiryDate).Hours() / 24)
}

// SSLDaysUntilExpiry returns days until SSL expiry
func (d *Domain) SSLDaysUntilExpiry() int {
	if d.SSLValidTo == nil {
		return -1
	}
	return int(time.Until(*d.SSLValidTo).Hours() / 24)
}

// GetStatus returns the current domain status
func (d *Domain) GetStatus() string {
	if !d.Active {
		return DomainStatusPaused
	}
	if d.IsExpired() {
		return DomainStatusExpired
	}
	if d.IsExpiring() {
		return DomainStatusExpiring
	}
	if d.ExpiryDate == nil {
		return DomainStatusUnknown
	}
	return DomainStatusActive
}
