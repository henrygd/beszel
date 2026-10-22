import { chartTimeData } from "@/lib/utils"
import {
	clearFailedResponse,
	mergeMonitorStats,
	mergeSameTimestamps,
	withMonitorGaps,
} from "@/lib/network-monitor-utils"
import type {
	ChartTimes,
	MonitorStats,
	NetworkMonitorRecord,
	NetworkMonitorStatsRecord,
	RawMonitorStatsRecord,
} from "@/types"
import { useEffect, useMemo, useRef, useState } from "react"
import { appendData } from "@/components/routes/system/chart-data"
import { pb, getPbTimestamp } from "@/lib/api"
import { toast } from "@/components/ui/use-toast"
import type { RecordListOptions, RecordSubscription } from "pocketbase"

const cache = new Map<string, NetworkMonitorStatsRecord[]>()

function getCacheValue(cacheKey: string, chartTime: ChartTimes | "rt") {
	return cache.get(`${cacheKey}:${chartTime}`) || []
}

function appendCacheValue(
	cacheKey: string,
	chartTime: ChartTimes | "rt",
	newStats: NetworkMonitorStatsRecord[],
	maxPoints = chartTimeData[chartTime]?.maxPoints ?? 100
) {
	const existingStats = getCacheValue(cacheKey, chartTime)
	const { expectedInterval } = chartTimeData[chartTime]
	const remaining = mergeSameTimestamps(existingStats, newStats)
	// Copy when records were only merged in place so React still sees a new array.
	const base = remaining.length < newStats.length ? existingStats.slice() : existingStats
	const updatedStats = appendData(base, remaining, expectedInterval, maxPoints)
	cache.set(`${cacheKey}:${chartTime}`, updatedStats)
	return updatedStats
}

/** Build a `(monitor={:m0} || monitor={:m1} ...)` filter expression and its params. */
function monitorIdsFilter(monitorIds: string[]) {
	const params: Record<string, string> = {}
	const expr = monitorIds
		.map((id, i) => {
			params[`m${i}`] = id
			return `monitor={:m${i}}`
		})
		.join(" || ")
	return { expr: `(${expr})`, params }
}

/** Raw 1m records are timestamped by each system, so align them when comparing monitors across systems. */
function getBucketMs(monitorIds: string[], chartTime: ChartTimes) {
	const { type, expectedInterval } = chartTimeData[chartTime]
	return monitorIds.length > 1 && type === "1m" ? expectedInterval : 0
}

/** Fetch stats for one or more monitors and a time range, returning merged chart records. */
async function fetchMonitorStats(
	monitorIds: string[],
	chartTime: ChartTimes,
	cached?: NetworkMonitorStatsRecord[]
): Promise<NetworkMonitorStatsRecord[]> {
	const lastCached = cached?.at(-1)?.created as number | undefined
	const bucketMs = getBucketMs(monitorIds, chartTime)
	// Bucketed timestamps are floored, so refetch the whole last bucket; mergeSameTimestamps folds in the overlap.
	const from = lastCached ? new Date(bucketMs ? lastCached - 1 : lastCached + 1000) : undefined
	const { expr, params } = monitorIdsFilter(monitorIds)
	const rawRecords = await pb.collection<RawMonitorStatsRecord>("network_monitor_stats").getFullList({
		filter: pb.filter(`${expr} && created>{:created} && type={:type}`, {
			...params,
			created: getPbTimestamp(chartTime, from, true),
			type: chartTimeData[chartTime].type,
		}),
		fields: "monitor,res_min,res_max,total_count,success_count,res_sum,created",
		sort: "created",
	})
	return mergeMonitorStats(rawRecords, bucketMs)
}

const NETWORK_MONITOR_FIELDS =
	"id,system,target,protocol,port,server,interval,skipTlsVerify,res,resMin1h,resMax1h,resAvg1h,loss1h,enabled,certInfo,updated"

interface UseNetworkMonitorsProps {
	systemId?: string
}

