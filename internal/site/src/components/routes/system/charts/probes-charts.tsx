import LineChartDefault from "@/components/charts/line-chart"
import type { DataPoint } from "@/components/charts/line-chart"
import { toFixedFloat, decimalString } from "@/lib/utils"
import { useLingui } from "@lingui/react/macro"
import { ChartCard, FilterBar } from "../chart-card"
import type { ChartData, NetworkProbeRecord, NetworkProbeStatsRecord } from "@/types"
import { useMemo } from "react"
import { atom } from "nanostores"
import { useStore } from "@nanostores/react"

function probeKey(p: NetworkProbeRecord) {
	if (p.protocol === "tcp") return `${p.protocol}:${p.target}:${p.port}`
	return `${p.protocol}:${p.target}`
}

const $filter = atom("")

export function LatencyChart({
	probeStats,
	grid,
	probes,
	chartData,
	empty,
}: {
	probeStats: NetworkProbeStatsRecord[]
	grid?: boolean
	probes: NetworkProbeRecord[]
	chartData: ChartData
	empty: boolean
}) {
	const { t } = useLingui()
	const filter = useStore($filter)

	const { dataPoints, visibleKeys } = useMemo(() => {
		const count = probes.length
		const points: DataPoint<NetworkProbeStatsRecord>[] = []
		const visibleKeys: string[] = []
		probes.sort((a, b) => a.name.localeCompare(b.name))
		const filterTerms = filter
			? filter
					.toLowerCase()
					.split(" ")
					.filter((term) => term.length > 0)
			: []
		for (let i = 0; i < count; i++) {
			const p = probes[i]
			const key = probeKey(p)
			const filtered = filterTerms.length > 0 && !filterTerms.some((term) => key.toLowerCase().includes(term))
			if (filtered) {
				continue
			}
			visibleKeys.push(key)
			points.push({
				label: p.name || p.target,
				dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[key]?.[0] ?? "-",
				color: count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`,
			})
		}
		return { dataPoints: points, visibleKeys }
	}, [probes, filter])

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
			title={t`Latency`}
			description={t`Average round-trip time (ms)`}
			grid={grid}
		>
			<LineChartDefault
				chartData={chartData}
				customData={filteredProbeStats}
				dataPoints={dataPoints}
				domain={["auto", "auto"]}
				connectNulls
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => {
					if (value === "-") {
						return value
					}
					return `${decimalString(value, 2)} ms`
				}}
				legend={legend}
				filter={filter}
			/>
		</ChartCard>
	)
}
