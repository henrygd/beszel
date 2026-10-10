import { getMonitorTarget, monitorGapRecord } from "@/lib/network-monitor-utils"
import LineChartDefault from "@/components/charts/line-chart"
import type { DataPoint } from "@/components/charts/line-chart"
import { decimalString, formatMicroseconds, matchesFilterGroups, parseFilterGroups, toFixedFloat } from "@/lib/utils"
import { $monitorFilter } from "@/lib/stores"
import { useLingui } from "@lingui/react/macro"
import { ChartCard, FilterBar } from "../chart-card"
import type { ChartOptions, MonitorStats, NetworkMonitorRecord, NetworkMonitorStatsRecord } from "@/types"
import { useMemo } from "react"
import { useStore } from "@nanostores/react"

type MonitorChartProps = {
	monitorStats: NetworkMonitorStatsRecord[]
	grid?: boolean
	monitors: NetworkMonitorRecord[]
	chartData: ChartOptions
	empty: boolean
	showFilter?: boolean
	/** Prepended to the chart title, e.g. a target/system name (rendered as "{titlePrefix} — Response"). */
	titlePrefix?: string
	/** Line label for each monitor. Defaults to the monitor target; use the system name when comparing systems. */
	getLabel?: (monitor: NetworkMonitorRecord) => string
	/** Filter store for the chart's filter bar. Pass a local atom to keep the filter scoped to one view. */
	filterStore?: typeof $monitorFilter
}

type MonitorChartBaseProps = MonitorChartProps & {
	metric: keyof MonitorStats
	title: string
	description: string
	tickFormatter: (value: number) => string
	contentFormatter: ({ value }: { value: number | string }) => string | number
	domain?: [number | "auto", number | "auto"]
	/** Overrides the per-monitor line colors (e.g. a fixed color for single-monitor charts). */
	color?: string
}

