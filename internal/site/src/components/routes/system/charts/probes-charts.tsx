import LineChartDefault from "@/components/charts/line-chart"
import type { DataPoint } from "@/components/charts/line-chart"
import { toFixedFloat, decimalString } from "@/lib/utils"
import { useLingui } from "@lingui/react/macro"
import { ChartCard, FilterBar } from "../chart-card"
import type { ChartData, NetworkProbeRecord, NetworkProbeStatsRecord } from "@/types"
import { useMemo } from "react"
import { atom } from "nanostores"
import { useStore } from "@nanostores/react"
import { probeKey } from "@/lib/use-network-probes"

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
		const sortedProbes = [...probes].sort((a, b) => b.latency - a.latency)
		const count = sortedProbes.length
		const points: DataPoint<NetworkProbeStatsRecord>[] = []
		const visibleKeys: string[] = []
		const filterTerms = filter
			? filter
					.toLowerCase()
					.split(" ")
					.filter((term) => term.length > 0)
			: []
		for (let i = 0; i < count; i++) {
			const p = sortedProbes[i]
			const key = probeKey(p)
			const filtered = filterTerms.length > 0 && !filterTerms.some((term) => key.toLowerCase().includes(term))
			if (filtered) {
				continue
			}
			visibleKeys.push(key)
			points.push({
				order: i,
				label: p.name || p.target,
				dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[key]?.[valueIndex] ?? "-",
				color: count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`,
			})
		}
		return { dataPoints: points, visibleKeys }
	}, [probes, filter, valueIndex])

	const filteredProbeStats = useMemo(() => {
		if (!visibleKeys.length) return probeStats
		return probeStats.filter((record) => visibleKeys.some((key) => record.stats?.[key] != null))
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

export function LatencyChart({ probeStats, grid, probes, chartData, empty }: ProbeChartProps) {
	const { t } = useLingui()

	return (
		<ProbeChart
			probeStats={probeStats}
			grid={grid}
			probes={probes}
			chartData={chartData}
			empty={empty}
			valueIndex={0}
			title={t`Latency`}
			description={t`Average round-trip time (ms)`}
			tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
			contentFormatter={({ value }) => {
				if (typeof value !== "number") {
					return value
				}
				return `${decimalString(value, 2)} ms`
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
			valueIndex={3}
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
