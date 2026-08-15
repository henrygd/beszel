import { t } from "@lingui/core/macro"
import LineChartDefault from "@/components/charts/line-chart"
import { $fail2banFilter } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { useMemo, useRef } from "react"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, FilterBar } from "../chart-card"

export function Fail2banChart({
	chartData,
	grid,
	dataEmpty,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	const showChart = chartData.systemStats.at(-1)?.stats.f2b

	const filter = useStore($fail2banFilter)
	const statsRef = useRef(chartData.systemStats)
	statsRef.current = chartData.systemStats

	// Derive jail names key from the latest data point that has f2b data
	let jailNamesKey = ""
	for (let i = chartData.systemStats.length - 1; i >= 0; i--) {
		const f2b = chartData.systemStats[i].stats?.f2b
		if (f2b) {
			jailNamesKey = Object.keys(f2b).sort().join("\0")
			break
		}
	}

	const { colorMap, dataKeys, sortedKeys } = useMemo(() => {
		const stats = statsRef.current
		const jailSums = {} as Record<string, number>
		for (const data of stats) {
			const f2b = data.stats?.f2b
			if (!f2b) continue
			for (const key of Object.keys(f2b)) {
				jailSums[key] = (jailSums[key] ?? 0) + f2b[key]
			}
		}
		const sorted = Object.keys(jailSums).sort((a, b) => jailSums[b] - jailSums[a])
		const colorMap = {} as Record<string, string>
		const dataKeys = {} as Record<string, (d: SystemStatsRecord) => number | undefined>
		for (let i = 0; i < sorted.length; i++) {
			const key = sorted[i]
			colorMap[key] = `hsl(${((i * 360) / sorted.length) % 360}, 60%, 55%)`
			dataKeys[key] = (d: SystemStatsRecord) => d.stats?.f2b?.[key]
		}
		return { colorMap, dataKeys, sortedKeys: sorted }
	}, [jailNamesKey])

	const dataPoints = useMemo(() => {
		return sortedKeys.map((key) => {
			const filterTerms = filter
				? filter
						.toLowerCase()
						.split(" ")
						.filter((term) => term.length > 0)
				: []
			const filtered = filterTerms.length > 0 && !filterTerms.some((term) => key.toLowerCase().includes(term))
			return {
				label: key,
				dataKey: dataKeys[key],
				color: colorMap[key],
				strokeOpacity: filtered ? 0.1 : 1,
				activeDot: !filtered,
			}
		})
	}, [sortedKeys, filter, dataKeys, colorMap])

	if (!showChart) {
		return null
	}

	const legend = dataPoints.length < 12

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Fail2ban`}
			description={t`Banned IPs per jail`}
			cornerEl={<FilterBar store={$fail2banFilter} />}
			legend={legend}
		>
			<LineChartDefault
				chartData={chartData}
				itemSorter={(a, b) => b.value - a.value}
				domain={[0, "auto"]}
				legend={legend}
				tickFormatter={(val) => String(Math.round(val))}
				contentFormatter={({ value }) => String(Math.round(value))}
				dataPoints={dataPoints}
				filter={filter}
			/>
		</ChartCard>
	)
}
