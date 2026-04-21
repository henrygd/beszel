import { chartTimeData } from "@/lib/utils"
import type { ChartTimes, NetworkProbeRecord, NetworkProbeStatsRecord } from "@/types"
import { useEffect, useRef, useState } from "react"
import { getStats, appendData } from "@/components/routes/system/chart-data"
import { pb } from "@/lib/api"
import { toast } from "@/components/ui/use-toast"
import type { RecordListOptions, RecordSubscription } from "pocketbase"

const cache = new Map<string, NetworkProbeStatsRecord[]>()

const NETWORK_PROBE_FIELDS = "id,name,system,target,protocol,port,interval,latency,loss,enabled,updated"

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

	// fetch probe stats when probes update
	useEffect(() => {
		if (!loadStats || !systemId || !chartTime || chartTime === "1m") {
			return
		}

		const { expectedInterval } = chartTimeData[chartTime]
		const cache_key = `${systemId}${chartTime}`
		const requestId = ++statsRequestId.current

		const cachedProbeStats = cache.get(cache_key) as NetworkProbeStatsRecord[] | undefined

		// Render from cache immediately if available
		if (cachedProbeStats?.length) {
			setProbeStats(cachedProbeStats)

			// Skip the fetch if the latest cached point is recent enough that no new point is expected yet
			const lastCreated = cachedProbeStats.at(-1)?.created
			if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
				return
			}
		}

		getStats<NetworkProbeStatsRecord>("network_probe_stats", systemId, chartTime, cachedProbeStats).then(
			(probeStats) => {
				// If another request has been made since this one, ignore the results
				if (requestId !== statsRequestId.current) {
					return
				}

				// make new system stats
				let probeStatsData = (cache.get(cache_key) || []) as NetworkProbeStatsRecord[]
				if (probeStats.length) {
					probeStatsData = appendData(probeStatsData, probeStats, expectedInterval, 100)
					cache.set(cache_key, probeStatsData)
				}
				setProbeStats(probeStatsData)
			}
		)
	}, [chartTime, probes])

	return {
		probes,
		probeStats,
	}
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
