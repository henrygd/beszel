import { atom, onMount } from "nanostores"
import type { NotificationChannelRecord } from "@/types"
import { isAdmin, pb } from "./api"

const collection = pb.collection<NotificationChannelRecord>("notification_channels")

const byName = (a: NotificationChannelRecord, b: NotificationChannelRecord) => a.name.localeCompare(b.name)

/** Notification channels, loaded and kept in sync while in use. Admin only. */
export const $notificationChannels = atom<NotificationChannelRecord[]>([])

onMount($notificationChannels, () => {
	if (!isAdmin()) {
		return
	}
	let unsubscribe: (() => void) | undefined
	let cancelled = false
	;(async () => {
		try {
			$notificationChannels.set((await collection.getFullList()).sort(byName))
			const unsub = await collection.subscribe("*", ({ action, record }) => {
				const others = $notificationChannels.get().filter((channel) => channel.id !== record.id)
				$notificationChannels.set(action === "delete" ? others : [...others, record].sort(byName))
			})
			if (cancelled) {
				unsub()
			} else {
				unsubscribe = unsub
			}
		} catch (e) {
			console.error("load notification channels", e)
		}
	})()
	return () => {
		cancelled = true
		unsubscribe?.()
	}
})
