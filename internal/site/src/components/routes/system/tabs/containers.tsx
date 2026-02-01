import { t } from "@lingui/core/macro"
import { lazy } from "react"
import ContainerChart from "@/components/charts/container-chart"
import { ChartType } from "@/lib/enums"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { ChartCard } from "./shared"
import type { ContainersTabProps } from "./types"

const ContainersTable = lazy(() => import("../../../containers-table/containers-table"))

function LazyContainersTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <ContainersTable systemId={systemId} />}
		</div>
	)
}

function dockerOrPodman(str: string, system: SystemRecord): string {
	if (system.info.p) {
		return str.replace("docker", "podman").replace("Docker", "Podman")
	}
	return str
}

export function ContainersTab({
	chartData,
	grid,
	dataEmpty,
	containerFilterBar,
	containerChartConfigs,
	system,
}: ContainersTabProps) {
	return (
		<>
			<div className="grid xl:grid-cols-2 gap-4 mb-4">
				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={dockerOrPodman(t`Docker CPU Usage`, system)}
					description={t`Average CPU utilization of containers`}
					cornerEl={containerFilterBar}
				>
					<ContainerChart
						chartData={chartData}
						dataKey="c"
						chartType={ChartType.CPU}
						chartConfig={containerChartConfigs.cpu}
					/>
				</ChartCard>

				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={dockerOrPodman(t`Docker Memory Usage`, system)}
					description={dockerOrPodman(t`Memory usage of docker containers`, system)}
					cornerEl={containerFilterBar}
				>
					<ContainerChart
						chartData={chartData}
						dataKey="m"
						chartType={ChartType.Memory}
						chartConfig={containerChartConfigs.memory}
					/>
				</ChartCard>

				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={dockerOrPodman(t`Docker Network I/O`, system)}
					description={dockerOrPodman(t`Network traffic of docker containers`, system)}
					cornerEl={containerFilterBar}
				>
					<ContainerChart
						chartData={chartData}
						chartType={ChartType.Network}
						dataKey="n"
						chartConfig={containerChartConfigs.network}
					/>
				</ChartCard>
			</div>
			<LazyContainersTable systemId={system.id} />
		</>
	)
}
