import { useState, useEffect } from "react"
import { pb } from "@/lib/api"

export interface PushNotificationState {
	isSupported: boolean
	permission: NotificationPermission | null
	subscription: PushSubscription | null
	isRegistering: boolean
	error: string | null
}

export function usePushNotifications() {
	const [state, setState] = useState<PushNotificationState>({
		isSupported: false,
		permission: null,
		subscription: null,
		isRegistering: false,
		error: null,
	})

	// Check if push notifications are supported
	useEffect(() => {
		const isSupported =
			"serviceWorker" in navigator &&
			"PushManager" in window &&
			"Notification" in window

		setState((prev) => ({
			...prev,
			isSupported,
			permission: isSupported ? Notification.permission : null,
		}))

		if (isSupported) {
			checkSubscription()
		}
	}, [])

	// Check existing subscription
	const checkSubscription = async () => {
		try {
			const registration = await navigator.serviceWorker.ready
			const existingSub = await registration.pushManager.getSubscription()
			setState((prev) => ({ ...prev, subscription: existingSub }))
		} catch (err) {
			console.error("Error checking subscription:", err)
		}
	}

	// Register service worker
	const registerServiceWorker = async () => {
		try {
			const registration = await navigator.serviceWorker.register("/sw.js")
			console.log("Service Worker registered:", registration)
			return registration
		} catch (err) {
			console.error("Service Worker registration failed:", err)
			throw err
		}
	}

	// Request permission and subscribe
	const subscribe = async () => {
		setState((prev) => ({ ...prev, isRegistering: true, error: null }))

		try {
			// Request notification permission
			const permission = await Notification.requestPermission()
			setState((prev) => ({ ...prev, permission }))

			if (permission !== "granted") {
				throw new Error("Notification permission denied")
			}

			// Register service worker
			const registration = await registerServiceWorker()

			// Get VAPID public key from server
			const response = await fetch("/api/beszel/notifications/vapid-key", {
				headers: {
					Authorization: `Bearer ${pb.authStore.token}`,
				},
			})
			
			if (!response.ok) {
				throw new Error("Failed to get VAPID key")
			}
			
			const { publicKey } = await response.json()

			// Subscribe to push notifications
			const subscription = await registration.pushManager.subscribe({
				userVisibleOnly: true,
				applicationServerKey: urlBase64ToUint8Array(publicKey) as BufferSource,
			})

			// Send subscription to server
			const saveResponse = await fetch("/api/beszel/notifications/subscribe", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					Authorization: `Bearer ${pb.authStore.token}`,
				},
				body: JSON.stringify(subscription),
			})

			if (!saveResponse.ok) {
				throw new Error("Failed to save subscription")
			}

			setState((prev) => ({
				...prev,
				subscription,
				isRegistering: false,
			}))

			return subscription
		} catch (err) {
			const errorMessage = err instanceof Error ? err.message : "Unknown error"
			setState((prev) => ({
				...prev,
				isRegistering: false,
				error: errorMessage,
			}))
			throw err
		}
	}

	// Unsubscribe from push notifications
	const unsubscribe = async () => {
		setState((prev) => ({ ...prev, isRegistering: true, error: null }))

		try {
			const registration = await navigator.serviceWorker.ready
			const subscription = await registration.pushManager.getSubscription()

			if (subscription) {
				await subscription.unsubscribe()

				// Notify server to remove subscription
				await fetch("/api/beszel/notifications/unsubscribe", {
					method: "POST",
					headers: {
						"Content-Type": "application/json",
						Authorization: `Bearer ${pb.authStore.token}`,
					},
					body: JSON.stringify({ endpoint: subscription.endpoint }),
				})
			}

			setState((prev) => ({
				...prev,
				subscription: null,
				isRegistering: false,
			}))
		} catch (err) {
			const errorMessage = err instanceof Error ? err.message : "Unknown error"
			setState((prev) => ({
				...prev,
				isRegistering: false,
				error: errorMessage,
			}))
			throw err
		}
	}

	// Send test notification
	const sendTestNotification = async () => {
		try {
			const response = await fetch("/api/beszel/notifications/test-push", {
				method: "POST",
				headers: {
					Authorization: `Bearer ${pb.authStore.token}`,
				},
			})

			if (!response.ok) {
				throw new Error("Failed to send test notification")
			}
		} catch (err) {
			console.error("Test notification failed:", err)
			throw err
		}
	}

	return {
		...state,
		subscribe,
		unsubscribe,
		sendTestNotification,
	}
}

// Helper function to convert VAPID key
function urlBase64ToUint8Array(base64String: string): Uint8Array {
	const padding = "=".repeat((4 - (base64String.length % 4)) % 4)
	const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/")
	const rawData = window.atob(base64)
	const outputArray = new Uint8Array(rawData.length)

	for (let i = 0; i < rawData.length; ++i) {
		outputArray[i] = rawData.charCodeAt(i)
	}

	return outputArray
}
