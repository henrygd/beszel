import LineChartDefault from "@/components/charts/line-chart"
import type { DataPoint } from "@/components/charts/line-chart"
import { decimalString, formatMicroseconds, matchesFilterGroups, parseFilterGroups, toFixedFloat } from "@/lib/utils"
import { $monitorFilter } from "@/lib/stores"
import { useLingui } from "@lingui/react/macro"
import { ChartCard, FilterBar } from "../chart-card"
import type { ChartData, MonitorStats, NetworkMonitorRecord, NetworkMonitorStatsRecord } from "@/types"
import { useMemo } from "react"
import { useStore } from "@nanostores/react"

type MonitorChartProps = {
	monitorStats: NetworkMonitorStatsRecord[]
	grid?: boolean
	monitors: NetworkMonitorRecord[]
	chartData: ChartData
	empty: boolean
	showFilter?: boolean
	/** Prepended to the chart title, e.g. a target/system name (rendered as "{titlePrefix} — Response"). */
	titlePrefix?: string
}

type MonitorChartBaseProps = MonitorChartProps & {
	metric: keyof MonitorStats
	title: string
	description: string
	tickFormatter: (value: number) => string
	contentFormatter: ({ value }: { value: number | string }) => string | number
	domain?: [number | "auto", number | "auto"]
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
	showFilter = monitors.length > 1,
}: MonitorChartBaseProps) {
	const storedFilter = useStore($monitorFilter)
	const filter = showFilter ? storedFilter : ""

	const { dataPoints, visibleKeys } = useMemo(() => {
		const sortedMonitors = [...monitors].sort((a, b) => b.resAvg1h - a.resAvg1h)
		const count = sortedMonitors.length
		const points: DataPoint<NetworkMonitorStatsRecord>[] = []
		const visibleIDs: string[] = []
		const filterGroups = parseFilterGroups(filter)
		const dot = chartData.chartTime === "1m"
		for (let i = 0; i < count; i++) {
			const p = sortedMonitors[i]
			const label = p.name || p.target
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
				color: count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`,
			})
		}
		return { dataPoints: points, visibleKeys: visibleIDs }
	}, [monitors, filter, metric, chartData.chartTime])

	const filteredMonitorStats = useMemo(() => {
		if (!visibleKeys.length) return monitorStats
		return monitorStats.filter((record) => visibleKeys.some((id) => record.stats?.[id] != null))
	}, [monitorStats, visibleKeys])

	const legend = dataPoints.length < 10 && showFilter

	return (
		<ChartCard
			legend={legend || !showFilter}
			cornerEl={showFilter ? <FilterBar store={$monitorFilter} /> : undefined}
			empty={empty}
			title={title}
			description={description}
			grid={grid}
		>
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={filteredMonitorStats}
				dataPoints={dataPoints}
				domain={domain ?? ["auto", "auto"]}
				connectNulls
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
	chartData: ChartData
	empty: boolean
}

export function AvgMinMaxResponseChart({ monitorStats, monitor, chartData, empty }: AvgMinMaxResponseChartProps) {
	const { t } = useLingui()

	const { chartTime } = chartData
	const hasLongInterval = (monitor?.interval ?? 61) > 60

	// only one monitor is relevant for this chart
	const dataPoints: DataPoint<NetworkMonitorStatsRecord>[] = useMemo(() => {
		const dataFn = (metric: keyof MonitorStats) => (record: NetworkMonitorStatsRecord) =>
			record.stats?.[monitor?.id ?? ""]?.[metric] ?? "-"
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

	const data = useMemo(() => {
		if (!monitor) return []
		return monitorStats.filter((record) => record.stats && monitor.id in record.stats)
	}, [monitor, monitorStats])

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
				chartData={chartData}
				customData={data}
				dataPoints={dataPoints}
				domain={["auto", "auto"]}
				connectNulls
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

export function LossChart({ monitorStats, grid, monitors, chartData, empty, titlePrefix }: MonitorChartProps) {
	const { t } = useLingui()
	const lossTitle = t`Loss`
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
