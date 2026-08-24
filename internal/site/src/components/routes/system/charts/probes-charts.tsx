import LineChartDefault from "@/components/charts/line-chart"
import type { DataPoint } from "@/components/charts/line-chart"
import { decimalString, formatMicroseconds, matchesFilterGroups, parseFilterGroups, toFixedFloat } from "@/lib/utils"
import { useLingui } from "@lingui/react/macro"
import { ChartCard, FilterBar } from "../chart-card"
import type { ChartData, NetworkProbeRecord, NetworkProbeStatsRecord } from "@/types"
import { useMemo } from "react"
import { atom } from "nanostores"
import { useStore } from "@nanostores/react"

const $filter = atom("")

type ProbeChartProps = {
	probeStats: NetworkProbeStatsRecord[]
	grid?: boolean
	probes: NetworkProbeRecord[]
	chartData: ChartData
	empty: boolean
	showFilter?: boolean
	/** Prepended to the chart title, e.g. a target/system name (rendered as "{titlePrefix} — Response"). */
	titlePrefix?: string
}

type ProbeChartBaseProps = ProbeChartProps & {
	valueIndex: number
	title: string
	description: string
	tickFormatter: (value: number) => string
	contentFormatter: ({ value }: { value: number | string }) => string | number
	domain?: [number | "auto", number | "auto"]
}

