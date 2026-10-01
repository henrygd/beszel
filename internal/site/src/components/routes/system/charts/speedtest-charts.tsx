import LineChartDefault, { type DataPoint } from "@/components/charts/line-chart"
import { decimalString, formatBytes, matchesFilterGroups, parseFilterGroups, toFixedFloat } from "@/lib/utils"
import { Unit } from "@/lib/enums"
import type { SpeedtestCompareRecord } from "@/lib/speedtest-compare"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import type { $containerFilter } from "@/lib/stores"
import { ChartCard, FilterBar } from "../chart-card"
import type { ChartData, SpeedtestRecord, SpeedtestStatsRecord } from "@/types"
import { useCallback, useMemo } from "react"

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
	/** Times of failed runs in Unix ms, marked with a red line */
	failures: number[]
	chartData: ChartData
	empty: boolean
}

/** Tooltip line for a failed run */
function failureNote(record: SpeedtestStatsRecord) {
	if (!record?.error) return null
	return (
		<div className="border-t pt-1.5 max-w-64 text-wrap font-medium text-destructive">
			<Trans>Run failed</Trans>: {record.error}
		</div>
	)
}

// Failed runs arrive without measurements (see useSpeedtestStats), so the lines break there
// and a red line marks each one. Runs are sparse, and a line needs two neighboring values,
// so a dot marks each run.
function point(label: string, color: number | string, dataKey: DataPoint<SpeedtestStatsRecord>["dataKey"], order = 0) {
	return {
		label,
		color,
		dataKey,
		order,
		dot: true,
	} satisfies DataPoint<SpeedtestStatsRecord>
}

function SpeedtestBandwidthChart({
	stats,
	failures,
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
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				markers={failures}
				tooltipNote={failureNote}
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
		<SpeedtestBandwidthChart {...props} title={t`Upload`} description={t`Upload speed`} color={4} dataKey={uploadKey} />
	)
}

export function SpeedtestLatencyChart({ stats, failures, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [point(t`Ping`, 1, (record) => record.ping, 0), point(t`Jitter`, 3, (record) => record.jitter, 1)],
		[t]
	)
	return (
		<ChartCard empty={empty} title={t`Latency`} description={t`Idle ping and jitter`} grid={false} legend>
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				markers={failures}
				tooltipNote={failureNote}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				legend
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)} ms` : value)}
			/>
		</ChartCard>
	)
}

// Loaded latency and jitter are 0 when the CLI didn't report them.
const downloadLatencyKey = (record: SpeedtestStatsRecord) => record.download_latency || null
const downloadJitterKey = (record: SpeedtestStatsRecord) => record.download_jitter || null
const uploadLatencyKey = (record: SpeedtestStatsRecord) => record.upload_latency || null
const uploadJitterKey = (record: SpeedtestStatsRecord) => record.upload_jitter || null

/** Latency and jitter while the connection is loaded, like the idle latency chart. */
function SpeedtestLoadedLatencyChart({
	stats,
	failures,
	chartData,
	empty,
	title,
	description,
	latencyKey,
	jitterKey,
}: SpeedtestChartProps & {
	title: string
	description: string
	latencyKey: DataPoint<SpeedtestStatsRecord>["dataKey"]
	jitterKey: DataPoint<SpeedtestStatsRecord>["dataKey"]
}) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [point(t`Latency`, 1, latencyKey, 0), point(t`Jitter`, 3, jitterKey, 1)],
		[t, latencyKey, jitterKey]
	)
	return (
		<ChartCard empty={empty} title={title} description={description} grid={false} legend>
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				markers={failures}
				tooltipNote={failureNote}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				legend
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)} ms` : value)}
			/>
		</ChartCard>
	)
}

export function SpeedtestDownloadLatencyChart(props: SpeedtestChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestLoadedLatencyChart
			{...props}
			title={t`Download latency`}
			description={t`Latency and jitter while downloading (interquartile mean)`}
			latencyKey={downloadLatencyKey}
			jitterKey={downloadJitterKey}
		/>
	)
}

export function SpeedtestUploadLatencyChart(props: SpeedtestChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestLoadedLatencyChart
			{...props}
			title={t`Upload latency`}
			description={t`Latency and jitter while uploading (interquartile mean)`}
			latencyKey={uploadLatencyKey}
			jitterKey={uploadJitterKey}
		/>
	)
}

type SpeedtestCompareChartProps = {
	compareStats: SpeedtestCompareRecord[]
	speedtests: SpeedtestRecord[]
	getLabel: (speedtest: SpeedtestRecord) => string
	chartData: ChartData
	empty: boolean
	/** Scoped to the sheet so a filter doesn't carry over to other speedtests' sheets. */
	filterStore: typeof $containerFilter
}

/**
 * Color of a compared speedtest. Reds and pinks are left out, so the red lines that mark
 * failed runs stand out: the first four chart colors, or else hues spread from 35° to 315°.
 */
