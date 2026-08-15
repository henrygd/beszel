import { t } from "@lingui/core/macro"
import AreaChartDefault, { type DataPoint } from "@/components/charts/area-chart"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { SystemStatsRecord, ZfsDatasetUsage } from "@/types"
import { ChartCard } from "../chart-card"
import { Unit } from "@/lib/enums"
import { useStore } from "@nanostores/react"
import { $userSettings } from "@/lib/stores"
import type { SystemData } from "../use-system-data"

// Accessors for ZFS metrics
const datasetUsage = (name: string) => ({ stats }: SystemStatsRecord) => stats?.zd?.[name]?.du ?? 0
const poolRead = (name: string) => ({ stats }: SystemStatsRecord) => stats?.z?.[name]?.rb ?? 0
const poolWrite = (name: string) => ({ stats }: SystemStatsRecord) => stats?.z?.[name]?.wb ?? 0

// poolLeafDatasets returns the leaf datasets of a pool, sorted. Parent
// datasets' `used` already includes their children, so graphing both would
// double-count space.
function poolLeafDatasets(poolName: string, datasets: Record<string, ZfsDatasetUsage>): string[] {
	const names = Object.keys(datasets)
		.filter((name) => name === poolName || name.startsWith(`${poolName}/`))
		.sort()
	return names.filter((name) => !names.some((other) => other.startsWith(`${name}/`)))
}

// poolOtherUsage is the pool's used space not attributable to any leaf dataset
// (root dataset data, snapshots, reserved), making the stacked total equal the
// pool's overall used.
const poolOtherUsage =
	(poolName: string) =>
	({ stats }: SystemStatsRecord) => {
		const pool = stats?.z?.[poolName]
		const datasets = stats?.zd
		if (!pool || !datasets) {
			return null
		}
		let sum = 0
		for (const name of poolLeafDatasets(poolName, datasets)) {
			sum += datasets[name].du
		}
		return Math.max(pool.du - sum, 0)
	}

export function ZfsPoolUsageChart({ systemData, poolName }: { systemData: SystemData; poolName: string }) {
	const { chartData, grid, dataEmpty } = systemData
	const latest = chartData.systemStats.at(-1)?.stats
	const pool = latest?.z?.[poolName]
	if (!pool) {
		return null
	}
	const datasets = latest?.zd ?? {}
	const leaves = poolLeafDatasets(poolName, datasets)
	const latestOther = Math.max(
		pool.du - leaves.reduce((sum, name) => sum + (datasets[name]?.du ?? 0), 0),
		0
	)

	const dataPoints: DataPoint[] = leaves.map((name, i) => ({
		label: name,
		dataKey: datasetUsage(name),
		color: `hsl(${226 + (((i * 360) / Math.max(leaves.length, 1)) % 360)}, 65%, 50%)`,
		opacity: 0.45,
		stackId: "1",
		order: i + 1,
	}))
	if (latestOther > 0) {
		dataPoints.unshift({
			label: t`Other`,
			dataKey: poolOtherUsage(poolName),
			color: "hsla(0 0% 50% / 0.6)",
			opacity: 0.4,
			stackId: "1",
			order: 0,
		})
	}

	let poolTotal = pool.d
	// round to nearest GB
	if (poolTotal >= 100) {
		poolTotal = Math.round(poolTotal)
	}

	return (
		<ChartCard empty={dataEmpty} grid={grid} title={`${poolName} ${t`Usage`}`} description={t`Usage of ZFS pool ${poolName}`}>
			<AreaChartDefault
				chartData={chartData}
				domain={[0, poolTotal]}
				showTotal={true}
				truncate={true}
				itemSorter={(a: { value: number }, b: { value: number }) => b.value - a.value}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val * 1024, false, Unit.Bytes, true)
					return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={({ value }) => {
					const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
					return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
				}}
				dataPoints={dataPoints}
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
		<ChartCard empty={dataEmpty} grid={grid} title={`${poolName} I/O`} description={t`Throughput of ZFS pool ${poolName}`}>
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
