import { chartTimeData } from "@/lib/utils"
import type { ChartTimes, NetworkProbeRecord, NetworkProbeStatsRecord } from "@/types"
import { useEffect, useRef, useState } from "react"
import { getStats, appendData } from "@/components/routes/system/chart-data"
import { pb } from "@/lib/api"
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

const NETWORK_PROBE_FIELDS =
	"id,name,system,target,protocol,port,interval,response,resMin1h,resMax1h,resAvg1h,loss,enabled,updated"

interface UseNetworkProbesProps {
	systemId?: string
	loadStats?: boolean
	chartTime?: ChartTimes
	existingProbes?: NetworkProbeRecord[]
}

export function useNetworkProbesData(props: UseNetworkProbesProps) {
	const { systemId, loadStats, chartTime, existingProbes } = props

	const [p, setProbes] = useState<NetworkProbeRecord[]>([])
	const [probeStats, setProbeStats] = useState<NetworkProbeStatsRecord[]>([])
	const statsRequestId = useRef(0)
	const pendingProbeEvents = useRef(new Map<string, RecordSubscription<NetworkProbeRecord>>())
	const probeBatchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	const probes = existingProbes ?? p

	// clear old data when systemId changes
	// useEffect(() => {
	// 	return setProbes([])
	// }, [systemId])

	// initial load - fetch probes if not provided by caller
	useEffect(() => {
		if (!existingProbes) {
			fetchProbes(systemId).then((probes) => setProbes(probes))
		}
	}, [systemId])

	// Subscribe to updates if probes not provided by caller
	useEffect(() => {
		if (existingProbes) {
			return
		}
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

	// Subscribe to new probe stats
	useEffect(() => {
		if (!loadStats || !systemId) {
			return
		}
		let unsubscribe: (() => void) | undefined
		const pbOptions = {
			fields: "stats,created,type",
			filter: pb.filter("system = {:system}", { system: systemId }),
		}

		;(async () => {
			try {
				unsubscribe = await pb.collection<NetworkProbeStatsRecord>("network_probe_stats").subscribe(
					"*",
					(event) => {
						if (!chartTime || event.action !== "create") {
							return
						}
						// if (typeof event.record.created === "string") {
						// 	event.record.created = new Date(event.record.created).getTime()
						// }
						// return if not current chart time
						// we could append to other chart times, but we would need to check the timestamps
						// to make sure they fit in correctly, so for simplicity just ignore non-chart-time updates
						// and fetch them via API when the user switches to that chart time
						const chartTimeRecordType = chartTimeData[chartTime].type as ChartTimes
						if (event.record.type !== chartTimeRecordType) {
							// const lastCreated = getCacheValue(systemId, chartTime)?.at(-1)?.created ?? 0
							// if (lastCreated) {
							// 	// if the new record is close enough to the last cached record, append it to the cache so it's available immediately if the user switches to that chart time
							// 	const { expectedInterval } = chartTimeData[chartTime]
							// 	if (event.record.created - lastCreated < expectedInterval * 1.5) {
							// 		console.log(
							// 			`Caching out-of-chart-time probe stats record for chart time ${chartTime} (record type: ${event.record.type})`
							// 		)
							// 		const newStats = appendCacheValue(systemId, chartTime, [event.record])
							// 		cache.set(`${systemId}${chartTime}`, newStats)
							// 	}
							// }
							// console.log(`Received probe stats for non-current chart time (${event.record.type}), ignoring for now`)
							return
						}

						// console.log("Appending new probe stats to chart:", event.record)
						const newStats = appendCacheValue(systemId, chartTime, [event.record])
						setProbeStats(newStats)
					},
					pbOptions
				)
			} catch (error) {
				console.error("Failed to subscribe to probe stats:", error)
			}
		})()

		return () => unsubscribe?.()
	}, [systemId])

	// fetch missing probe stats on load and when chart time changes
	useEffect(() => {
		if (!loadStats || !systemId || !chartTime || chartTime === "1m") {
			return
		}

		const { expectedInterval } = chartTimeData[chartTime]
		const requestId = ++statsRequestId.current

		const cachedProbeStats = getCacheValue(systemId, chartTime)

		// Render from cache immediately if available
		if (cachedProbeStats.length) {
			setProbeStats(cachedProbeStats)

			// Skip the fetch if the latest cached point is recent enough that no new point is expected yet
			const lastCreated = cachedProbeStats.at(-1)?.created
			if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
				console.log("Using cached probe stats, skipping fetch")
				return
			}
		}

		getStats<NetworkProbeStatsRecord>("network_probe_stats", systemId, chartTime, cachedProbeStats).then(
			(probeStats) => {
				// If another request has been made since this one, ignore the results
				if (requestId !== statsRequestId.current) {
					return
				}
				const newStats = appendCacheValue(systemId, chartTime, probeStats)
				setProbeStats(newStats)
			}
		)
	}, [chartTime])

	// subscribe to realtime metrics if chart time is 1m
	useEffect(() => {
		if (!loadStats || !systemId || chartTime !== "1m") {
			return
		}
		let unsubscribe: (() => void) | undefined
		const cache_key = `${systemId}rt`
		pb.realtime
			.subscribe(
				`rt_metrics`,
				(data: { Probes: NetworkProbeStatsRecord["stats"] }) => {
					let prev = getCacheValue(systemId, "rt")
					const now = Date.now()
					// if no previous data or the last data point is older than 1min,
					// create a new data set starting with a point 1 second ago to seed the chart data
					if (!prev || (prev.at(-1)?.created ?? 0) < now - 60_000) {
						prev = [{ created: now - 1000, stats: probesToStats(probes) }]
					}
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

	return {
		probes,
		probeStats,
	}
}

export function probeKey(p: NetworkProbeRecord) {
	if (p.protocol === "tcp") return `${p.protocol}:${p.target}:${p.port}`
	return `${p.protocol}:${p.target}`
}

function probesToStats(probes: NetworkProbeRecord[]): NetworkProbeStatsRecord["stats"] {
	const stats: NetworkProbeStatsRecord["stats"] = {}
	for (const probe of probes) {
		const key = probeKey(probe)
		stats[key] = [probe.response, 0, 0, probe.loss]
	}
	return stats
}

async function fetchProbes(systemId?: string) {
	try {
		const res = await pb.collection<NetworkProbeRecord>("network_probes").getList(0, 2000, {
			fields: NETWORK_PROBE_FIELDS,
			filter: systemId ? pb.filter("system={:system}", { system: systemId }) : undefined,
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
	// Use a map to handle updates/deletes in constant time
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
	// Prepend brand new probes (matching previous behavior)
	for (let index = createdProbes.length - 1; index >= 0; index -= 1) {
		nextProbes.push(createdProbes[index])
	}

	// Rebuild the final list while preserving original order for existing probes
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