function ProbeChart({
	probeStats,
	grid,
	probes,
	chartData,
	empty,
	valueIndex,
	title,
	description,
	tickFormatter,
	contentFormatter,
	domain,
	showFilter = probes.length > 1,
}: ProbeChartBaseProps) {
	const storedFilter = useStore($filter)
	const filter = showFilter ? storedFilter : ""

	const { dataPoints, visibleKeys } = useMemo(() => {
		const sortedProbes = [...probes].sort((a, b) => b.resAvg1h - a.resAvg1h)
		const count = sortedProbes.length
		const points: DataPoint<NetworkProbeStatsRecord>[] = []
		const visibleIDs: string[] = []
		const filterGroups = parseFilterGroups(filter)
		const dot = chartData.chartTime === "1m"
		for (let i = 0; i < count; i++) {
			const p = sortedProbes[i]
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
				dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[p.id]?.[valueIndex] ?? null,
				dot,
				color: count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`,
			})
		}
		return { dataPoints: points, visibleKeys: visibleIDs }
	}, [probes, filter, valueIndex, chartData.chartTime])

	const filteredProbeStats = useMemo(() => {
		if (!visibleKeys.length) return probeStats
		return probeStats.filter((record) => visibleKeys.some((id) => record.stats?.[id] != null))
	}, [probeStats, visibleKeys])

	const legend = dataPoints.length < 10 && showFilter

	return (
		<ChartCard
			legend={legend || !showFilter}
			cornerEl={showFilter ? <FilterBar store={$filter} /> : undefined}
			empty={empty}
			title={title}
			description={description}
			grid={grid}
		>
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={filteredProbeStats}
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
	probeStats: NetworkProbeStatsRecord[]
	probe: NetworkProbeRecord | null
	chartData: ChartData
	empty: boolean
}

export function AvgMinMaxResponseChart({ probeStats, probe, chartData, empty }: AvgMinMaxResponseChartProps) {
	const { t } = useLingui()

	const { chartTime } = chartData
	const hasLongInterval = (probe?.interval ?? 61) > 60

	// only one probe is relevant for this chart
	const dataPoints: DataPoint<NetworkProbeStatsRecord>[] = useMemo(() => {
		const dataFn = (index: number) => (record: NetworkProbeStatsRecord) =>
			record.stats?.[probe?.id ?? ""]?.[index] ?? "-"
		const avgPoint = {
			label: "Avg",
			dataKey: dataFn(0),
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
				dataKey: dataFn(2),
				color: 3,
				order: 0,
			},
			avgPoint,
			{
				label: "Min",
				dataKey: dataFn(1),
				color: 2,
				order: 2,
			},
		]
	}, [chartTime, hasLongInterval])

	const data = useMemo(() => {
		if (!probe) return []
		return probeStats.filter((record) => record.stats && probe.id in record.stats)
	}, [probe, probeStats])

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

/** Combined response-time (left axis, solid) + packet-loss (right axis, dashed) chart, one pair of lines per probe. */
export function ResponseLossChart({ probeStats, grid, probes, chartData, empty, showFilter = probes.length > 1, titlePrefix }: ProbeChartProps) {
	const { t } = useLingui()
	const storedFilter = useStore($filter)
	const filter = showFilter ? storedFilter : ""
	const lossSuffix = t`Loss`

	const { dataPoints, visibleKeys } = useMemo(() => {
		const sortedProbes = [...probes].sort((a, b) => b.resAvg1h - a.resAvg1h)
		const count = sortedProbes.length
		const points: DataPoint<NetworkProbeStatsRecord>[] = []
		const visibleIDs: string[] = []
		const filterGroups = parseFilterGroups(filter)
		const dot = chartData.chartTime === "1m"
		for (let i = 0; i < count; i++) {
			const p = sortedProbes[i]
			const label = p.name || p.target
			const labelLower = label.toLowerCase()
			const filtered = filterGroups.length > 0 && !matchesFilterGroups(labelLower, filterGroups)
			if (filtered) {
				continue
			}
			visibleIDs.push(p.id)
			const color = count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`
			points.push({
				order: i,
				label,
				dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[p.id]?.[0] ?? null,
				dot,
				color,
				yAxisId: "left",
			})
			points.push({
				order: i,
				label: `${label} · ${lossSuffix}`,
				dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[p.id]?.[3] ?? null,
				dot,
				color: "var(--destructive)",
				yAxisId: "right",
				strokeDasharray: "4 3",
			})
		}
		return { dataPoints: points, visibleKeys: visibleIDs }
	}, [probes, filter, chartData.chartTime, lossSuffix])

	const filteredProbeStats = useMemo(() => {
		if (!visibleKeys.length) return probeStats
		return probeStats.filter((record) => visibleKeys.some((id) => record.stats?.[id] != null))
	}, [probeStats, visibleKeys])

	const legend = visibleKeys.length < 8 && showFilter

	const responseTitle = t`Response`
	const title = titlePrefix ?? `${responseTitle} · ${lossSuffix}`

	return (
		<ChartCard
			legend={legend || !showFilter}
			cornerEl={showFilter ? <FilterBar store={$filter} /> : undefined}
			empty={empty}
			title={title}
			description={t`Average response time (left) and packet loss (right)`}
			grid={grid}
		>
			<LineChartDefault
				truncate
				chartData={chartData}
				customData={filteredProbeStats}
				dataPoints={dataPoints}
				domain={["auto", "auto"]}
				domain2={[0, 100]}
				connectNulls
				tickFormatter={(value) => formatMicroseconds(value, false)}
				tickFormatter2={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)}%`}
				contentFormatter={(item: { value?: number | string }, key: string) => {
					const value = item?.value
					if (typeof value !== "number") {
						return value
					}
					return key.endsWith(lossSuffix) ? `${decimalString(value, 2)}%` : formatMicroseconds(value)
				}}
				legend={legend}
				filter={filter}
			/>
		</ChartCard>
	)
}

export function LossChart({ probeStats, grid, probes, chartData, empty, titlePrefix }: ProbeChartProps) {
	const { t } = useLingui()
	const lossTitle = t`Loss`
	const title = titlePrefix ? `${titlePrefix} — ${lossTitle}` : lossTitle

	return (
		<ProbeChart
			probeStats={probeStats}
			grid={grid}
			probes={probes}
			chartData={chartData}
			empty={empty}
			valueIndex={3}
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
