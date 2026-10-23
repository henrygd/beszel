import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { Unit } from "@/lib/enums"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, FilterBar, SelectAvgMax } from "../chart-card"
import { dockerOrPodman } from "../chart-data"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"

/**
 * Converts deprecated GB memory values from records before 0.21.1 to bytes.
 * TODO: remove when the deprecated fields are removed. Also needed for realtime data from
 * agents < 0.21.1, which is not migrated by the hub, and for stored records (kept up to 30 days).
 */
const gbToBytes = (gb?: number) => (gb === undefined ? undefined : gb * 1024 ** 3)

const tickFormatter = (value: number) => {
	const { value: convertedValue, unit } = formatBytes(value, false, Unit.Bytes)
	return `${toFixedFloat(convertedValue, convertedValue >= 10 ? 0 : 1)} ${unit}`
}

const contentFormatter = ({ value }: { value: number }) => {
	const { value: convertedValue, unit } = formatBytes(value, false, Unit.Bytes)
	return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
}

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
	const lastStats = chartData.systemStats.at(-1)?.stats
	const totalMem = lastStats?.mem?.[0] ?? gbToBytes(lastStats?.m) ?? 0

	return (
		<ChartCard
			alertSection="memory"
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
				tickFormatter={tickFormatter}
				contentFormatter={contentFormatter}
				dataPoints={[
					{
						label: t`Used`,
						dataKey: ({ stats }) =>
							showMax ? (stats?.memm ?? gbToBytes(stats?.mm)) : (stats?.mem?.[1] ?? gbToBytes(stats?.mu)),
						color: 2,
						opacity: 0.4,
						stackId: "1",
						order: 3,
					},
					{
						label: "ZFS ARC",
						// undefined when zero to keep ZFS ARC out of the tooltip on systems without ZFS
						dataKey: ({ stats }) => (showMax ? null : (stats?.mem?.[3] ?? gbToBytes(stats?.mz)) || undefined),
						color: "hsla(175 60% 45% / 0.8)",
						opacity: 0.5,
						stackId: "1",
						order: 2,
					},
					{
						label: t`Cache / Buffers`,
						dataKey: ({ stats }) => (showMax ? null : (stats?.mem?.[2] ?? gbToBytes(stats?.mb))),
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
			description={t`Memory usage of containers`}
			cornerEl={<FilterBar />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.containerData}
				dataPoints={dataPoints}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val, false, Unit.Bytes, true)
					return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={(item) => {
					const { value, unit } = formatBytes(item.value, false, Unit.Bytes, true)
					return `${decimalString(value)} ${unit}`
				}}
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

	const lastStats = systemStats.at(-1)?.stats
	const hasSwapData = (lastStats?.swap?.[1] ?? gbToBytes(lastStats?.su) ?? 0) > 0
	if (!hasSwapData) {
		return null
	}
	return (
		<ChartCard empty={dataEmpty} grid={grid} title={t`Swap Usage`} description={t`Swap space used by the system`}>
			<AreaChartDefault
				chartData={chartData}
				domain={[
					0,
					() => {
						const stats = chartData.systemStats.at(-1)?.stats
						return stats?.swap?.[0] ?? gbToBytes(stats?.s) ?? 0
					},
				]}
				contentFormatter={contentFormatter}
				tickFormatter={tickFormatter}
				dataPoints={[
					{
						label: t`Used`,
						dataKey: ({ stats }) => stats?.swap?.[1] ?? gbToBytes(stats?.su),
						color: 2,
						opacity: 0.4,
					},
				]}
			></AreaChartDefault>
		</ChartCard>
	)
}
