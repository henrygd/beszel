import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { decimalString, toFixedFloat } from "@/lib/utils"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData } from "@/types"
import { pinnedAxisDomain } from "@/components/ui/chart"
import CpuCoresSheet from "../cpu-sheet"
import { ChartCard, FilterBar, SelectAvgMax } from "../chart-card"
import { dockerOrPodman } from "../chart-data"

export function CpuChart({
	chartData,
	grid,
	dataEmpty,
	showMax,
	isLongerChart,
	maxValues,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	showMax: boolean
	isLongerChart: boolean
	maxValues: boolean
}) {
	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`CPU Usage`}
			description={t`Average system-wide CPU utilization`}
			cornerEl={
				<div className="flex gap-2">
					{maxValSelect}
					<CpuCoresSheet chartData={chartData} dataEmpty={dataEmpty} grid={grid} maxValues={maxValues} />
				</div>
			}
		>
			<AreaChartDefault
				chartData={chartData}
				maxToggled={showMax}
				dataPoints={[
					{
						label: t`CPU Usage`,
						dataKey: ({ stats }) => (showMax ? stats?.cpum : stats?.cpu),
						color: 1,
						opacity: 0.4,
					},
				]}
				tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
				contentFormatter={({ value }) => `${decimalString(value)}%`}
				domain={pinnedAxisDomain()}
			/>
		</ChartCard>
	)
}

export function ContainerCpuChart({
	chartData,
	grid,
	dataEmpty,
	isPodman,
	cpuConfig,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	isPodman: boolean
	cpuConfig: ChartConfig
}) {
	const { filter, dataPoints } = useContainerDataPoints(cpuConfig, (key, data) => data[key]?.c ?? null)

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={dockerOrPodman(t`Docker CPU Usage`, isPodman)}
			description={t`Average CPU utilization of containers`}
			cornerEl={<FilterBar />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.containerData}
				dataPoints={dataPoints}
				tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
				contentFormatter={({ value }) => `${decimalString(value)}%`}
				domain={pinnedAxisDomain()}
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}
