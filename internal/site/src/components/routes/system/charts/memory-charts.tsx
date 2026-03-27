import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { Unit } from "@/lib/enums"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, FilterBar, SelectAvgMax } from "../chart-card"
import { dockerOrPodman } from "../chart-data"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import { pinnedAxisDomain } from "@/components/ui/chart"

export function MemoryChart({
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
	const totalMem = toFixedFloat(chartData.systemStats.at(-1)?.stats.m ?? 0, 1)

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Memory Usage`}
			description={t`Precise utilization at the recorded time`}
			cornerEl={maxValSelect}
		>
			<AreaChartDefault
				chartData={chartData}
				domain={[0, totalMem]}
				itemSorter={(a, b) => a.order - b.order}
				maxToggled={showMax}
				showTotal={true}
				tickFormatter={(value) => {
					const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
					return `${toFixedFloat(convertedValue, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={({ value }) => {
					const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
					return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
				}}
				dataPoints={[
					{
						label: t`Used`,
						dataKey: ({ stats }) => (showMax ? stats?.mm : stats?.mu),
						color: 2,
						opacity: 0.4,
						stackId: "1",
						order: 3,
					},
					{
						label: "ZFS ARC",
						dataKey: ({ stats }) => (showMax ? null : stats?.mz),
						color: "hsla(175 60% 45% / 0.8)",
						opacity: 0.5,
						order: 2,
					},
					{
						label: t`Cache / Buffers`,
						dataKey: ({ stats }) => (showMax ? null : stats?.mb),
						color: "hsla(160 60% 45% / 0.5)",
						opacity: 0.4,
						stackId: "1",
						order: 1,
					},
				]}
			/>
		</ChartCard>
	)
}

export function ContainerMemoryChart({
	chartData,
	grid,
	dataEmpty,
	isPodman,
	memoryConfig,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	isPodman: boolean
	memoryConfig: ChartConfig
}) {
	const { filter, dataPoints } = useContainerDataPoints(memoryConfig, (key, data) => data[key]?.m ?? null)

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={dockerOrPodman(t`Docker Memory Usage`, isPodman)}
			description={dockerOrPodman(t`Memory usage of docker containers`, isPodman)}
			cornerEl={<FilterBar />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.containerData}
				dataPoints={dataPoints}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val, false, Unit.Bytes, true)
					return `${toFixedFloat(value, val >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={(item) => {
					const { value, unit } = formatBytes(item.value, false, Unit.Bytes, true)
					return `${decimalString(value)} ${unit}`
				}}
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

export function SwapChart({
	chartData,
	grid,
	dataEmpty,
	systemStats,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	systemStats: SystemStatsRecord[]
}) {
	// const userSettings = useStore($userSettings)

	const hasSwapData = (systemStats.at(-1)?.stats.su ?? 0) > 0
	if (!hasSwapData) {
		return null
	}
	return (
		<ChartCard empty={dataEmpty} grid={grid} title={t`Swap Usage`} description={t`Swap space used by the system`}>
			<AreaChartDefault
				chartData={chartData}
				domain={[0, () => toFixedFloat(chartData.systemStats.at(-1)?.stats.s ?? 0.04, 2)]}
				contentFormatter={({ value }) => {
					// mem values are supplied as GB
					const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
					return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
				}}
				tickFormatter={(value) => {
					const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
					return `${toFixedFloat(convertedValue, value >= 10 ? 0 : 1)} ${unit}`
				}}
				dataPoints={[
					{
						label: t`Used`,
						dataKey: ({ stats }) => stats?.su,
						color: 2,
						opacity: 0.4,
					},
				]}
			></AreaChartDefault>
		</ChartCard>
	)
}
