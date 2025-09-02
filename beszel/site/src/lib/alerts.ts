import type { AlertInfo, AlertRecord } from "@/types"
import type { RecordSubscription } from "pocketbase"
import { $alerts } from "@/lib/stores"
import { EthernetIcon } from "@/components/ui/icons"
import { ServerIcon, CpuIcon, MemoryStickIcon, HardDriveIcon, ThermometerIcon, HourglassIcon } from "lucide-react"
import { t } from "@lingui/core/macro"
import { pb } from "./api"

/** Alert info for each alert type */
export const alertInfo: Record<string, AlertInfo> = {
	Status: {
		name: () => t`Status`,
		unit: "",
		icon: ServerIcon,
		desc: () => t`Triggers when status switches between up and down`,
		/** "for x minutes" is appended to desc when only one value */
		singleDesc: () => t`System` + " " + t`Down`,
	},
	CPU: {
		name: () => t`CPU Usage`,
		unit: "%",
		icon: CpuIcon,
		desc: () => t`Triggers when CPU usage exceeds a threshold`,
	},
	Memory: {
		name: () => t`Memory Usage`,
		unit: "%",
		icon: MemoryStickIcon,
		desc: () => t`Triggers when memory usage exceeds a threshold`,
	},
	Disk: {
		name: () => t`Disk Usage`,
		unit: "%",
		icon: HardDriveIcon,
		desc: () => t`Triggers when usage of any disk exceeds a threshold`,
	},
	Bandwidth: {
		name: () => t`Bandwidth`,
		unit: " Mbps",
		icon: EthernetIcon,
		desc: () => t`Triggers when combined up/down exceeds a threshold`,
		max: 100,
		step: 1,
		start: 100,
		// Special handling for bandwidth units
		hasUnits: true,
		units: [
			{ label: "Mbps", multiplier: 1 },
			{ label: "Gbps", multiplier: 1000 },
		],
	},
	BandwidthUp: {
		name: () => t`Upload Bandwidth`,
		unit: " Mbps",
		icon: EthernetIcon,
		desc: () => t`Triggers when upload speed exceeds a threshold`,
		max: 100,
		step: 1,
		start: 50,
		hasUnits: true,
		units: [
			{ label: "Mbps", multiplier: 1 },
			{ label: "Gbps", multiplier: 1000 },
		],
	},
	BandwidthDown: {
		name: () => t`Download Bandwidth`,
		unit: " Mbps",
		icon: EthernetIcon,
		desc: () => t`Triggers when download speed exceeds a threshold`,
		max: 100,
		step: 1,
		start: 50,
		hasUnits: true,
		units: [
			{ label: "Mbps", multiplier: 1 },
			{ label: "Gbps", multiplier: 1000 },
		],
	},
	Temperature: {
		name: () => t`Temperature`,
		unit: "°C",
		icon: ThermometerIcon,
		desc: () => t`Triggers when any sensor exceeds a threshold`,
	},
	LoadAvg1: {
		name: () => t`Load Average 1m`,
		unit: "",
		icon: HourglassIcon,
		max: 100,
		min: 0.1,
		start: 10,
		step: 0.1,
		desc: () => t`Triggers when 1 minute load average exceeds a threshold`,
	},
	LoadAvg5: {
		name: () => t`Load Average 5m`,
		unit: "",
		icon: HourglassIcon,
		max: 100,
		min: 0.1,
		start: 10,
		step: 0.1,
		desc: () => t`Triggers when 5 minute load average exceeds a threshold`,
	},
	LoadAvg15: {
		name: () => t`Load Average 15m`,
		unit: "",
		icon: HourglassIcon,
		min: 0.1,
		max: 100,
		start: 10,
		step: 0.1,
		desc: () => t`Triggers when 15 minute load average exceeds a threshold`,
	},
	Swap: {
		name: () => t`Swap Usage`,
		unit: "%",
		icon: MemoryStickIcon,
		desc: () => t`Triggers when swap usage exceeds a threshold`,
	},
} as const

/** Helper to manage user alerts */
export const alertManager = (() => {
	const collection = pb.collection<AlertRecord>("alerts")
	let unsub: () => void

	/** Fields to fetch from alerts collection */
	const fields = "id,name,system,value,min,triggered,repeat_interval,max_repeats,repeat_count,last_sent"

	/** Fetch alerts from collection */
	async function fetchAlerts(): Promise<AlertRecord[]> {
		return await collection.getFullList<AlertRecord>({ fields, sort: "updated" })
	}

	/** Format alerts into a map of system id to alert name to alert record */
	function add(alerts: AlertRecord[]) {
		for (const alert of alerts) {
			const systemId = alert.system
			const systemAlerts = $alerts.get()[systemId] ?? new Map()
			const newAlerts = new Map(systemAlerts)
			newAlerts.set(alert.name, alert)
			$alerts.setKey(systemId, newAlerts)
		}
	}

	function remove(alerts: Pick<AlertRecord, "name" | "system">[]) {
		for (const alert of alerts) {
			const systemId = alert.system
			const systemAlerts = $alerts.get()[systemId]
			const newAlerts = new Map(systemAlerts)
			newAlerts.delete(alert.name)
			$alerts.setKey(systemId, newAlerts)
		}
	}

	const actionFns = {
		create: add,
		update: add,
		delete: remove,
	}

	// batch alert updates to prevent unnecessary re-renders when adding many alerts at once
	const batchUpdate = (() => {
		const batch = new Map<string, RecordSubscription<AlertRecord>>()
		let timeout: ReturnType<typeof setTimeout>

		return (data: RecordSubscription<AlertRecord>) => {
			const { record } = data
			batch.set(`${record.system}${record.name}`, data)
			clearTimeout(timeout!)
			timeout = setTimeout(() => {
				const groups = { create: [], update: [], delete: [] } as Record<string, AlertRecord[]>
				for (const { action, record } of batch.values()) {
					groups[action]?.push(record)
				}
				for (const key in groups) {
					if (groups[key].length) {
						actionFns[key as keyof typeof actionFns]?.(groups[key])
					}
				}
				batch.clear()
			}, 50)
		}
	})()

	async function subscribe() {
		unsub = await collection.subscribe("*", batchUpdate, { fields })
	}

	function unsubscribe() {
		unsub?.()
	}

	async function refresh() {
		const records = await fetchAlerts()
		add(records)
	}

	return {
		/** Add alerts to store */
		add,
		/** Remove alerts from store */
		remove,
		/** Subscribe to alerts */
		subscribe,
		/** Unsubscribe from alerts */
		unsubscribe,
		/** Refresh alerts with latest data from hub */
		refresh,
	}
})()
