import { map } from "nanostores"
import { pb } from "@/lib/api"
import type { MonitorRecord, MonitorsSummary } from "@/types"

export const $monitors = map<Record<string, MonitorRecord>>({})
export const $monitorsSummary = map<MonitorsSummary | null>(null)

let initialized = false
let unsub: (() => void) | null = null
let loadedOnce: Promise<void> | null = null

async function refresh() {
	try {
		const records = await pb.collection<MonitorRecord>("monitors").getFullList()
		$monitors.set(Object.fromEntries(records.map((m) => [m.id, m])))
	} catch {
		// collection may not exist on older hubs; stay empty
	}
	await refreshSummary()
}

async function refreshSummary() {
	try {
		const summary = await pb.send<MonitorsSummary>("/api/beszel/monitors/summary", {})
		$monitorsSummary.set(summary)
	} catch {
		// ignore
	}
}

/** Initialize the monitors store and realtime subscription (call once).
 * Returns a promise that resolves after the first load. Pages must NOT
 * call cleanup() (app-lifetime subscription owned by main.tsx). */
export function init(): Promise<void> {
	if (initialized) {
		return loadedOnce ?? Promise.resolve()
	}
	initialized = true
	loadedOnce = refresh()
	pb.collection("monitors")
		.subscribe("*", (e) => {
			const record = e.record as unknown as MonitorRecord
			const current = $monitors.get()
			if (e.action === "delete") {
				const next = { ...current }
				delete next[record.id]
				$monitors.set(next)
			} else {
				$monitors.set({ ...current, [record.id]: record })
			}
			refreshSummary()
		})
		.then((fn) => {
			unsub = fn
		})
		.catch(() => {
			unsub = null
		})
	return loadedOnce
}

export function cleanup() {
	unsub?.()
	unsub = null
	initialized = false
}
