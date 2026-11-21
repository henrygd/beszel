import { pb } from "./api"

export interface Subdomain {
	id: string
	domain: string
	subdomain_name: string
	full_domain: string
	status: "active" | "inactive" | "error"
	ip_addresses?: string
	http_status?: number
	server_header?: string
	discovery_source: string
	last_checked?: string
	created: string
	updated: string
}

export interface Domain {
	id: string
	domain_name: string
	status: "active" | "expiring" | "expired" | "unknown" | "paused"
	availability?: "available" | "registered" | ""
	active: boolean
	expiry_date?: string
	creation_date?: string
	updated_date?: string
	days_until_expiry?: number
	registrar_name?: string
	registrar_id?: string
	registrar_url?: string
	registry_domain_id?: string
	name_servers?: string[]
	mx_records?: string[]
	txt_records?: string[]
	cname_record?: string
	srv_records?: string[]
	ipv4_addresses?: string[]
	ipv6_addresses?: string[]
	dnssec?: string
	ssl_issuer?: string
	ssl_issuer_country?: string
	ssl_subject?: string
	ssl_valid_from?: string
	ssl_valid_to?: string
	ssl_days_until?: number
	ssl_key_size?: number
	ssl_signature_algo?: string
	ssl_fingerprint?: string
	host_country?: string
	host_region?: string
	host_city?: string
	host_isp?: string
	host_org?: string
	host_as?: string
	host_lat?: number
	host_lon?: number
	purchase_price?: number
	current_value?: number
	renewal_cost?: number
	auto_renew: boolean
	alert_days_before: number
	ssl_alert_enabled: boolean
	registrant_name?: string
	registrant_org?: string
	registrant_country?: string
	registrant_city?: string
	registrant_state?: string
	abuse_email?: string
	abuse_phone?: string
	monitor_type?: "expiry" | "watchlist" | "portfolio"
	ssl_alert_days?: number
	notify_on_expiry?: boolean
	notify_on_ssl_expiry?: boolean
	notify_on_dns_change?: boolean
	notify_on_registrar_change?: boolean
	notify_on_value_change?: boolean
	value_change_threshold?: number
	quiet_hours_enabled?: boolean
	quiet_hours_start?: string
	quiet_hours_end?: string
	tags?: string[]
	notes?: string
	favicon_url?: string
	last_checked?: string
	created: string
	updated: string
	whois_server?: string
	whois_updated?: string
	whois_status?: string
	whois_registrar?: string
	whois_registrant_name?: string
	whois_registrant_org?: string
	whois_registrant_country?: string
	whois_registrant_city?: string
	whois_registrant_state?: string
	whois_admin_name?: string
	whois_admin_org?: string
	whois_admin_country?: string
	whois_admin_city?: string
	whois_admin_state?: string
	whois_tech_name?: string
	whois_tech_org?: string
	whois_tech_country?: string
	whois_tech_city?: string
	whois_tech_state?: string
	ssl_subject_alt_names?: string[]
	ssl_san_count?: number
	ssl_cert_type?: string
	ssl_cert_issuer?: string
	ssl_cert_serial_number?: string
	ssl_cert_valid_from?: string
	ssl_cert_valid_to?: string
	ssl_cert_days_until?: number
	ssl_cert_key_size?: number
	ssl_cert_signature_algo?: string
	ssl_cert_fingerprint?: string
	dns_a_records?: string[]
	dns_aaaa_records?: string[]
	dns_ns_records?: string[]
	dns_mx_records?: string[]
	dns_txt_records?: string[]
	dns_spf_records?: string[]
	dns_dkim_records?: string[]
	dns_dmarc_records?: string[]

	// Provider Detection
	dns_provider?: string
	hosting_provider?: string
	email_provider?: string
	ca_provider?: string

	// HTTP Headers
	headers?: { name: string; value: string }[]

	// Certificate Chain
	certificates?: {
		issuer: string
		subject: string
		alt_names: string[]
		valid_from: string
		valid_to: string
		ca_provider: string
	}[]

	// SEO Metadata
	seo_meta?: {
		openGraph: {
			url: string
			type: string
			title: string
			images: string[]
			description: string
		}
		twitter: {
			title: string
			description: string
			image: string
			card: string
		}
		general: {
			title: string
			author: string
			robots: string
			keywords: string
			canonical: string
			description: string
		}
		robots: {
			fetched: boolean
			groups: {
				userAgents: string[]
				rules: { type: string; value: string }[]
			}[]
			sitemaps: string[]
		}
	}

