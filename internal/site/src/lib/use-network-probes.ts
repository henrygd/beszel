import { chartTimeData } from "@/lib/utils"
import type {
	ChartTimes,
	NetworkProbeRecord,
	NetworkProbeStatsRecord,
	RawProbeStatsRecord,
	SystemRecord,
} from "@/types"
import { useEffect, useRef, useState } from "react"
import { appendData } from "@/components/routes/system/chart-data"
import { pb, getPbTimestamp } from "@/lib/api"
import { toast } from "@/components/ui/use-toast"
import type { RecordListOptions, RecordSubscription } from "pocketbase"

const cache = new Map<string, NetworkProbeStatsRecord[]>()

function getCacheValue(systemId: string, chartTime: ChartTimes | "rt") {
	return cache.get(`${systemId}${chartTime}`) || []
}

function appendCacheValue(
	systemId: string,
	chartTime: ChartTimes | "rt",
	newStats: NetworkProbeStatsRecord[],
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

/** Merge an array of per-probe raw records into the map-keyed format expected by chart components. */
export function mergeProbeStats(rawRecords: RawProbeStatsRecord[]): NetworkProbeStatsRecord[] {
	const byTimestamp = new Map<number, Record<string, number[]>>()
	for (const rec of rawRecords) {
		let statsMap = byTimestamp.get(rec.created)
		if (!statsMap) {
			statsMap = {}
			byTimestamp.set(rec.created, statsMap)
		}
		statsMap[rec.probe] = rec.stats
	}
	return Array.from(byTimestamp.entries())
		.sort(([a], [b]) => a - b)
		.map(([created, stats]) => ({ created, stats }))
}

/** Fetch raw per-probe stats records for a system and time range, returning merged chart records. */
async function fetchProbeStats(
	systemId: string,
	chartTime: ChartTimes,
	cached?: NetworkProbeStatsRecord[]
): Promise<NetworkProbeStatsRecord[]> {
	const lastCached = cached?.at(-1)?.created as number | undefined
	const rawRecords = await pb.collection<RawProbeStatsRecord>("network_probe_stats").getFullList({
		filter: pb.filter("system={:id} && created>{:created} && type={:type}", {
			id: systemId,
			created: getPbTimestamp(chartTime, lastCached ? new Date(lastCached + 1000) : undefined, true),
			type: chartTimeData[chartTime].type,
		}),
		fields: "probe,stats,created",
		sort: "created",
	})
	return mergeProbeStats(rawRecords)
}

const NETWORK_PROBE_FIELDS =
	"id,name,system,target,protocol,port,interval,res,resMin1h,resMax1h,resAvg1h,loss1h,enabled,updated"

interface UseNetworkProbesProps {
	systemId?: string
}

export function useNetworkProbes(props: UseNetworkProbesProps) {
	const { systemId } = props

	const [probes, setProbes] = useState<NetworkProbeRecord[]>([])
	const pendingProbeEvents = useRef(new Map<string, RecordSubscription<NetworkProbeRecord>>())
	const probeBatchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	// initial load
	useEffect(() => {
		fetchProbes(systemId).then((probes) => setProbes(probes))
	}, [systemId])

	// subscribe to updates
	useEffect(() => {
		let unsubscribe: (() => void) | undefined

		function flushPendingProbeEvents() {
			probeBatchTimeout.current = null
			if (!pendingProbeEvents.current.size) {
				return
			}
			const events = pendingProbeEvents.current
			pendingProbeEvents.current = new Map()
			setProbes((currentProbes) => {
				return applyProbeEvents(currentProbes ?? [], events.values(), systemId)
			})
		}

		const pbOptions: RecordListOptions = { fields: NETWORK_PROBE_FIELDS }
		if (systemId) {
			pbOptions.filter = pb.filter("system = {:system}", { system: systemId })
		}

		;(async () => {
			try {
				unsubscribe = await pb.collection<NetworkProbeRecord>("network_probes").subscribe(
					"*",
					(event) => {
						pendingProbeEvents.current.set(event.record.id, event)
						if (!probeBatchTimeout.current) {
							probeBatchTimeout.current = setTimeout(flushPendingProbeEvents, 50)
						}
					},
					pbOptions
				)
			} catch (error) {
				console.error("Failed to subscribe to probes", error)
			}
		})()

		return () => {
			if (probeBatchTimeout.current !== null) {
				clearTimeout(probeBatchTimeout.current)
				probeBatchTimeout.current = null
			}
			pendingProbeEvents.current.clear()
			unsubscribe?.()
		}
	}, [systemId])

	return probes
}

interface UseNetworkProbeStatsProps {
	systemId?: string
	chartTime: ChartTimes
}

export function useNetworkProbeStats(props: UseNetworkProbeStatsProps) {
	const { systemId, chartTime } = props
	const [probeStats, setProbeStats] = useState<NetworkProbeStatsRecord[]>([])
	const requestID = useRef(0)
	// pending raw events to be merged (keyed by probe+created)
	const pendingRaw = useRef(new Map<string, RawProbeStatsRecord>())
	const mergeBatchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	useEffect(() => {
		if (!systemId) {
			setProbeStats([])
			return
		}
		if (chartTime === "1m") {
			setProbeStats(getCacheValue(systemId, "rt"))
			return
		}
		setProbeStats(getCacheValue(systemId, chartTime))
	}, [systemId, chartTime])

	// fetch missing probe stats on load and when chart time changes
	useEffect(() => {
		if (!systemId || !chartTime || chartTime === "1m") {
			return
		}

		const { expectedInterval } = chartTimeData[chartTime]
		const requestId = ++requestID.current

		const cachedProbeStats = getCacheValue(systemId, chartTime)

		if (cachedProbeStats.length) {
			setProbeStats(cachedProbeStats)
			const lastCreated = cachedProbeStats.at(-1)?.created
			if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
				return
			}
		}

		fetchProbeStats(systemId, chartTime, cachedProbeStats).then((newProbeStats) => {
			if (requestId !== requestID.current) {
				return
			}
			const merged = appendCacheValue(systemId, chartTime, newProbeStats)
			setProbeStats(merged)
		})
	}, [systemId, chartTime])

	// subscribe to new per-probe stats records; batch them into merged chart records
	useEffect(() => {
		if (!systemId || !chartTime || chartTime === "1m") {
			return
		}
		let unsubscribe: (() => void) | undefined
		const pbOptions = {
			fields: "probe,stats,created,type",
			filter: pb.filter("system={:system} && type={:type}", {
				system: systemId,
				type: chartTimeData[chartTime].type,
			}),
		}

		function flushPending() {
			mergeBatchTimeout.current = null
			const pending = pendingRaw.current
			pendingRaw.current = new Map()
			const merged = mergeProbeStats(Array.from(pending.values()))
			if (merged.length > 0) {
				const newStats = appendCacheValue(systemId!, chartTime, merged)
				setProbeStats(newStats)
			}
		}

		;(async () => {
			try {
				unsubscribe = await pb.collection<RawProbeStatsRecord>("network_probe_stats").subscribe(
					"*",
					(event) => {
						if (event.action !== "create") {
							return
						}
						const rec = event.record
						pendingRaw.current.set(`${rec.probe}:${rec.created}`, rec)
						if (!mergeBatchTimeout.current) {
							mergeBatchTimeout.current = setTimeout(flushPending, 200)
						}
					},
					pbOptions
				)
			} catch (error) {
				console.error("Failed to subscribe to probe stats:", error)
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
		const cache_key = `${systemId}rt`
		pb.realtime
			.subscribe(
				`rt_metrics`,
				(data: { Probes: NetworkProbeStatsRecord["stats"] }) => {
					const prev = getCacheValue(systemId, "rt")
					const now = Date.now()
					const stats = { created: now, stats: data.Probes } as NetworkProbeStatsRecord
					const newStats = appendData(prev, [stats], 1000, 120)
					setProbeStats(() => newStats)
					cache.set(cache_key, newStats)
				},
				{ query: { system: systemId } }
			)
			.then((us) => {
				unsubscribe = us
			})
		return () => unsubscribe?.()
	}, [chartTime, systemId])

	return probeStats
}

async function fetchProbes(system?: string) {
	try {
		const res = await pb.collection<NetworkProbeRecord>("network_probes").getList(0, 2000, {
			fields: NETWORK_PROBE_FIELDS,
			filter: system ? pb.filter("system={:system}", { system }) : undefined,
		})
		return res.items
	} catch (error) {
		toast({
			title: "Error",
			description: (error as Error)?.message,
			variant: "destructive",
		})
		return []
	}
}

function applyProbeEvents(
	probes: NetworkProbeRecord[],
	events: Iterable<RecordSubscription<NetworkProbeRecord>>,
	systemId?: string
) {
	const probeById = new Map(probes.map((probe) => [probe.id, probe]))
	const createdProbes: NetworkProbeRecord[] = []

	for (const { action, record } of events) {
		const matchesSystemScope = !systemId || record.system === systemId

		if (action === "delete" || !matchesSystemScope) {
			probeById.delete(record.id)
			continue
		}

		if (!probeById.has(record.id)) {
			createdProbes.push(record)
		}

		probeById.set(record.id, record)
	}

	const nextProbes: NetworkProbeRecord[] = []
	for (let index = createdProbes.length - 1; index >= 0; index -= 1) {
		nextProbes.push(createdProbes[index])
	}

	for (const probe of probes) {
		const nextProbe = probeById.get(probe.id)
		if (!nextProbe) {
			continue
		}
		nextProbes.push(nextProbe)
		probeById.delete(probe.id)
	}

	return nextProbes
}

export interface ComparisonResult {
	/** Synthetic probe-like objects: id is the probe ID, name is the system name. */
	syntheticProbes: Array<{ id: string; name: string }>
	/** Merged stats keyed by probe ID (same format as NetworkProbeStatsRecord). */
	stats: NetworkProbeStatsRecord[]
	/** Probe interval in seconds (from the first matching probe). */
	interval: number
	/** TCP port, if applicable. */
	port: number
}

/**
 * Fetches stats for all probes sharing the same target+protocol across all systems.
 * Returns merged data suitable for the comparison chart.
 */
export async function fetchTargetComparison(
	target: string,
	protocol: string,
	chartTime: ChartTimes,
	systemIds?: string[]
): Promise<ComparisonResult> {
	const empty: ComparisonResult = { syntheticProbes: [], stats: [], interval: 0, port: 0 }

	// Find all probes that match this target + protocol, optionally restricted to specific systems
	const requestKey = systemIds?.length
		? `compare-${target}-${protocol}-${[...systemIds].sort().join(",")}`
		: `compare-${target}-${protocol}`
	let filter = pb.filter("target={:target} && protocol={:protocol}", { target, protocol })
	if (systemIds?.length) {
		filter = `(${filter}) && (${systemIds.map((id) => pb.filter("system={:id}", { id })).join(" || ")})`
	}
	const probeList = await pb
		.collection<NetworkProbeRecord>("network_probes")
		.getFullList({
			filter,
			fields: "id,system,interval,port",
			requestKey,
		})
	if (probeList.length === 0) {
		return empty
	}

	const matchedSystemIds = [...new Set(probeList.map((p) => p.system))]
	const systemFilter = matchedSystemIds.map((id) => `id='${id}'`).join(" || ")
	const systems = await pb
		.collection<SystemRecord>("systems")
		.getFullList({ filter: systemFilter, fields: "id,name", requestKey: `${requestKey}-systems` })
	const systemById = new Map(systems.map((s) => [s.id, s]))

	// Fetch raw per-probe stats for all matching probes in one query
	const probeIdFilter = probeList.map((p) => `probe='${p.id}'`).join(" || ")
	const statsType = chartTimeData[chartTime].type
	const rawRecords = await pb.collection<RawProbeStatsRecord>("network_probe_stats").getFullList({
		filter: `(${probeIdFilter}) && type='${statsType}'`,
		fields: "probe,stats,created",
		sort: "created",
		requestKey: `${requestKey}-stats`,
	})

	// mergeProbeStats groups by timestamp and keys by probe ID — exactly what the chart needs
	const stats = mergeProbeStats(rawRecords)

	// Build synthetic probe list (one entry per system that monitors this target)
	const probeToSystem = new Map(probeList.map((p) => [p.id, p.system]))
	const seen = new Set<string>()
	const syntheticProbes: ComparisonResult["syntheticProbes"] = []
	for (const p of probeList) {
		const sId = probeToSystem.get(p.id)!
		if (seen.has(sId)) continue
		seen.add(sId)
		const sys = systemById.get(sId)
		syntheticProbes.push({ id: p.id, name: sys?.name ?? sId })
	}

	return { syntheticProbes, stats, interval: probeList[0].interval, port: probeList[0].port }
}
