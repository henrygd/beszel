import { chartTimeData } from "@/lib/utils"
import type { ChartTimes, MonitorStats, NetworkMonitorRecord, NetworkMonitorStatsRecord, RawMonitorStatsRecord } from "@/types"
import { useEffect, useRef, useState } from "react"
import { appendData } from "@/components/routes/system/chart-data"
import { pb, getPbTimestamp } from "@/lib/api"
import { toast } from "@/components/ui/use-toast"
import type { RecordListOptions, RecordSubscription } from "pocketbase"

const cache = new Map<string, NetworkMonitorStatsRecord[]>()

function getCacheValue(systemId: string, chartTime: ChartTimes | "rt") {
	return cache.get(`${systemId}${chartTime}`) || []
}

function appendCacheValue(
	systemId: string,
	chartTime: ChartTimes | "rt",
	newStats: NetworkMonitorStatsRecord[],
	maxPoints = 100
) {
	const cache_key = `${systemId}${chartTime}`
	const existingStats = getCacheValue(systemId, chartTime)
	if (existingStats) {
		const { expectedInterval } = chartTimeData[chartTime]
		const updatedStats = appendData(existingStats, newStats, expectedInterval, maxPoints)
		cache.set(cache_key, updatedStats)
		return updatedStats
	} else {
		cache.set(cache_key, newStats)
		return newStats
	}
}

/** Merge an array of per-monitor raw records into the map-keyed format expected by chart components. */
export function mergeMonitorStats(rawRecords: RawMonitorStatsRecord[]): NetworkMonitorStatsRecord[] {
	const byTimestamp = new Map<number, Record<string, MonitorStats>>()
	for (const rec of rawRecords) {
		let statsMap = byTimestamp.get(rec.created)
		if (!statsMap) {
			statsMap = {}
			byTimestamp.set(rec.created, statsMap)
		}
		statsMap[rec.monitor] = {
			res_avg: rec.res_avg,
			res_min: rec.res_min,
			res_max: rec.res_max,
			loss: rec.loss,
		}
	}
	return Array.from(byTimestamp.entries())
		.sort(([a], [b]) => a - b)
		.map(([created, stats]) => ({ created, stats }))
}

/** Fetch raw per-monitor stats records for a system and time range, returning merged chart records. */
async function fetchMonitorStats(
	systemId: string,
	chartTime: ChartTimes,
	cached?: NetworkMonitorStatsRecord[]
): Promise<NetworkMonitorStatsRecord[]> {
	const lastCached = cached?.at(-1)?.created as number | undefined
	const rawRecords = await pb.collection<RawMonitorStatsRecord>("network_monitor_stats").getFullList({
		filter: pb.filter("system={:id} && created>{:created} && type={:type}", {
			id: systemId,
			created: getPbTimestamp(chartTime, lastCached ? new Date(lastCached + 1000) : undefined, true),
			type: chartTimeData[chartTime].type,
		}),
		fields: "monitor,res_avg,res_min,res_max,loss,created",
		sort: "created",
	})
	return mergeMonitorStats(rawRecords)
}

const NETWORK_MONITOR_FIELDS =
	"id,name,system,target,protocol,port,interval,res,resMin1h,resMax1h,resAvg1h,loss1h,enabled,updated"

interface UseNetworkMonitorsProps {
	systemId?: string
}

export function useNetworkMonitors(props: UseNetworkMonitorsProps) {
	const { systemId } = props

	const [monitors, setMonitors] = useState<NetworkMonitorRecord[]>([])
	const pendingMonitorEvents = useRef(new Map<string, RecordSubscription<NetworkMonitorRecord>>())
	const monitorBatchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	// initial load
	useEffect(() => {
		fetchMonitors(systemId).then((monitors) => setMonitors(monitors))
	}, [systemId])

	// subscribe to updates
	useEffect(() => {
		let unsubscribe: (() => void) | undefined

		function flushPendingMonitorEvents() {
			monitorBatchTimeout.current = null
			if (!pendingMonitorEvents.current.size) {
				return
			}
			const events = pendingMonitorEvents.current
			pendingMonitorEvents.current = new Map()
			setMonitors((currentMonitors) => {
				return applyMonitorEvents(currentMonitors ?? [], events.values(), systemId)
			})
		}

		const pbOptions: RecordListOptions = { fields: NETWORK_MONITOR_FIELDS }
		if (systemId) {
			pbOptions.filter = pb.filter("system = {:system}", { system: systemId })
		}

		;(async () => {
			try {
				unsubscribe = await pb.collection<NetworkMonitorRecord>("network_monitors").subscribe(
					"*",
					(event) => {
						pendingMonitorEvents.current.set(event.record.id, event)
						if (!monitorBatchTimeout.current) {
							monitorBatchTimeout.current = setTimeout(flushPendingMonitorEvents, 50)
						}
					},
					pbOptions
				)
			} catch (error) {
				console.error("Failed to subscribe to monitors", error)
			}
		})()

		return () => {
			if (monitorBatchTimeout.current !== null) {
				clearTimeout(monitorBatchTimeout.current)
				monitorBatchTimeout.current = null
			}
			pendingMonitorEvents.current.clear()
			unsubscribe?.()
		}
	}, [systemId])

	return monitors
}

interface UseNetworkMonitorStatsProps {
	systemId?: string
	chartTime: ChartTimes
}

