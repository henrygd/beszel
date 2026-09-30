import AreaChartDefault from "@/components/charts/area-chart"
import type { DataPoint } from "@/components/charts/area-chart"
import { decimalString, formatBytes, matchesFilterGroups, parseFilterGroups, toFixedFloat } from "@/lib/utils"
import { Unit } from "@/lib/enums"
import type { SpeedtestCompareRecord } from "@/lib/speedtest-compare"
import { useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import type { WritableAtom } from "nanostores"
import { ChartCard, FilterBar } from "../chart-card"
import type { ChartData, SpeedtestRecord, SpeedtestStatsRecord } from "@/types"
import { useMemo } from "react"

/**
 * Format a bandwidth in bytes/s as bits per second, the unit speedtests are usually quoted in.
 * The short form is for axis ticks and uses a non-breaking space, since recharts measures tick
 * words without the axis font styles and would otherwise wrap wide labels like "840 Mbps".
 */
export function formatBandwidth(bytesPerSecond: number, short = false) {
	const { value, unit } = formatBytes(bytesPerSecond, true, Unit.Bits, false)
	if (short) return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
	return `${decimalString(value, value >= 100 ? 1 : 2)} ${unit}`
}

type SpeedtestChartProps = {
	stats: SpeedtestStatsRecord[]
	chartData: ChartData
	empty: boolean
}

// Series overlap rather than stack, so a translucent fill keeps each one visible.
// Failed runs arrive as gap markers (see useSpeedtestStats). Runs are sparse, and an area
// needs two neighboring values, so a dot marks each run.
function point(label: string, color: number | string, dataKey: DataPoint<SpeedtestStatsRecord>["dataKey"], order = 0) {
	return {
		label,
		color,
		dataKey,
		order,
		opacity: 0.2,
		dot: true,
	} satisfies DataPoint<SpeedtestStatsRecord>
}

function SpeedtestBandwidthChart({
	stats,
	chartData,
	empty,
	title,
	description,
	color,
	dataKey,
}: SpeedtestChartProps & {
	title: string
	description: string
	color: number
	dataKey: DataPoint<SpeedtestStatsRecord>["dataKey"]
}) {
	const dataPoints = useMemo(() => [point(title, color, dataKey)], [title, color, dataKey])
	return (
		<ChartCard empty={empty} title={title} description={description} grid={false}>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				tickFormatter={(value) => formatBandwidth(value, true)}
				contentFormatter={({ value }) => (typeof value === "number" ? formatBandwidth(value) : value)}
			/>
		</ChartCard>
	)
}

const downloadKey = (record: SpeedtestStatsRecord) => record.download
const uploadKey = (record: SpeedtestStatsRecord) => record.upload

export function SpeedtestDownloadChart(props: SpeedtestChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestBandwidthChart
			{...props}
			title={t`Download`}
			description={t`Download speed`}
			color={2}
			dataKey={downloadKey}
		/>
	)
}

export function SpeedtestUploadChart(props: SpeedtestChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestBandwidthChart {...props} title={t`Upload`} description={t`Upload speed`} color={5} dataKey={uploadKey} />
	)
}

export function SpeedtestLatencyChart({ stats, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [point(t`Ping`, 1, (record) => record.ping, 0), point(t`Jitter`, 3, (record) => record.jitter, 1)],
		[t]
	)
	return (
		<ChartCard empty={empty} title={t`Latency`} description={t`Idle ping and jitter`} grid={false} legend>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				legend
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)} ms` : value)}
			/>
		</ChartCard>
	)
}

export function SpeedtestLoadedLatencyChart({ stats, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [
			point(t`Download`, 2, (record) => record.download_latency || null, 0),
			point(t`Upload`, 5, (record) => record.upload_latency || null, 1),
		],
		[t]
	)
	return (
		<ChartCard
			empty={empty}
			title={t`Loaded latency`}
			description={t`Latency while downloading and uploading (interquartile mean)`}
			grid={false}
			legend
		>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				legend
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)} ms` : value)}
			/>
		</ChartCard>
	)
}

type SpeedtestCompareChartProps = {
	compareStats: SpeedtestCompareRecord[]
	speedtests: SpeedtestRecord[]
	getLabel: (speedtest: SpeedtestRecord) => string
	chartData: ChartData
	empty: boolean
	/** Scoped to the sheet so a filter doesn't carry over to other speedtests' sheets. */
	filterStore: WritableAtom<string>
}

