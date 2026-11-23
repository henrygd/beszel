import { useLingui } from "@lingui/react/macro"
import { useMemo } from "react"
import LineChartDefault, { type DataPoint } from "@/components/charts/line-chart"
import { memoryBandwidthTotal } from "@/lib/memory-bandwidth"
import { decimalString, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, SelectAvgMax } from "../chart-card"

/** Memory bandwidth in GB/s: system memory read, write and total plus one line per GPU.
 *  Sources without a read/write split (mbwt) show only the total.
 *  GPU values estimated from memory busy time (bwa) are prefixed with "~".
 *  GPU bandwidth is often an order of magnitude above system RAM, so GPU lines use the
 *  right axis when both are present. There is no GPU max field, so GPU lines are hidden
 *  while the max view is selected. */
export function MemoryBandwidthChart({
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
	const { t } = useLingui()
	// Derive a stable series key (cheap per render): CPU presence plus GPU ids/names/approx flag
	let hasCpu = false
	let totalOnly = false
	const gpus = new Map<string, string>()
	for (const record of chartData.systemStats) {
		const stats = record.stats
		if (!stats) continue
		if (stats.mbw) hasCpu = true
		if (stats.mbwt) totalOnly = true
		for (const id in stats.g ?? {}) {
			const gpu = stats.g?.[id]
			if (gpu?.bwp) gpus.set(id, `${gpu.bwa ? "~" : ""}${gpu.n}`)
		}
	}
	const seriesKey = `${hasCpu}\0${totalOnly}\0${[...gpus].map(([id, label]) => `${id}\u0001${label}`).join("\0")}`

	const dataPoints = useMemo((): DataPoint[] => {
		const points: DataPoint[] = []
		const cpu = (stats: SystemStatsRecord["stats"]) => (showMax ? (stats?.mbwm ?? stats?.mbw) : stats?.mbw)
		if (hasCpu && totalOnly) {
			points.push({
				label: t`Total`,
				dataKey: ({ stats }: SystemStatsRecord) => memoryBandwidthTotal(stats, showMax),
				color: 3,
			})
		} else if (hasCpu) {
			points.push(
				{ label: t`Read`, dataKey: ({ stats }: SystemStatsRecord) => cpu(stats)?.[0], color: 1 },
				{ label: t`Write`, dataKey: ({ stats }: SystemStatsRecord) => cpu(stats)?.[1], color: 2 }
			)
			// read and write peaks need not coincide, so their sum is not a max total
			if (!showMax) {
				points.push({
					label: t`Total`,
					dataKey: ({ stats }: SystemStatsRecord) => memoryBandwidthTotal(stats, false),
					color: 3,
				})
			}
		}
		// no GPU max field exists: hide GPU averages rather than mixing them with CPU maxima
		const showGpus = !(showMax && hasCpu)
		let i = 0
		for (const [id, label] of showGpus ? gpus : []) {
			points.push({
				label,
				// a present peak means the GPU was sampled; a missing value is then 0
				dataKey: ({ stats }: SystemStatsRecord) => {
					const gpu = stats?.g?.[id]
					return gpu?.bwp ? (gpu.bw ?? 0) : undefined
				},
				color: `hsl(${226 + (((i++ * 360) / gpus.size) % 360)}, 65%, 52%)`,
				yAxisId: hasCpu ? "right" : "left",
			})
		}
		return points
	}, [seriesKey, showMax, t])

	if (dataPoints.length === 0) return null

	const formatTick = (val: number) => `${toFixedFloat(val, val >= 10 ? 0 : 1)} GB/s`

	const hasApprox = [...gpus.values()].some((label) => label.startsWith("~"))
	const gpuOnRight = dataPoints.some((dp) => dp.yAxisId === "right")
	let description = hasApprox
		? t`Memory throughput in GB/s. ~ marks GPU values estimated from memory busy time.`
		: t`Memory throughput in GB/s`
	if (gpuOnRight) description += ` ${t`GPU lines use the right axis.`}`

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Memory Bandwidth`}
			description={description}
			cornerEl={isLongerChart && hasCpu ? <SelectAvgMax max={maxValues} /> : null}
		>
			<LineChartDefault
				chartData={chartData}
				dataPoints={dataPoints}
				maxToggled={showMax}
				legend={dataPoints.length > 1 || totalOnly}
				tickFormatter={formatTick}
				tickFormatter2={formatTick}
				contentFormatter={({ value }) => `${decimalString(value, value >= 100 ? 1 : 2)} GB/s`}
			/>
		</ChartCard>
	)
}
