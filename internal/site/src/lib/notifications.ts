import { pb } from "./api"

export type NotificationType =
	| "email"
	| "webhook"
	| "discord"
	| "slack"
	| "telegram"
	| "gotify"
	| "pushover"

export interface Notification {
	id: string
	name: string
	type: NotificationType
	is_default: boolean
	active: boolean
	settings: Record<string, unknown>
	created: string
	updated: string
}

export interface EmailSettings {
	smtp_host: string
	smtp_port: number
	smtp_user: string
	smtp_password: string
	from_email: string
	to_email: string
	use_tls: boolean
}

export interface WebhookSettings {
	url: string
	method: string
	headers?: Record<string, string>
	body_template?: string
}

export interface DiscordSettings {
	webhook_url: string
	username?: string
	avatar_url?: string
}

export interface SlackSettings {
	webhook_url: string
	channel?: string
	username?: string
}

export interface TelegramSettings {
	bot_token: string
	chat_id: string
}

export interface GotifySettings {
	server_url: string
	app_token: string
	priority?: number
}

export interface PushoverSettings {
	app_token: string
	user_key: string
	priority?: number
	device?: string
}

export type NotificationSettings =
	| EmailSettings
	| WebhookSettings
	| DiscordSettings
	| SlackSettings
	| TelegramSettings
	| GotifySettings
	| PushoverSettings

export interface CreateNotificationRequest {
	name: string
	type: NotificationType
	settings: NotificationSettings
	is_default?: boolean
}

export interface UpdateNotificationRequest {
	name?: string
	settings?: NotificationSettings
	is_default?: boolean
	active?: boolean
}

const API_BASE = "/api/beszel/notifications"

export async function getNotifications(): Promise<Notification[]> {
	const response = await fetch(API_BASE, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch notifications: ${response.statusText}`)
	}
	return response.json()
}

export async function getNotification(id: string): Promise<Notification> {
	const response = await fetch(`${API_BASE}/${id}`, {
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch notification: ${response.statusText}`)
	}
	return response.json()
}

export async function createNotification(
	data: CreateNotificationRequest
): Promise<Notification> {
	const response = await fetch(API_BASE, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify(data),
	})
	if (!response.ok) {
		throw new Error(`Failed to create notification: ${response.statusText}`)
	}
	return response.json()
}

export async function updateNotification(
	id: string,
	data: UpdateNotificationRequest
): Promise<Notification> {
	const response = await fetch(`${API_BASE}/${id}`, {
		method: "PATCH",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify(data),
	})
	if (!response.ok) {
		throw new Error(`Failed to update notification: ${response.statusText}`)
	}
	return response.json()
}

export async function deleteNotification(id: string): Promise<void> {
	const response = await fetch(`${API_BASE}/${id}`, {
		method: "DELETE",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to delete notification: ${response.statusText}`)
	}
}

export async function testNotification(id: string): Promise<void> {
	const response = await fetch(`${API_BASE}/${id}/test`, {
		method: "POST",
		headers: {
			Authorization: `Bearer ${pb.authStore.token}`,
		},
	})
	if (!response.ok) {
		throw new Error(`Failed to send test notification: ${response.statusText}`)
	}
}

export function getProviderLabel(type: NotificationType): string {
	const labels: Record<NotificationType, string> = {
		email: "Email (SMTP)",
		webhook: "Webhook",
		discord: "Discord",
		slack: "Slack",
		telegram: "Telegram",
		gotify: "Gotify",
		pushover: "Pushover",
	}
	return labels[type]
}

export function getDefaultSettings(type: NotificationType): NotificationSettings {
	switch (type) {
		case "email":
			return {
				smtp_host: "",
				smtp_port: 587,
				smtp_user: "",
				smtp_password: "",
				from_email: "",
				to_email: "",
				use_tls: true,
			}
		case "webhook":
			return {
				url: "",
				method: "POST",
				headers: {},
			}
		case "discord":
			return {
				webhook_url: "",
				username: "Beszel",
			}
		case "slack":
			return {
				webhook_url: "",
				username: "Beszel",
			}
		case "telegram":
			return {
				bot_token: "",
				chat_id: "",
			}
		case "gotify":
			return {
				server_url: "",
				app_token: "",
				priority: 5,
			}
		case "pushover":
			return {
				app_token: "",
				user_key: "",
				priority: 0,
			}
		default:
			return {} as NotificationSettings
	}
}

const MONITORS_API_BASE = "/api/beszel/monitors"

export async function getMonitorNotifications(monitorId: string): Promise<string[]> {
	const response = await fetch(`${MONITORS_API_BASE}/${monitorId}/notifications`, {
		headers: { Authorization: `Bearer ${pb.authStore.token}` },
	})
	if (!response.ok) {
		throw new Error(`Failed to fetch monitor notifications: ${response.statusText}`)
	}
	const data = await response.json()
	return data.notification_ids ?? []
}

export async function setMonitorNotifications(monitorId: string, notificationIds: string[]): Promise<void> {
	const response = await fetch(`${MONITORS_API_BASE}/${monitorId}/notifications`, {
		method: "PUT",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${pb.authStore.token}`,
		},
		body: JSON.stringify({ notification_ids: notificationIds }),
	})
	if (!response.ok) {
		throw new Error(`Failed to set monitor notifications: ${response.statusText}`)
	}
}
