import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, SelectAvgMax } from "../chart-card"
import { Unit } from "@/lib/enums"

export function DiskCharts({
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
	systemStats: SystemStatsRecord[]
}) {
	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null
	const userSettings = $userSettings.get()

	let diskSize = chartData.systemStats?.at(-1)?.stats.d ?? NaN
	// round to nearest GB
	if (diskSize >= 100) {
		diskSize = Math.round(diskSize)
	}

	return (
		<>
			<ChartCard empty={dataEmpty} grid={grid} title={t`Disk Usage`} description={t`Usage of root partition`}>
				<AreaChartDefault
					chartData={chartData}
					domain={[0, diskSize]}
					tickFormatter={(val) => {
						const { value, unit } = formatBytes(val * 1024, false, Unit.Bytes, true)
						return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
					}}
					contentFormatter={({ value }) => {
						const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
						return `${decimalString(convertedValue)} ${unit}`
					}}
					dataPoints={[
						{
							label: t`Disk Usage`,
							color: 4,
							opacity: 0.4,
							dataKey: ({ stats }) => stats?.du,
						},
					]}
				></AreaChartDefault>
			</ChartCard>

			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`Disk I/O`}
				description={t`Throughput of root filesystem`}
				cornerEl={maxValSelect}
			>
				<AreaChartDefault
					chartData={chartData}
					maxToggled={showMax}
					dataPoints={[
						{
							label: t({ message: "Write", comment: "Disk write" }),
							dataKey: ({ stats }: SystemStatsRecord) => {
								if (showMax) {
									return stats?.dio?.[1] ?? (stats?.dwm ?? 0) * 1024 * 1024
								}
								return stats?.dio?.[1] ?? (stats?.dw ?? 0) * 1024 * 1024
							},
							color: 3,
							opacity: 0.3,
						},
						{
							label: t({ message: "Read", comment: "Disk read" }),
							dataKey: ({ stats }: SystemStatsRecord) => {
								if (showMax) {
									return stats?.diom?.[0] ?? (stats?.drm ?? 0) * 1024 * 1024
								}
								return stats?.dio?.[0] ?? (stats?.dr ?? 0) * 1024 * 1024
							},
							color: 1,
							opacity: 0.3,
						},
					]}
					tickFormatter={(val) => {
						const { value, unit } = formatBytes(val, true, userSettings.unitDisk, false)
						return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
					}}
					contentFormatter={({ value }) => {
						const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, false)
						return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
					}}
					showTotal={true}
				/>
			</ChartCard>
		</>
	)
}
