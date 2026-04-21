import { lazy, useEffect, useRef, useState } from "react"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { chartTimeData, cn } from "@/lib/utils"
import { NetworkProbeRecord, NetworkProbeStatsRecord } from "@/types"
import { LatencyChart } from "./charts/probes-charts"
import { SystemData } from "./use-system-data"
import { $chartTime } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import system from "../system"
import { getStats, appendData } from "./chart-data"

const ContainersTable = lazy(() => import("../../containers-table/containers-table"))

export function LazyContainersTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <ContainersTable systemId={systemId} />}
		</div>
	)
}

const SmartTable = lazy(() => import("./smart-table"))

export function LazySmartTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SmartTable systemId={systemId} />}
		</div>
	)
}

const SystemdTable = lazy(() => import("../../systemd-table/systemd-table"))

export function LazySystemdTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver()
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SystemdTable systemId={systemId} />}
		</div>
	)
}

const NetworkProbesTableNew = lazy(() => import("@/components/network-probes-table/network-probes-table"))

const cache = new Map<string, any>()

export function LazyNetworkProbesTableNew({ systemId, systemData }: { systemId: string; systemData: SystemData }) {
	const { grid, chartData } = systemData ?? {}
	const [probes, setProbes] = useState<NetworkProbeRecord[]>([])
	const chartTime = useStore($chartTime)
	const [probeStats, setProbeStats] = useState<NetworkProbeStatsRecord[]>([])
	const { isIntersecting, ref } = useIntersectionObserver()

	const statsRequestId = useRef(0)

	// get stats when system "changes." (Not just system to system,
	// also when new info comes in via systemManager realtime connection, indicating an update)
	useEffect(() => {
		if (!systemId || !chartTime || chartTime === "1m") {
			return
		}

		const { expectedInterval } = chartTimeData[chartTime]
		const ss_cache_key = `${systemId}${chartTime}`
		const requestId = ++statsRequestId.current

		const cachedProbeStats = cache.get(ss_cache_key) as NetworkProbeStatsRecord[] | undefined

		// Render from cache immediately if available
		// if (cachedProbeStats?.length) {
		// 	setProbeStats(cachedProbeStats)

		// 	// Skip the fetch if the latest cached point is recent enough that no new point is expected yet
		// 	const lastCreated = cachedProbeStats.at(-1)?.created as number | undefined
		// 	if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
		// 		return
		// 	}
		// }

		getStats<NetworkProbeStatsRecord>("network_probe_stats", systemId, chartTime, cachedProbeStats).then(
			(probeStats) => {
				// If another request has been made since this one, ignore the results
				if (requestId !== statsRequestId.current) {
					return
				}

				// make new system stats
				let probeStatsData = (cache.get(ss_cache_key) || []) as NetworkProbeStatsRecord[]
				if (probeStats.length) {
					probeStatsData = appendData(probeStatsData, probeStats, expectedInterval, 100)
					cache.set(ss_cache_key, probeStatsData)
				}
				setProbeStats(probeStatsData)
			}
		)
	}, [system, chartTime, probes])

	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && (
				<>
					<NetworkProbesTableNew systemId={systemId} probes={probes} setProbes={setProbes} />
					{!!chartData && (
						<LatencyChart
							probeStats={probeStats}
							grid={grid}
							probes={probes}
							chartData={chartData}
							empty={!probeStats.length}
						/>
					)}
				</>
			)}
		</div>
	)
}
const NetworkProbesTable = lazy(() => import("@/components/routes/system/network-probes"))

export function LazyNetworkProbesTable({
	system,
	chartData,
	grid,
	probeStats,
}: {
	system: any
	chartData: any
	grid: any
	probeStats: any
}) {
	const { isIntersecting, ref } = useIntersectionObserver()
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && (
				<NetworkProbesTable system={system} chartData={chartData} grid={grid} realtimeProbeStats={probeStats} />
			)}
		</div>
	)
}
