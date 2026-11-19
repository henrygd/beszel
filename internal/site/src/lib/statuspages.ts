import { pb } from "./api"

export type StatusPageTheme = "light" | "dark" | "auto"

export interface StatusPage {
	id: string
	name: string
	slug: string
	title: string
	description: string
	logo: string
	favicon: string
	theme: StatusPageTheme
	public: boolean
	show_uptime: boolean
	monitor_count: number
	created: string
	updated: string
}

export interface StatusPageMonitor {
	id: string
	monitor_id: string
	display_name: string
	group: string
	sort_order: number
}

export interface PublicMonitorStatus {
	id: string
	name: string
	display_name?: string
	group?: string
	status: string
	last_error?: string
	uptime_24h: number
	uptime_7d: number
	uptime_30d: number
	last_check: string
	heartbeats?: PublicHeartbeat[]
}

export interface PublicHeartbeat {
	status: string
	ping: number
	msg?: string
	time: string
}

export interface PublicIncident {
	id: string
	title: string
	description: string
	status: string
	severity: string
	started_at: string
	resolved_at?: string
}

export interface PublicStatusPage {
	id: string
	name: string
	title: string
	description: string
	logo?: string
	favicon?: string
	theme?: string
	custom_css?: string
	monitors: PublicMonitorStatus[]
	incidents: PublicIncident[]
	overall_status: string
	updated_at: string
}

export interface CreateStatusPageRequest {
	name: string
	slug: string
	title: string
	description?: string
	logo?: string
	favicon?: string
	theme?: StatusPageTheme
	custom_css?: string
	public: boolean
	show_uptime?: boolean
}

export interface UpdateStatusPageRequest {
	name?: string
	title?: string
	description?: string
	logo?: string
	favicon?: string
	theme?: StatusPageTheme
	custom_css?: string
	public?: boolean
	show_uptime?: boolean
}

export interface AddMonitorRequest {
	monitor: string
	display_name?: string
	group?: string
	sort_order?: number
}

const API_BASE = "/api/beszel/status-pages"

export async function getStatusPages(): Promise<StatusPage[]> {
	const response = await fetch(API_BASE, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch status pages: ${response.statusText}`)
	}
	return response.json()
}

export async function getStatusPage(id: string): Promise<StatusPage> {
	const response = await fetch(`${API_BASE}/${id}`, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch status page: ${response.statusText}`)
	}
	return response.json()
}

export async function createStatusPage(data: CreateStatusPageRequest): Promise<StatusPage> {
	const response = await fetch(API_BASE, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify(data),
	})
	if (!response.ok) {
		throw new Error(`Failed to create status page: ${response.statusText}`)
	}
	return response.json()
}

export async function updateStatusPage(
	id: string,
	data: UpdateStatusPageRequest
): Promise<StatusPage> {
	const response = await fetch(`${API_BASE}/${id}`, {
		method: "PATCH",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify(data),
	})
	if (!response.ok) {
		throw new Error(`Failed to update status page: ${response.statusText}`)
	}
	return response.json()
}

export async function deleteStatusPage(id: string): Promise<void> {
	const response = await fetch(`${API_BASE}/${id}`, {
		method: "DELETE",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to delete status page: ${response.statusText}`)
	}
}

export async function getStatusPageMonitors(id: string): Promise<StatusPageMonitor[]> {
	const response = await fetch(`${API_BASE}/${id}/monitors`, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch monitors: ${response.statusText}`)
	}
	return response.json()
}

export async function addMonitorToStatusPage(
	id: string,
	data: AddMonitorRequest
): Promise<void> {
	const response = await fetch(`${API_BASE}/${id}/monitors`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify(data),
	})
	if (!response.ok) {
		throw new Error(`Failed to add monitor: ${response.statusText}`)
	}
}

export async function removeMonitorFromStatusPage(
	id: string,
	monitorId: string
): Promise<void> {
	const response = await fetch(`${API_BASE}/${id}/monitors/${monitorId}`, {
		method: "DELETE",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to remove monitor: ${response.statusText}`)
	}
}

export async function getPublicStatusPage(slug: string): Promise<PublicStatusPage> {
	const response = await fetch(`/status/${slug}`)
	if (!response.ok) {
		throw new Error(`Status page not found: ${response.statusText}`)
	}
	return response.json()
}

export function getStatusPageUrl(slug: string): string {
	return `/status/${slug}`
}

export function getStatusBadgeColor(status: string): string {
	switch (status) {
		case "operational":
			return "bg-green-500"
		case "degraded":
			return "bg-yellow-500"
		case "partial_outage":
			return "bg-orange-500"
		case "major_outage":
			return "bg-red-500"
		case "up":
			return "bg-green-500"
		case "down":
			return "bg-red-500"
		default:
			return "bg-gray-500"
	}
}

export function getStatusLabel(status: string): string {
	switch (status) {
		case "operational":
			return "All Systems Operational"
		case "degraded":
			return "Degraded Performance"
		case "partial_outage":
			return "Partial Outage"
		case "major_outage":
			return "Major Outage"
		case "up":
			return "Up"
		case "down":
			return "Down"
		default:
			return "Unknown"
	}
}