export function useNetworkMonitorStats(props: UseNetworkMonitorStatsProps) {
	const { systemId, chartTime } = props
	const [monitorStats, setMonitorStats] = useState<NetworkMonitorStatsRecord[]>([])
	const requestID = useRef(0)
	// pending raw events to be merged (keyed by monitor+created)
	const pendingRaw = useRef(new Map<string, RawMonitorStatsRecord>())
	const mergeBatchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	useEffect(() => {
		if (!systemId) {
			setMonitorStats([])
			return
		}
		if (chartTime === "1m") {
			setMonitorStats(getCacheValue(systemId, "rt"))
			return
		}
		setMonitorStats(getCacheValue(systemId, chartTime))
	}, [systemId, chartTime])

	// fetch missing monitor stats on load and when chart time changes
	useEffect(() => {
		if (!systemId || !chartTime || chartTime === "1m") {
			return
		}

		const { expectedInterval } = chartTimeData[chartTime]
		const requestId = ++requestID.current

		const cachedMonitorStats = getCacheValue(systemId, chartTime)

		if (cachedMonitorStats.length) {
			setMonitorStats(cachedMonitorStats)
			const lastCreated = cachedMonitorStats.at(-1)?.created
			if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
				return
			}
		}

		fetchMonitorStats(systemId, chartTime, cachedMonitorStats).then((newMonitorStats) => {
			if (requestId !== requestID.current) {
				return
			}
			const merged = appendCacheValue(systemId, chartTime, newMonitorStats)
			setMonitorStats(merged)
		})
	}, [systemId, chartTime])

	// subscribe to new per-monitor stats records; batch them into merged chart records
	useEffect(() => {
		if (!systemId || !chartTime || chartTime === "1m") {
			return
		}
		let unsubscribe: (() => void) | undefined
		const pbOptions = {
			fields: "monitor,res_avg,res_min,res_max,loss,created,type",
			filter: pb.filter("system={:system} && type={:type}", {
				system: systemId,
				type: chartTimeData[chartTime].type,
			}),
		}

		function flushPending() {
			mergeBatchTimeout.current = null
			const pending = pendingRaw.current
			pendingRaw.current = new Map()
			const merged = mergeMonitorStats(Array.from(pending.values()))
			if (merged.length > 0) {
				const newStats = appendCacheValue(systemId!, chartTime, merged)
				setMonitorStats(newStats)
			}
		}

		;(async () => {
			try {
				unsubscribe = await pb.collection<RawMonitorStatsRecord>("network_monitor_stats").subscribe(
					"*",
					(event) => {
						if (event.action !== "create") {
							return
						}
						const rec = event.record
						pendingRaw.current.set(`${rec.monitor}:${rec.created}`, rec)
						if (!mergeBatchTimeout.current) {
							mergeBatchTimeout.current = setTimeout(flushPending, 200)
						}
					},
					pbOptions
				)
			} catch (error) {
				console.error("Failed to subscribe to monitor stats:", error)
			}
		})()

		return () => {
			if (mergeBatchTimeout.current) {
				clearTimeout(mergeBatchTimeout.current)
				mergeBatchTimeout.current = null
			}
			pendingRaw.current.clear()
			unsubscribe?.()
		}
	}, [systemId, chartTime])

	// subscribe to realtime metrics if chart time is 1m
	useEffect(() => {
		if (!systemId || chartTime !== "1m") {
			return
		}
		let unsubscribe: (() => void) | undefined
		pb.realtime
			.subscribe(
				`rt_metrics`,
				(data: { Monitors: NetworkMonitorStatsRecord["stats"] }) => {
					const stats = { created: Date.now(), stats: data.Monitors } as NetworkMonitorStatsRecord
					const newStats = appendCacheValue(systemId, "rt", [stats], 120)
					setMonitorStats(newStats)
				},
				{ query: { system: systemId } }
			)
			.then((us) => {
				unsubscribe = us
			})
		return () => unsubscribe?.()
	}, [chartTime, systemId])

	return monitorStats
}

async function fetchMonitors(system?: string) {
	try {
		return await pb.collection<NetworkMonitorRecord>("network_monitors").getFullList({
			fields: NETWORK_MONITOR_FIELDS,
			filter: system ? pb.filter("system={:system}", { system }) : undefined,
		})
	} catch (error) {
		toast({
			title: "Error",
			description: (error as Error)?.message,
			variant: "destructive",
		})
		return []
	}
}

function applyMonitorEvents(
	monitors: NetworkMonitorRecord[],
	events: Iterable<RecordSubscription<NetworkMonitorRecord>>,
	systemId?: string
) {
	const monitorById = new Map(monitors.map((monitor) => [monitor.id, monitor]))
	const createdMonitors: NetworkMonitorRecord[] = []

	for (const { action, record } of events) {
		const matchesSystemScope = !systemId || record.system === systemId

		if (action === "delete" || !matchesSystemScope) {
			monitorById.delete(record.id)
			continue
		}

		if (!monitorById.has(record.id)) {
			createdMonitors.push(record)
		}

		monitorById.set(record.id, record)
	}

	const nextMonitors: NetworkMonitorRecord[] = []
	for (let index = createdMonitors.length - 1; index >= 0; index -= 1) {
		nextMonitors.push(createdMonitors[index])
	}

	for (const monitor of monitors) {
		const nextMonitor = monitorById.get(monitor.id)
		if (!nextMonitor) {
			continue
		}
		nextMonitors.push(nextMonitor)
		monitorById.delete(monitor.id)
	}

	return nextMonitors
}
