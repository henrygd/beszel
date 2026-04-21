import LineChartDefault, { DataPoint } from "@/components/charts/line-chart"
import { pinnedAxisDomain } from "@/components/ui/chart"
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

	const dataPoints: DataPoint<NetworkProbeStatsRecord>[] = useMemo(() => {
		const count = probes.length
		return probes
			.sort((a, b) => a.name.localeCompare(b.name))
			.map((p, i) => {
				const key = probeKey(p)
				const filterTerms = filter
					? filter
							.toLowerCase()
							.split(" ")
							.filter((term) => term.length > 0)
					: []
				const filtered = filterTerms.length > 0 && !filterTerms.some((term) => key.toLowerCase().includes(term))
				const strokeOpacity = filtered ? 0.1 : 1
				return {
					label: p.name || p.target,
					dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[key]?.[0] ?? null,
					color: count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`,
					strokeOpacity,
					activeDot: !filtered,
				}
			})
	}, [probes, filter])

	return (
		<ChartCard
			legend
			cornerEl={<FilterBar store={$filter} />}
			empty={empty}
			title={t`Latency`}
			description={t`Average round-trip time (ms)`}
			grid={grid}
		>
			<LineChartDefault
				chartData={chartData}
				customData={probeStats}
				dataPoints={dataPoints}
				domain={pinnedAxisDomain()}
				connectNulls
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => `${decimalString(value, 2)} ms`}
				legend
				filter={filter}
			/>
		</ChartCard>
	)
}
