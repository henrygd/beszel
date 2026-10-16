import LineChartDefault, { type DataPoint } from "@/components/charts/line-chart"
import { decimalString, formatBytes, matchesFilterGroups, parseFilterGroups, toFixedFloat } from "@/lib/utils"
import { Unit } from "@/lib/enums"
import type { SpeedtestChartRecord } from "@/lib/speedtest-compare"
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
	if (short) return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
	return `${decimalString(value, value >= 100 ? 1 : 2)} ${unit}`
}

/** Axis tick and tooltip formatters for a metric's values. */
type ValueFormat = {
	tick: (value: number) => string
	content: ({ value }: { value: number | string }) => string | number
}

const tickNumber = (value: number) => toFixedFloat(value, value >= 10 ? 0 : 1)

const bandwidthFormat: ValueFormat = {
	tick: (value) => formatBandwidth(value, true),
	content: ({ value }) => (typeof value === "number" ? formatBandwidth(value) : value),
}
const msFormat: ValueFormat = {
	tick: (value) => `${tickNumber(value)} ms`,
	content: ({ value }) => (typeof value === "number" ? `${decimalString(value, 2)} ms` : value),
}
const percentFormat: ValueFormat = {
	tick: (value) => `${tickNumber(value)}%`,
	content: ({ value }) => (typeof value === "number" ? `${decimalString(value, 2)}%` : value),
}

const download = (run: SpeedtestStatsRecord) => run.download
const upload = (run: SpeedtestStatsRecord) => run.upload
const ping = (run: SpeedtestStatsRecord) => run.ping
const jitter = (run: SpeedtestStatsRecord) => run.jitter
// Loaded latency and jitter are 0 when the CLI didn't report them.
const downloadLatency = (run: SpeedtestStatsRecord) => run.download_latency || null
const downloadJitter = (run: SpeedtestStatsRecord) => run.download_jitter || null
const uploadLatency = (run: SpeedtestStatsRecord) => run.upload_latency || null
const uploadJitter = (run: SpeedtestStatsRecord) => run.upload_jitter || null
// Servers that don't measure packet loss report -1.
const loss = (run: SpeedtestStatsRecord) => (run.loss >= 0 ? run.loss : null)

type SpeedtestSeries = {
	label: string
	color: number | string
	value: (run: SpeedtestStatsRecord) => number | null
}

/** One speedtest chart: its series for a single speedtest, of which comparisons chart the first. */
type SpeedtestMetric = {
	key: string
	title: string
	description: string
	/** Title and description when comparing, where only the first series is charted. */
	compare?: { title?: string; description?: string }
	format: ValueFormat
	series: SpeedtestSeries[]
}

function useSpeedtestMetrics(): SpeedtestMetric[] {
	const { t } = useLingui()
	return useMemo(() => {
		const lossTitle = t({ message: "Loss", context: "Packet loss" })
		return [
			{
				key: "download",
				title: t`Download`,
				description: t`Download speed`,
				format: bandwidthFormat,
				series: [{ label: t`Download`, color: 2, value: download }],
			},
			{
				key: "upload",
				title: t`Upload`,
				description: t`Upload speed`,
				format: bandwidthFormat,
				series: [{ label: t`Upload`, color: 4, value: upload }],
			},
			{
				key: "latency",
				title: t`Latency`,
				description: t`Idle ping and jitter`,
				compare: { title: t`Ping`, description: t`Idle ping` },
				format: msFormat,
				series: [
					{ label: t`Ping`, color: 1, value: ping },
					{ label: t`Jitter`, color: 3, value: jitter },
				],
			},
			{
				key: "download_latency",
				title: t`Download latency`,
				description: t`Latency and jitter while downloading (interquartile mean)`,
				compare: { description: t`Latency while downloading (interquartile mean)` },
				format: msFormat,
				series: [
					{ label: t`Latency`, color: 1, value: downloadLatency },
					{ label: t`Jitter`, color: 3, value: downloadJitter },
				],
			},
			{
				key: "upload_latency",
				title: t`Upload latency`,
				description: t`Latency and jitter while uploading (interquartile mean)`,
				compare: { description: t`Latency while uploading (interquartile mean)` },
				format: msFormat,
				series: [
					{ label: t`Latency`, color: 1, value: uploadLatency },
					{ label: t`Jitter`, color: 3, value: uploadJitter },
				],
			},
			{
				key: "loss",
				title: lossTitle,
				description: t`Packet loss (%)`,
				format: percentFormat,
				series: [{ label: lossTitle, color: "var(--destructive)", value: loss }],
			},
		]
	}, [t])
}

/**
 * Color of a compared speedtest. Reds and pinks are left out, so the red lines that mark
 * failed runs stand out: the first four chart colors, or else hues spread from 35° to 315°.
 */
function compareColor(index: number, count: number) {
	if (count <= 4) return index + 1
	return `hsl(${35 + (index * 280) / count}, var(--chart-saturation), var(--chart-lightness))`
}

