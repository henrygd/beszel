import { lazy } from "react"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn } from "@/lib/utils"
import { ResponseChart, LossChart } from "./charts/probes-charts"
import type { SystemData } from "./use-system-data"
import { $chartTime } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { useNetworkProbes, useNetworkProbeStats } from "@/lib/use-network-probes"

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

const NetworkProbesTable = lazy(() => import("@/components/network-probes-table/network-probes-table"))

export function LazyNetworkProbesTable({ systemId, systemData }: { systemId: string; systemData: SystemData }) {
	const { isIntersecting, ref } = useIntersectionObserver()

	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <ProbesTable systemId={systemId} systemData={systemData} />}
		</div>
	)
}

function ProbesTable({ systemId, systemData }: { systemId: string; systemData: SystemData }) {
	const { grid, chartData } = systemData ?? {}
	const chartTime = useStore($chartTime)

	const probes = useNetworkProbes({ systemId })
	const probeStats = useNetworkProbeStats({ systemId, chartTime })

	return (
		<>
			<NetworkProbesTable systemId={systemId} probes={probes} />
			{!!chartData && !!probes.length && (
				<div className="grid xl:grid-cols-2 gap-4">
					<ResponseChart
						probeStats={probeStats}
						grid={grid}
						probes={probes}
						chartData={chartData}
						empty={!probeStats.length}
					/>
					<LossChart
						probeStats={probeStats}
						grid={grid}
						probes={probes}
						chartData={chartData}
						empty={!probeStats.length}
					/>
				</div>
			)}
		</>
	)
}