	// Raw WHOIS & Registration Details
	whois_raw?: string
	privacy_enabled?: boolean
	transfer_lock?: boolean
	tld?: string
	domain_statuses?: string[]
	host_country_code?: string
}

export interface DomainHistory {
	id: string
	change_type: "expiry" | "ssl" | "dns" | "registrar" | "ip" | "host" | "status"
	field_name: string
	old_value: string
	new_value: string
	created_at: string
}

export interface CreateDomainRequest {
	domain_name: string
	auto_lookup?: boolean
	tags?: string[]
	notes?: string
	purchase_price?: number
	current_value?: number
	renewal_cost?: number
	auto_renew?: boolean
	alert_days_before?: number
	ssl_alert_enabled?: boolean
	ssl_alert_days?: number
	monitor_type?: "expiry" | "watchlist" | "portfolio"
	notify_on_expiry?: boolean
	notify_on_ssl_expiry?: boolean
	notify_on_dns_change?: boolean
	notify_on_registrar_change?: boolean
	notify_on_value_change?: boolean
	value_change_threshold?: number
	quiet_hours_enabled?: boolean
	quiet_hours_start?: string
	quiet_hours_end?: string
	// Manual expiry override when WHOIS fails
	expiry_date?: string
	creation_date?: string
}

export interface UpdateDomainRequest {
	tags?: string[]
	notes?: string
	purchase_price?: number
	current_value?: number
	renewal_cost?: number
	auto_renew?: boolean
	active?: boolean
	alert_days_before?: number
	ssl_alert_enabled?: boolean
	ssl_alert_days?: number
	monitor_type?: "expiry" | "watchlist" | "portfolio"
	notify_on_expiry?: boolean
	notify_on_ssl_expiry?: boolean
	notify_on_dns_change?: boolean
	notify_on_registrar_change?: boolean
	notify_on_value_change?: boolean
	value_change_threshold?: number
	quiet_hours_enabled?: boolean
	quiet_hours_start?: string
	quiet_hours_end?: string
}

export interface DomainLookupResult {
	domain_name: string
	status: string
	active: boolean
	expiry_date?: string
	creation_date?: string
	updated_date?: string
	registrar_name?: string
	registrar_id?: string
	registrar_url?: string
	dnssec?: string
	name_servers?: string[]
	mx_records?: string[]
	txt_records?: string[]
	ipv4_addresses?: string[]
	ipv6_addresses?: string[]
	ssl_issuer?: string
	ssl_valid_to?: string
	host_country?: string
	host_isp?: string
	favicon_url?: string
	last_checked?: string

	// Provider Detection
	dns_provider?: string
	hosting_provider?: string
	email_provider?: string
	ca_provider?: string

	// HTTP Headers
	headers?: { name: string; value: string }[]

	// Certificate Chain
	certificates?: {
		issuer: string
		subject: string
		alt_names: string[]
		valid_from: string
		valid_to: string
		ca_provider: string
	}[]

	// SEO Metadata
	seo_meta?: {
		openGraph: {
			url: string
			type: string
			title: string
			images: string[]
			description: string
		}
		twitter: {
			title: string
			description: string
			image: string
			card: string
		}
		general: {
			title: string
			author: string
			robots: string
			keywords: string
			canonical: string
			description: string
		}
		robots: {
			fetched: boolean
			groups: {
				userAgents: string[]
				rules: { type: string; value: string }[]
			}[]
			sitemaps: string[]
		}
	}

	// Raw WHOIS & Registration Details
	whois_raw?: string
	privacy_enabled?: boolean
	transfer_lock?: boolean
	tld?: string
	domain_statuses?: string[]
	host_country_code?: string
}

const API_BASE = "/api/beszel/domains"

export async function getDomains(): Promise<Domain[]> {
	const response = await fetch(API_BASE, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch domains: ${response.statusText}`)
	}
	return response.json()
}

export async function lookupDomain(domainName: string): Promise<DomainLookupResult> {
	const response = await fetch(`${API_BASE}/lookup`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify({ domain_name: domainName }),
	})
	if (!response.ok) {
		throw new Error(`Failed to lookup domain: ${response.statusText}`)
	}
	return response.json()
}

export async function createDomain(data: CreateDomainRequest): Promise<Domain> {
	const response = await fetch(API_BASE, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify(data),
	})
	if (!response.ok) {
		throw new Error(`Failed to create domain: ${response.statusText}`)
	}
	return response.json()
}

export async function getDomain(id: string): Promise<Domain> {
	const response = await fetch(`${API_BASE}/${id}`, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch domain: ${response.statusText}`)
	}
	return response.json()
}