function MonitorChart({
	monitorStats,
	grid,
	monitors,
	chartData,
	empty,
	metric,
	title,
	description,
	tickFormatter,
	contentFormatter,
	domain,
	color,
	getLabel = getMonitorTarget,
	filterStore = $monitorFilter,
	showFilter = monitors.length > 1,
}: MonitorChartBaseProps) {
	const storedFilter = useStore(filterStore)
	const filter = showFilter ? storedFilter : ""

	const { dataPoints, visibleKeys } = useMemo(() => {
		const sortedMonitors = [...monitors].sort((a, b) => b.resAvg1h - a.resAvg1h)
		const count = sortedMonitors.length
		const points: DataPoint<NetworkMonitorStatsRecord>[] = []
		const visibleIDs: string[] = []
		const filterGroups = parseFilterGroups(filter)
		// show every point at 1m; otherwise the chart default draws only isolated points
		const dot = chartData.chartTime === "1m" || undefined
		for (let i = 0; i < count; i++) {
			const p = sortedMonitors[i]
			const label = getLabel(p)
			const labelLower = label.toLowerCase()
			const filtered = filterGroups.length > 0 && !matchesFilterGroups(labelLower, filterGroups)
			if (filtered) {
				continue
			}
			visibleIDs.push(p.id)
			points.push({
				order: i,
				label,
				dataKey: (record: NetworkMonitorStatsRecord) => record.stats?.[p.id]?.[metric] ?? null,
				dot,
				color:
					color ?? (count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`),
			})
		}
		return { dataPoints: points, visibleKeys: visibleIDs }
	}, [monitors, filter, metric, chartData.chartTime, color, getLabel])

	// Monitors with different intervals don't share timestamps, so multiple lines need connectNulls.
	// A single monitor's stats already contain empty records at real gaps, so the line breaks there.
	const multipleMonitors = visibleKeys.length > 1

	const filteredMonitorStats = useMemo(() => {
		if (!multipleMonitors) return monitorStats
		return monitorStats.filter((record) => visibleKeys.some((id) => record.stats?.[id] != null))
	}, [monitorStats, visibleKeys, multipleMonitors])

	const legend = dataPoints.length < 10 && showFilter

	return (
		<ChartCard
			legend={legend || !showFilter}
			cornerEl={showFilter ? <FilterBar store={filterStore} /> : undefined}
			empty={empty}
			title={title}
			description={description}
			grid={grid}
		>
			<LineChartDefault
				truncate
				chartData={{
					...chartData,
					dataScope: monitors
						.map((m) => m.id)
						.sort()
						.join(","),
				}}
				customData={filteredMonitorStats}
				dataPoints={dataPoints}
				domain={domain ?? ["auto", "auto"]}
				connectNulls={multipleMonitors}
				tickFormatter={tickFormatter}
				contentFormatter={contentFormatter}
				legend={legend}
				filter={filter}
			/>
		</ChartCard>
	)
}

interface AvgMinMaxResponseChartProps {
	monitorStats: NetworkMonitorStatsRecord[]
	monitor: NetworkMonitorRecord | null
	chartData: ChartOptions
	empty: boolean
}

export function AvgMinMaxResponseChart({ monitorStats, monitor, chartData, empty }: AvgMinMaxResponseChartProps) {
	const { t } = useLingui()

	const { chartTime } = chartData
	const hasLongInterval = (monitor?.interval ?? 61) > 60

	// only one monitor is relevant for this chart
	const dataPoints: DataPoint<NetworkMonitorStatsRecord>[] = useMemo(() => {
		const dataFn = (metric: keyof MonitorStats) => (record: NetworkMonitorStatsRecord) =>
			record.stats?.[monitor?.id ?? ""]?.[metric] ?? null
		const avgPoint = {
			label: "Avg",
			dataKey: dataFn("res_avg"),
			color: 1,
			order: 0,
		}
		if (chartTime === "1m" || (hasLongInterval && chartTime === "1h")) {
			// avg, min, max are all the same for 1m interval, so just show avg
			return [avgPoint]
		}
		return [
			{
				label: "Max",
				dataKey: dataFn("res_max"),
				color: 3,
				order: 0,
			},
			avgPoint,
			{
				label: "Min",
				dataKey: dataFn("res_min"),
				color: 2,
				order: 2,
			},
		]
	}, [chartTime, hasLongInterval, monitor?.id])

	// Replace records where every probe failed with gap markers, so the line breaks there without
	// leaving points that have no response time for the tooltip to show.
	const data = useMemo(() => {
		const id = monitor?.id ?? ""
		return monitorStats.map((record) =>
			record.stats?.[id] && record.stats[id].res_avg == null ? monitorGapRecord : record
		)
	}, [monitorStats, monitor?.id])

	const legend = dataPoints.length > 1

	return (
		<ChartCard
			legend={true}
			empty={empty}
			title={t`Response`}
			description={t`Average, minimum, and maximum response time`}
			grid={false}
		>
			<LineChartDefault
				truncate
				chartData={{ ...chartData, dataScope: monitor?.id }}
				tableData={monitorStats}
				customData={data}
				dataPoints={dataPoints}
				domain={["auto", "auto"]}
				legend={legend}
				tickFormatter={(value) => formatMicroseconds(value, false)}
				contentFormatter={({ value }) => {
					if (typeof value !== "number") {
						return value
					}
					return formatMicroseconds(value)
				}}
			/>
		</ChartCard>
	)
}

export function ResponseChart({
	monitorStats,
	grid,
	monitors,
	chartData,
	empty,
	titlePrefix,
	getLabel,
	filterStore,
	showFilter,
}: MonitorChartProps) {
	const { t } = useLingui()
	const responseTitle = t`Response`
	const title = titlePrefix ? `${titlePrefix} — ${responseTitle}` : responseTitle

	return (
		<MonitorChart
			monitorStats={monitorStats}
			grid={grid}
			monitors={monitors}
			chartData={chartData}
			empty={empty}
			metric="res_avg"
			title={title}
			description={t`Average response time`}
			getLabel={getLabel}
			filterStore={filterStore}
			showFilter={showFilter}
			tickFormatter={(value) => formatMicroseconds(value, false)}
			contentFormatter={({ value }) => {
				if (typeof value !== "number") {
					return value
				}
				return formatMicroseconds(value)
			}}
		/>
	)
}

export function LossChart({
	monitorStats,
	grid,
	monitors,
	chartData,
	empty,
	titlePrefix,
	getLabel,
	filterStore,
	showFilter,
}: MonitorChartProps) {
	const { t } = useLingui()
	const lossTitle = t({ message: "Loss", context: "Packet loss" })
	const title = titlePrefix ? `${titlePrefix} — ${lossTitle}` : lossTitle

	return (
		<MonitorChart
			monitorStats={monitorStats}
			grid={grid}
			monitors={monitors}
			chartData={chartData}
			empty={empty}
			metric="loss"
			title={title}
			description={t`Packet loss (%)`}
			domain={[0, 100]}
			// a single destructive color only makes sense for single-monitor charts
			color={monitors.length > 1 ? undefined : "var(--destructive)"}
			getLabel={getLabel}
			filterStore={filterStore}
			showFilter={showFilter}
			tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)}%`}
			contentFormatter={({ value }) => {
				if (typeof value !== "number") {
					return value
				}
				return `${decimalString(value, 2)}%`
			}}
		/>
	)
}
