import { pb } from "@/lib/api"
import { $allMonitorsById, $downMonitors, $pausedMonitors, $upMonitors } from "@/lib/stores"
import type { MonitorRecord } from "@/types"
import { SystemStatus } from "./enums"

const COLLECTION = pb.collection<MonitorRecord>("monitors")
const FIELDS_DEFAULT = "id,name,type,url,host,port,interval,timeout,secure,retry,method,expected_status,expected_body,status,num_retries"

let initialized = false
// biome-ignore lint/suspicious/noConfusingVoidType: typescript rocks
let unsub: (() => void) | undefined | void

/** Initialize the monitors manager and set up listeners */
export function init() {
	if (initialized) {
		return
	}
	initialized = true

	// sync monitor stores on change
	$allMonitorsById.listen((newMonitors, oldMonitors, changedKey) => {
		const oldMonitor = oldMonitors[changedKey]
		const newMonitor = newMonitors[changedKey]

		// if monitor is undefined (deleted), remove it from the stores
		if (oldMonitor && !newMonitor?.id) {
			$upMonitors.setKey(oldMonitor.id, undefined as unknown as MonitorRecord)
			$downMonitors.setKey(oldMonitor.id, undefined as unknown as MonitorRecord)
			$pausedMonitors.setKey(oldMonitor.id, undefined as unknown as MonitorRecord)
			return
		}

		if (!newMonitor) {
			return
		}

		const newStatus = newMonitor.status
		if (newStatus === SystemStatus.Up) {
			$upMonitors.setKey(newMonitor.id, newMonitor)
			$downMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
			$pausedMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
		} else if (newStatus === SystemStatus.Down) {
			$downMonitors.setKey(newMonitor.id, newMonitor)
			$upMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
			$pausedMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
		} else if (newStatus === SystemStatus.Paused) {
			$pausedMonitors.setKey(newMonitor.id, newMonitor)
			$upMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
			$downMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
		} else if (newStatus === SystemStatus.Pending) {
			$upMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
			$downMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
			$pausedMonitors.setKey(newMonitor.id, undefined as unknown as MonitorRecord)
		}
	})
}

/** Fetch monitors from collection */
async function fetchMonitors(): Promise<MonitorRecord[]> {
	try {
		return await COLLECTION.getFullList({ sort: "+name", fields: FIELDS_DEFAULT })
	} catch (error) {
		console.error("Failed to fetch monitors:", error)
		return []
	}
}

/** Add monitor to store */
export function add(monitor: MonitorRecord) {
	$allMonitorsById.setKey(monitor.id, monitor)
}

/** Update monitor in store */
export function update(monitor: MonitorRecord) {
	$allMonitorsById.setKey(monitor.id, monitor)
}

/** Remove monitor from store */
export function remove(monitor: MonitorRecord) {
	$allMonitorsById.setKey(monitor.id, undefined as unknown as MonitorRecord)
}

/** Action functions for subscription */
const actionFns: Record<string, (monitor: MonitorRecord) => void> = {
	create: add,
	update: update,
	delete: remove,
}

/** Subscribe to real-time monitor updates from the collection */
export async function subscribe() {
	try {
		unsub = await COLLECTION.subscribe("*", ({ action, record }) => actionFns[action]?.(record), {
			fields: FIELDS_DEFAULT,
		})
	} catch (error) {
		console.error("Failed to subscribe to monitors collection:", error)
	}
}

/** Refresh all monitors with latest data from the hub */
export async function refresh() {
	try {
		const records = await fetchMonitors()
		for (const record of records) {
			add(record)
		}
	} catch (error) {
		console.error("Failed to refresh monitors:", error)
	}
}

/** Unsubscribe from real-time monitor updates */
export const unsubscribe = () => (unsub = unsub?.())
