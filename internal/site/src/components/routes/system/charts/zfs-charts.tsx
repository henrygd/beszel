import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { SystemStatsRecord } from "@/types"
import { ChartCard } from "../chart-card"
import { Unit } from "@/lib/enums"
import { useStore } from "@nanostores/react"
import { $userSettings } from "@/lib/stores"
import type { SystemData } from "../use-system-data"

// Accessors for ZFS metrics
const poolUsage =
	(name: string) =>
	({ stats }: SystemStatsRecord) =>
		stats?.z?.[name]?.du ?? 0
const poolRead =
	(name: string) =>
	({ stats }: SystemStatsRecord) =>
		stats?.z?.[name]?.rb ?? 0
const poolWrite =
	(name: string) =>
	({ stats }: SystemStatsRecord) =>
		stats?.z?.[name]?.wb ?? 0

export function ZfsPoolUsageChart({ systemData, poolName }: { systemData: SystemData; poolName: string }) {
	const { chartData, grid, dataEmpty } = systemData
	const latest = chartData.systemStats.at(-1)?.stats
	const pool = latest?.z?.[poolName]
	if (!pool) {
		return null
	}
	let poolTotal = pool.d
	// round to nearest GB
	if (poolTotal >= 100) {
		poolTotal = Math.round(poolTotal)
	}

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={`${poolName} ${t`Usage`}`}
			description={t`Usage of ZFS pool ${poolName}`}
		>
			<AreaChartDefault
				chartData={chartData}
				domain={[0, poolTotal]}
				showTotal={true}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val * 1024, false, Unit.Bytes, true)
					return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={({ value }) => {
					const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
					return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
				}}
				dataPoints={[
					{
						label: t`Pool Usage`,
						dataKey: poolUsage(poolName),
						color: 4,
						opacity: 0.4,
					},
				]}
			/>
		</ChartCard>
	)
}

export function ZfsPoolIOChart({ systemData, poolName }: { systemData: SystemData; poolName: string }) {
	const { chartData, grid, dataEmpty } = systemData
	const userSettings = useStore($userSettings)
	if (!chartData.systemStats?.length) {
		return null
	}
	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={`${poolName} I/O`}
			description={t`Throughput of ZFS pool ${poolName}`}
		>
			<AreaChartDefault
				chartData={chartData}
				showTotal={true}
				dataPoints={[
					{
						label: t({ message: "Write", comment: "Disk write" }),
						dataKey: poolWrite(poolName),
						color: 3,
						opacity: 0.3,
					},
					{
						label: t({ message: "Read", comment: "Disk read" }),
						dataKey: poolRead(poolName),
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
			/>
		</ChartCard>
	)
}

/** ZFS section: one stacked usage card per pool plus per-pool I/O cards. */
export function ZfsCharts({ systemData }: { systemData: SystemData }) {
	const latest = systemData.chartData.systemStats?.at(-1)?.stats
	const pools = latest?.z ?? {}
	if (Object.keys(pools).length === 0) {
		return null
	}
	return (
		<div className="grid xl:grid-cols-2 gap-4">
			{Object.keys(pools).map((poolName) => (
				<div key={poolName} className="contents">
					<ZfsPoolUsageChart systemData={systemData} poolName={poolName} />
					<ZfsPoolIOChart systemData={systemData} poolName={poolName} />
				</div>
			))}
		</div>
	)
}