export function useNetworkMonitors(props: UseNetworkMonitorsProps) {
	const { systemId } = props

	const [monitors, setMonitors] = useState<NetworkMonitorRecord[]>([])
	const [isLoading, setIsLoading] = useState(true)
	const pendingMonitorEvents = useRef(new Map<string, RecordSubscription<NetworkMonitorRecord>>())
	const monitorBatchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	// initial load
	useEffect(() => {
		let cancelled = false
		setIsLoading(true)
		setMonitors([])
		fetchMonitors(systemId).then((monitors) => {
			if (cancelled) return
			setMonitors(monitors)
			setIsLoading(false)
		})
		return () => {
			cancelled = true
		}
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

	return { monitors, isLoading }
}

interface UseNetworkMonitorStatsProps {
	systemId: string
	/** One monitor, or several (e.g. the same target on different systems) to chart together. */
	monitorIds: string[]
	/** Opened monitor's probe interval in seconds, used to tell missing data apart from slow probes */
	interval: number
	chartTime: ChartTimes
	enabled?: boolean
}

/**
 * Returns stats for the given monitors. A single monitor's stats get empty records inserted where
 * data is missing (see withMonitorGaps).
 */
export function useNetworkMonitorStats(props: UseNetworkMonitorStatsProps) {
	const { systemId, interval, chartTime, enabled = true } = props
	// Stable key so effects don't re-run when callers pass a new array with the same IDs.
	const cacheKey = [...props.monitorIds].sort().join(",")
	const [monitorStats, setMonitorStats] = useState<NetworkMonitorStatsRecord[]>([])
	// pending raw events to be merged (keyed by monitor+created)
	const pendingRaw = useRef(new Map<string, RawMonitorStatsRecord>())
	const mergeBatchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	useEffect(() => {
		setMonitorStats(getCacheValue(cacheKey, chartTime === "1m" ? "rt" : chartTime))
	}, [cacheKey, chartTime])

	// Fetch only the selected monitors' missing history.
	useEffect(() => {
		if (!enabled || chartTime === "1m") {
			return
		}

		let cancelled = false
		const { expectedInterval } = chartTimeData[chartTime]
		const cachedMonitorStats = getCacheValue(cacheKey, chartTime)

		if (cachedMonitorStats.length) {
			setMonitorStats(cachedMonitorStats)
			const lastCreated = cachedMonitorStats.at(-1)?.created
			if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
				return
			}
		}

		fetchMonitorStats(cacheKey.split(","), chartTime, cachedMonitorStats)
			.then((newMonitorStats) => {
				if (cancelled) return
				setMonitorStats(appendCacheValue(cacheKey, chartTime, newMonitorStats))
			})
			.catch((error) => {
				if (!cancelled) console.error("Failed to fetch monitor stats:", error)
			})
		return () => {
			cancelled = true
		}
	}, [cacheKey, chartTime, enabled])

	// subscribe to new per-monitor stats records; batch them into merged chart records
	useEffect(() => {
		if (!enabled || chartTime === "1m") {
			return
		}
		let cancelled = false
		let unsubscribe: (() => void) | undefined
		const monitorIds = cacheKey.split(",")
		const bucketMs = getBucketMs(monitorIds, chartTime)
		const { expr, params } = monitorIdsFilter(monitorIds)
		const pbOptions = {
			fields: "monitor,res_min,res_max,total_count,success_count,res_sum,created,type",
			filter: pb.filter(`${expr} && type={:type}`, {
				...params,
				type: chartTimeData[chartTime].type,
			}),
		}

		function flushPending() {
			mergeBatchTimeout.current = null
			const pending = pendingRaw.current
			pendingRaw.current = new Map()
			const merged = mergeMonitorStats(Array.from(pending.values()), bucketMs)
			if (merged.length > 0) {
				const newStats = appendCacheValue(cacheKey, chartTime, merged)
				setMonitorStats(newStats)
			}
		}

		;(async () => {
			try {
				unsubscribe = await pb.collection<RawMonitorStatsRecord>("network_monitor_stats").subscribe(
					"*",
					(event) => {
						if (cancelled || event.action !== "create") {
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
				if (cancelled) unsubscribe()
			} catch (error) {
				console.error("Failed to subscribe to monitor stats:", error)
			}
		})()

		return () => {
			cancelled = true
			if (mergeBatchTimeout.current) {
				clearTimeout(mergeBatchTimeout.current)
				mergeBatchTimeout.current = null
			}
			pendingRaw.current.clear()
			unsubscribe?.()
		}
	}, [cacheKey, chartTime, enabled])

	// subscribe to realtime metrics if chart time is 1m
	useEffect(() => {
		if (!enabled || chartTime !== "1m") {
			return
		}
		let cancelled = false
		let unsubscribe: (() => void) | undefined
		pb.realtime
			.subscribe(
				`rt_metrics`,
				(data: { Monitors: NetworkMonitorStatsRecord["stats"] }) => {
					if (cancelled || !data.Monitors) return
					// realtime metrics are per system, so only this system's monitors are available
					const monitorStats: Record<string, MonitorStats> = {}
					for (const id of cacheKey.split(",")) {
						if (data.Monitors[id]) monitorStats[id] = clearFailedResponse(data.Monitors[id])
					}
					if (!Object.keys(monitorStats).length) return
					const stats = { created: Date.now(), stats: monitorStats }
					const newStats = appendCacheValue(cacheKey, "rt", [stats], 120)
					setMonitorStats(newStats)
				},
				{ query: { system: systemId } }
			)
			.then((us) => {
				unsubscribe = us
				if (cancelled) unsubscribe()
			})
		return () => {
			cancelled = true
			unsubscribe?.()
		}
	}, [chartTime, systemId, cacheKey, enabled])

	return useMemo(() => {
		// Multi-monitor charts connect lines instead (monitors don't share timestamps), so skip gap markers.
		if (cacheKey.includes(",")) return monitorStats
		return withMonitorGaps(monitorStats, { id: cacheKey, interval }, chartTimeData[chartTime].expectedInterval)
	}, [monitorStats, cacheKey, interval, chartTime])
}

/** Only what comparison charts and labels need. */
const COMPARE_MONITOR_FIELDS = "id,system,target,protocol,port,server,interval,resAvg1h"

/**
 * Monitors of one protocol on all systems except the given one, to compare against (#2385).
 * Fetched per open so it also works in single-system tables, which only hold one system's monitors.
 */
export function useCompareMonitors(system: string, protocol: string, enabled = true) {
	const key = `${system}:${protocol}`
	const [result, setResult] = useState<{ key: string; monitors: NetworkMonitorRecord[] }>({ key, monitors: [] })

	useEffect(() => {
		if (!enabled) return
		let cancelled = false
		pb.collection<NetworkMonitorRecord>("network_monitors")
			.getFullList({
				fields: COMPARE_MONITOR_FIELDS,
				filter: pb.filter("system!={:system} && protocol={:protocol}", { system, protocol }),
			})
			.then((monitors) => {
				if (!cancelled) setResult({ key: `${system}:${protocol}`, monitors })
			})
			.catch((error) => {
				if (!cancelled) console.error("Failed to fetch compare monitors:", error)
			})
		return () => {
			cancelled = true
		}
	}, [system, protocol, enabled])

	// Keep showing the last result while reopening refreshes it, but never another monitor's.
	return result.key === key ? result.monitors : []
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