type SpeedtestChartsProps = {
	stats: SpeedtestChartRecord[]
	/** Speedtests to chart, starting with the opened one. Several are compared with one line each. */
	speedtests: SpeedtestRecord[]
	getLabel: (speedtest: SpeedtestRecord) => string
	chartData: ChartData
	empty: boolean
	/** Scoped to the sheet so a filter doesn't carry over to other speedtests' sheets. */
	filterStore: typeof $containerFilter
}

/**
 * One speedtest metric. A single speedtest gets a line per series of the metric; compared
 * speedtests get a line each for its first series.
 */
function SpeedtestChart({
	metric,
	stats,
	speedtests,
	getLabel,
	chartData,
	empty,
	filterStore,
}: SpeedtestChartsProps & { metric: SpeedtestMetric }) {
	const comparing = speedtests.length > 1
	const storedFilter = useStore(filterStore)
	const filter = comparing ? storedFilter : ""
	const { title, description } = comparing ? { ...metric, ...metric.compare } : metric

	const { dataPoints, labels } = useMemo(() => {
		const filterGroups = parseFilterGroups(filter)
		// Segments of each speedtest that have runs; failed and missed runs split a line into segments.
		const usedSegments = new Map<string, Set<number>>()
		for (const record of stats) {
			for (const id in record.segments) {
				const used = usedSegments.get(id) ?? new Set<number>()
				used.add(record.segments[id])
				usedSegments.set(id, used)
			}
		}
		const points: DataPoint<SpeedtestChartRecord>[] = []
		// Labels of the charted speedtests by ID, to mark and name only their failed runs.
		const labels = new Map<string, string>()
		for (const [i, speedtest] of speedtests.entries()) {
			const label = getLabel(speedtest)
			if (filterGroups.length > 0 && !matchesFilterGroups(label.toLowerCase(), filterGroups)) continue
			labels.set(speedtest.id, label)
			const series = comparing
				? [{ ...metric.series[0], label, color: compareColor(i, speedtests.length) }]
				: metric.series
			const segments = [...(usedSegments.get(speedtest.id) ?? [0])].sort((a, b) => a - b)
			for (const [s, line] of series.entries()) {
				for (const [n, segment] of segments.entries()) {
					points.push({
						id: `${speedtest.id}:${s}:${segment}`,
						order: comparing ? i : s,
						label: line.label,
						dataKey: (record) => {
							const run = record.stats[speedtest.id]
							return run && record.segments[speedtest.id] === segment ? line.value(run) : null
						},
						dot: true,
						color: line.color,
						legend: n === 0,
					})
				}
			}
		}
		return { dataPoints: points, labels }
	}, [stats, speedtests, getLabel, filter, metric, comparing])
	const legend = comparing ? labels.size < 10 : metric.series.length > 1

	const failures = useMemo(
		() =>
			stats
				.filter((record) => Object.keys(record.failures ?? {}).some((id) => labels.has(id)))
				.map((record) => record.created),
		[stats, labels]
	)
	// When comparing, the note names each speedtest that failed.
	const failureNote = useCallback(
		(record: SpeedtestChartRecord) => {
			const failed = Object.entries(record?.failures ?? {}).filter(([id]) => labels.has(id))
			if (!failed.length) return null
			return (
				<div className="border-t pt-1.5 max-w-64 text-wrap font-medium text-destructive grid gap-1">
					{failed.map(([id, error]) => (
						<div key={id}>
							<Trans>Run failed</Trans>: {comparing && `${labels.get(id)}: `}
							{error}
						</div>
					))}
				</div>
			)
		},
		[labels, comparing]
	)

	// Automatic speedtests can use a different server each run, so a comparison's tooltip names the run's server.
	const formatContent = useMemo(() => {
		const automaticIds = new Map(speedtests.filter((s) => !s.server_id).map((s) => [getLabel(s), s.id]))
		if (!comparing || !automaticIds.size) return metric.format.content
		return (item: { value: number | string; name?: string; payload?: SpeedtestChartRecord }) => {
			const formatted = metric.format.content(item)
			const id = automaticIds.get(item.name ?? "")
			const server = id ? item.payload?.stats[id]?.server_name : ""
			return server ? `${formatted} · ${server}` : formatted
		}
	}, [speedtests, getLabel, metric, comparing])

	return (
		<ChartCard
			legend={legend}
			cornerEl={comparing ? <FilterBar store={filterStore} /> : undefined}
			empty={empty}
			title={title}
			description={description}
			grid={false}
		>
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				// Speedtests run at different times, so each line joins its runs across the others'.
				// Failed and missed runs start a new segment, which breaks the line there.
				connectNulls
				legend={legend}
				filter={filter}
				tickFormatter={metric.format.tick}
				contentFormatter={formatContent}
				markers={failures}
				tooltipNote={failureNote}
			/>
		</ChartCard>
	)
}

/** All speedtest charts, for one speedtest or several to compare. */
export function SpeedtestCharts(props: SpeedtestChartsProps) {
	const metrics = useSpeedtestMetrics()
	return (
		<>
			{metrics.map((metric) => (
				<SpeedtestChart key={metric.key} metric={metric} {...props} />
			))}
		</>
	)
}
