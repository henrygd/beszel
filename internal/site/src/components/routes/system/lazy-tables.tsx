import { lazy } from "react"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn } from "@/lib/utils"
import { ResponseLossChart } from "./charts/probes-charts"
import type { SystemData } from "./use-system-data"
import { $chartTime } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { useNetworkProbeStats } from "@/lib/use-network-probes"
import type { NetworkProbeRecord } from "@/types"

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

export function LazyProbesCharts({
	systemId,
	probes,
	systemData,
}: {
	systemId: string
	probes: NetworkProbeRecord[]
	systemData: SystemData
}) {
	const { isIntersecting, ref } = useIntersectionObserver()

	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <ProbesCharts systemId={systemId} probes={probes} systemData={systemData} />}
		</div>
	)
}

function ProbesCharts({
	systemId,
	probes,
	systemData,
}: {
	systemId: string
	probes: NetworkProbeRecord[]
	systemData: SystemData
}) {
	const { grid, chartData } = systemData ?? {}
	const chartTime = useStore($chartTime)

	const probeStats = useNetworkProbeStats({ systemId, chartTime })

	return (
		<>
			{!!chartData && !!probes.length && (
				<ResponseLossChart
					probeStats={probeStats}
					grid={grid}
					probes={probes}
					chartData={chartData}
					empty={!probeStats.length}
				/>
			)}
		</>
	)
}