function compareColor(index: number, count: number) {
	if (count <= 4) return index + 1
	return `hsl(${35 + (index * 280) / count}, var(--chart-saturation), var(--chart-lightness))`
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
	const { dataPoints, visibleLabels } = useMemo(() => {
		const count = speedtests.length
		const filterGroups = parseFilterGroups(filter)
		// Segments of each speedtest that have runs; failed runs split a line into segments.
		const usedSegments = new Map<string, Set<number>>()
		for (const record of compareStats) {
			for (const id in record.segments) {
				const used = usedSegments.get(id) ?? new Set<number>()
				used.add(record.segments[id])
				usedSegments.set(id, used)
			}
		}
		const points: DataPoint<SpeedtestCompareRecord>[] = []
		// Labels of the charted speedtests by ID, to mark and name only their failed runs.
		const labels = new Map<string, string>()
		for (let i = 0; i < count; i++) {
			const speedtest = speedtests[i]
			const label = getLabel(speedtest)
			if (filterGroups.length > 0 && !matchesFilterGroups(label.toLowerCase(), filterGroups)) continue
			labels.set(speedtest.id, label)
			const color = compareColor(i, count)
			const segments = [...(usedSegments.get(speedtest.id) ?? [0])].sort((a, b) => a - b)
			for (const [n, segment] of segments.entries()) {
				points.push({
					id: `${speedtest.id}:${segment}`,
					order: i,
					label,
					dataKey: (record) => {
						const run = record.stats[speedtest.id]
						return run && record.segments[speedtest.id] === segment ? value(run) : null
					},
					dot: true,
					color,
					legend: n === 0,
				})
			}
		}
		return { dataPoints: points, visibleLabels: labels }
	}, [compareStats, speedtests, getLabel, filter, value])
	const legend = visibleLabels.size < 10

	const failures = useMemo(
		() =>
			compareStats
				.filter((record) => Object.keys(record.failures ?? {}).some((id) => visibleLabels.has(id)))
				.map((record) => record.created),
		[compareStats, visibleLabels]
	)
	// Several speedtests share the chart, so the note names each one that failed.
	const compareFailureNote = useCallback(
		(record: SpeedtestCompareRecord) => {
			const failed = Object.entries(record?.failures ?? {}).filter(([id]) => visibleLabels.has(id))
			if (!failed.length) return null
			return (
				<div className="border-t pt-1.5 max-w-64 text-wrap font-medium text-destructive grid gap-1">
					{failed.map(([id, error]) => (
						<div key={id}>
							<Trans>Run failed</Trans>: {visibleLabels.get(id)}: {error}
						</div>
					))}
				</div>
			)
		},
		[visibleLabels]
	)

	// Automatic speedtests can use a different server each run, so the tooltip names the run's server.
	const formatContent = useMemo(() => {
		const automaticIds = new Map(speedtests.filter((s) => !s.server_id).map((s) => [getLabel(s), s.id]))
		if (!automaticIds.size) return contentFormatter
		return (item: { value: number | string; name?: string; payload?: SpeedtestCompareRecord }) => {
			const formatted = contentFormatter(item)
			const id = automaticIds.get(item.name ?? "")
			const server = id ? item.payload?.stats[id]?.server_name : ""
			return server ? `${formatted} · ${server}` : formatted
		}
	}, [speedtests, getLabel, contentFormatter])

	return (
		<ChartCard
			legend={legend}
			cornerEl={<FilterBar store={filterStore} />}
			empty={empty}
			title={title}
			description={description}
			grid={false}
		>
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={compareStats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				// Speedtests run at different times, so each line joins its runs across the others'.
				// A failed run starts a new line for its speedtest, which breaks it there.
				connectNulls
				legend={legend}
				filter={filter}
				tickFormatter={tickFormatter}
				contentFormatter={formatContent}
				markers={failures}
				tooltipNote={compareFailureNote}
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

export function SpeedtestCompareDownloadLatencyChart(props: SpeedtestCompareChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestCompareChart
			{...props}
			title={t`Download latency`}
			description={t`Latency while downloading (interquartile mean)`}
			value={downloadLatencyKey}
			tickFormatter={msTick}
			contentFormatter={msContent}
		/>
	)
}

export function SpeedtestCompareUploadLatencyChart(props: SpeedtestCompareChartProps) {
	const { t } = useLingui()
	return (
		<SpeedtestCompareChart
			{...props}
			title={t`Upload latency`}
			description={t`Latency while uploading (interquartile mean)`}
			value={uploadLatencyKey}
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

export function SpeedtestLossChart({ stats, failures, chartData, empty }: SpeedtestChartProps) {
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
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				markers={failures}
				tooltipNote={failureNote}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)}%`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)}%` : value)}
			/>
		</ChartCard>
	)
}