/** One line per compared speedtest, for a single measurement. */
function SpeedtestCompareChart({
	compareStats,
	speedtests,
	getLabel,
	chartData,
	empty,
	filterStore,
	title,
	description,
	value,
	tickFormatter,
	contentFormatter,
}: SpeedtestCompareChartProps & {
	title: string
	description: string
	value: (run: SpeedtestStatsRecord) => number | null
	tickFormatter: (value: number) => string
	contentFormatter: ({ value }: { value: number | string }) => string | number
}) {
	const filter = useStore(filterStore)
	const dataPoints = useMemo(() => {
		const count = speedtests.length
		const filterGroups = parseFilterGroups(filter)
		const points: DataPoint<SpeedtestCompareRecord>[] = []
		for (let i = 0; i < count; i++) {
			const speedtest = speedtests[i]
			const label = getLabel(speedtest)
			if (filterGroups.length > 0 && !matchesFilterGroups(label.toLowerCase(), filterGroups)) continue
			points.push({
				order: i,
				label,
				dataKey: (record) => {
					const run = record.stats[speedtest.id]
					return run ? value(run) : null
				},
				opacity: 0.2,
				dot: true,
				color: count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`,
			})
		}
		return points
	}, [speedtests, getLabel, filter, value])
	const legend = dataPoints.length < 10

	return (
		<ChartCard
			legend={legend}
			cornerEl={<FilterBar store={filterStore} />}
			empty={empty}
			title={title}
			description={description}
			grid={false}
		>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={compareStats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				// Speedtests run at different times, so each area joins its runs across the others'.
				connectNulls
				legend={legend}
				filter={filter}
				tickFormatter={tickFormatter}
				contentFormatter={contentFormatter}
			/>
		</ChartCard>
	)
}

const compareDownload = (run: SpeedtestStatsRecord) => run.download
const compareUpload = (run: SpeedtestStatsRecord) => run.upload
const comparePing = (run: SpeedtestStatsRecord) => run.ping
// Servers that don't measure packet loss report -1.
const compareLoss = (run: SpeedtestStatsRecord) => (run.loss >= 0 ? run.loss : null)
const bandwidthTick = (value: number) => formatBandwidth(value, true)
const bandwidthContent = ({ value }: { value: number | string }) =>
	typeof value === "number" ? formatBandwidth(value) : value
const msTick = (value: number) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`
const msContent = ({ value }: { value: number | string }) =>
	typeof value === "number" ? `${decimalString(value, 2)} ms` : value
const percentTick = (value: number) => `${toFixedFloat(value, value >= 10 ? 0 : 1)}%`
const percentContent = ({ value }: { value: number | string }) =>
	typeof value === "number" ? `${decimalString(value, 2)}%` : value

export function SpeedtestCompareDownloadChart(props: SpeedtestCompareChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestCompareChart
			{...props}
			title={t`Download`}
			description={t`Download speed`}
			value={compareDownload}
			tickFormatter={bandwidthTick}
			contentFormatter={bandwidthContent}
		/>
	)
}

export function SpeedtestCompareUploadChart(props: SpeedtestCompareChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestCompareChart
			{...props}
			title={t`Upload`}
			description={t`Upload speed`}
			value={compareUpload}
			tickFormatter={bandwidthTick}
			contentFormatter={bandwidthContent}
		/>
	)
}

export function SpeedtestComparePingChart(props: SpeedtestCompareChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestCompareChart
			{...props}
			title={t`Ping`}
			description={t`Idle ping`}
			value={comparePing}
			tickFormatter={msTick}
			contentFormatter={msContent}
		/>
	)
}

export function SpeedtestCompareLossChart(props: SpeedtestCompareChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestCompareChart
			{...props}
			title={t({ message: "Loss", context: "Packet loss" })}
			description={t`Packet loss (%)`}
			value={compareLoss}
			tickFormatter={percentTick}
			contentFormatter={percentContent}
		/>
	)
}

export function SpeedtestLossChart({ stats, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [
			// Servers that don't measure packet loss report -1.
			point(t({ message: "Loss", context: "Packet loss" }), "var(--destructive)", (record) =>
				record.loss >= 0 ? record.loss : null
			),
		],
		[t]
	)
	return (
		<ChartCard
			empty={empty}
			title={t({ message: "Loss", context: "Packet loss" })}
			description={t`Packet loss (%)`}
			grid={false}
		>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)}%`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)}%` : value)}
			/>
		</ChartCard>
	)
}