export async function updateDomain(id: string, data: UpdateDomainRequest): Promise<Domain> {
	const response = await fetch(`${API_BASE}/${id}`, {
		method: "PATCH",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify(data),
	})
	if (!response.ok) {
		throw new Error(`Failed to update domain: ${response.statusText}`)
	}
	return response.json()
}

export async function deleteDomain(id: string): Promise<void> {
	const response = await fetch(`${API_BASE}/${id}`, {
		method: "DELETE",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to delete domain: ${response.statusText}`)
	}
}

export async function refreshDomain(id: string): Promise<Domain> {
	const response = await fetch(`${API_BASE}/${id}/refresh`, {
		method: "POST",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to refresh domain: ${response.statusText}`)
	}
	return response.json()
}

export async function getDomainHistory(id: string): Promise<DomainHistory[]> {
	const response = await fetch(`${API_BASE}/${id}/history`, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch domain history: ${response.statusText}`)
	}
	return response.json()
}

export function getDomainFaviconUrl(domainName: string): string {
	return `https://www.google.com/s2/favicons?domain=${domainName}&sz=128`
}

export function getStatusBadgeColor(status: string): string {
	switch (status) {
		case "active":
			return "bg-green-500"
		case "expiring":
			return "bg-yellow-500"
		case "expired":
			return "bg-red-500"
		case "unknown":
			return "bg-gray-500"
		case "paused":
			return "bg-blue-500"
		default:
			return "bg-gray-500"
	}
}

export function getStatusLabel(status: string): string {
	switch (status) {
		case "active":
			return "Active"
		case "expiring":
			return "Expiring Soon"
		case "expired":
			return "Expired"
		case "unknown":
			return "Unknown"
		case "paused":
			return "Paused"
		default:
			return status
	}
}

export function formatDate(dateString?: string): string {
	if (!dateString) return "Unknown"
	const date = new Date(dateString)
	return date.toLocaleDateString("en-US", {
		year: "numeric",
		month: "short",
		day: "numeric",
	})
}

export function formatDays(days?: number): string {
	if (days === undefined || days === null || days === -1) return "Unknown"
	if (days < 0) return "Expired"
	if (days === 0) return "Today"
	if (days === 1) return "1 day"
	if (days >= 365) {
		const years = Math.floor(days / 365)
		const remaining = days % 365
		if (remaining >= 30) {
			const months = Math.floor(remaining / 30)
			return `${years}y ${months}m`
		}
		return `${years}y`
	}
	if (days >= 30) {
		const months = Math.floor(days / 30)
		const remaining = days % 30
		if (remaining > 0) {
			return `${months}m ${remaining}d`
		}
		return `${months}m`
	}
	return `${days} days`
}

export function cleanDomain(domain: string): string {
	return domain
		.replace(/^https?:\/\//, "")
		.replace(/^www\./, "")
		.replace(/[/?#:].*$/, "")
		.toLowerCase()
		.trim()
}

// Subdomain API functions
export async function getDomainSubdomains(domainId: string): Promise<Subdomain[]> {
	const response = await fetch(`/api/beszel/domains/${domainId}/subdomains`, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch subdomains: ${response.statusText}`)
	}
	return response.json()
}

export async function refreshSubdomainDiscovery(domainId: string): Promise<void> {
	const response = await fetch(`/api/beszel/domains/${domainId}/discover-subdomains`, {
		method: "POST",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to start subdomain discovery: ${response.statusText}`)
	}
}

export async function deleteSubdomain(subdomainId: string): Promise<void> {
	const response = await fetch(`/api/beszel/subdomains/${subdomainId}`, {
		method: "DELETE",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to delete subdomain: ${response.statusText}`)
	}
}

export function extractDomainFromUrl(url: string): string {
	try {
		const urlObj = new URL(url.startsWith("http") ? url : `https://${url}`)
		return urlObj.hostname.toLowerCase()
	} catch {
		return cleanDomain(url)
	}
}

export function isSubdomain(fullDomain: string, parentDomain: string): boolean {
	const cleanFull = cleanDomain(fullDomain)
	const cleanParent = cleanDomain(parentDomain)
	return cleanFull.endsWith(`.${cleanParent}`) || cleanFull === cleanParent
}

export function getSubdomainName(fullDomain: string, parentDomain: string): string {
	const cleanFull = cleanDomain(fullDomain)
	const cleanParent = cleanDomain(parentDomain)
	if (cleanFull === cleanParent) return "@"
	if (cleanFull.endsWith(`.${cleanParent}`)) {
		return cleanFull.slice(0, -cleanParent.length - 1)
	}
	return cleanFull
}
