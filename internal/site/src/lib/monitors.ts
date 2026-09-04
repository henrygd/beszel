import { map } from "nanostores"
import { pb } from "@/lib/api"
import type { MonitorRecord, MonitorsSummary } from "@/types"

export const $monitors = map<Record<string, MonitorRecord>>({})
export const $monitorsSummary = map<MonitorsSummary | null>(null)

let initialized = false
let unsub: (() => void) | undefined | void

async function refresh() {
	try {
		const records = await pb.collection<MonitorRecord>("monitors").getFullList()
		$monitors.set(Object.fromEntries(records.map((m) => [m.id, m])))
	} catch {
		// collection may not exist on older hubs; stay empty
	}
	try {
		const summary = await pb.send<MonitorsSummary>("/api/beszel/monitors/summary", {})
		$monitorsSummary.set(summary)
	} catch {
		// ignore
	}
}

/** Initialize the monitors store and realtime subscription */
export function init() {
	if (initialized) {
		return
	}
	initialized = true
	refresh()
	unsub = pb.collection("monitors").subscribe("*", (e) => {
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
	}) as unknown as void
}

async function refreshSummary() {
	try {
		const summary = await pb.send<MonitorsSummary>("/api/beszel/monitors/summary", {})
		$monitorsSummary.set(summary)
	} catch {
		// ignore
	}
}

export function cleanup() {
	unsub?.()
	unsub = undefined
	initialized = false
}
