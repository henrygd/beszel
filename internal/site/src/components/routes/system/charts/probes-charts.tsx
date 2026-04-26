import LineChartDefault from "@/components/charts/line-chart"
import type { DataPoint } from "@/components/charts/line-chart"
import { decimalString, formatMicroseconds, toFixedFloat } from "@/lib/utils"
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
}: ProbeChartBaseProps) {
	const filter = useStore($filter)

	const { dataPoints, visibleKeys } = useMemo(() => {
		const sortedProbes = [...probes].sort((a, b) => b.resAvg1h - a.resAvg1h)
		const count = sortedProbes.length
		const points: DataPoint<NetworkProbeStatsRecord>[] = []
		const visibleIDs: string[] = []
		const filterTerms = filter
			? filter
					.toLowerCase()
					.split(" ")
					.filter((term) => term.length > 0)
			: []
		const dot = chartData.chartTime === "1m"
		for (let i = 0; i < count; i++) {
			const p = sortedProbes[i]
			const label = p.name || p.target
			const filtered = filterTerms.length > 0 && !filterTerms.some((term) => label.toLowerCase().includes(term))
			if (filtered) {
				continue
			}
			visibleIDs.push(p.id)
			points.push({
				order: i,
				label,
				dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[p.id]?.[valueIndex] ?? "-",
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

	const legend = dataPoints.length < 10

	return (
		<ChartCard
			legend={legend}
			cornerEl={<FilterBar store={$filter} />}
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

export function ResponseChart({ probeStats, grid, probes, chartData, empty }: ProbeChartProps) {
	const { t } = useLingui()

	return (
		<ProbeChart
			probeStats={probeStats}
			grid={grid}
			probes={probes}
			chartData={chartData}
			empty={empty}
			valueIndex={0}
			title={t`Response`}
			description={t`Average response time`}
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

export function LossChart({ probeStats, grid, probes, chartData, empty }: ProbeChartProps) {
	const { t } = useLingui()

	return (
		<ProbeChart
			probeStats={probeStats}
			grid={grid}
			probes={probes}
			chartData={chartData}
			empty={empty}
			valueIndex={4}
			title={t`Loss`}
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
