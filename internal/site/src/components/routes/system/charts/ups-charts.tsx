import { t } from "@lingui/core/macro"
import { useMemo } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import type { ChartData, SystemStatsRecord, UpsData } from "@/types"
import { ChartCard } from "../chart-card"

type UpsField = keyof Pick<UpsData, "bat" | "lp" | "iv" | "ov" | "tl">

/** Union of UPS names across all records, sorted for stable colors. */
function useUpsNames(chartData: ChartData) {
	return useMemo(() => {
		const names = new Set<string>()
		for (const record of chartData.systemStats) {
			for (const name in record.stats?.ups ?? {}) {
				names.add(name)
			}
		}
		return [...names].sort()
	}, [chartData.systemStats])
}

/** One series per UPS for a given metric field. */
function upsSeries(upsNames: string[], field: UpsField) {
	return upsNames.map((name, index) => ({
		label: name,
		dataKey: ({ stats }: SystemStatsRecord) => stats?.ups?.[name]?.[field],
		color: `hsl(${(index * 360 + 226) / Math.max(upsNames.length, 1)}, 65%, 52%)`,
		opacity: 0.3,
	}))
}

export function UpsChart({
	chartData,
	grid,
	dataEmpty,
	maxValues,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	maxValues: boolean
}) {
	const upsNames = useUpsNames(chartData)
	if (upsNames.length === 0) {
		return null
	}

	const latest = chartData.systemStats.at(-1)?.stats?.ups ?? {}
	const onBattery = Object.values(latest).some((u) => u?.ob)
	const primaryName = upsNames[0]
	const multi = upsNames.length > 1

	const pct = (val: number) => `${val}%`

	return (
		<>
			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`UPS Battery`}
				description={`${t`Status`}: ${onBattery ? t`On battery` : t`On line`}`}
			>
				<AreaChartDefault
					chartData={chartData}
					maxToggled={maxValues}
					dataPoints={upsSeries(upsNames, "bat")}
					domain={[0, 100]}
					legend={multi}
					tickFormatter={pct}
					contentFormatter={({ value }) => pct(value)}
					itemSorter={(a, b) => b.value - a.value}
				/>
			</ChartCard>

			<ChartCard empty={dataEmpty} grid={grid} title={t`UPS Load`} description={t`UPS load percentage`}>
				<AreaChartDefault
					chartData={chartData}
					maxToggled={maxValues}
					dataPoints={upsSeries(upsNames, "lp")}
					domain={[0, 100]}
					legend={multi}
					tickFormatter={pct}
					contentFormatter={({ value }) => pct(value)}
					itemSorter={(a, b) => b.value - a.value}
				/>
			</ChartCard>

			<ChartCard empty={dataEmpty} grid={grid} title={t`UPS Voltage`} description={t`Input and output voltage`}>
				<AreaChartDefault
					chartData={chartData}
					maxToggled={maxValues}
					dataPoints={[
						{
							label: t`Input`,
							dataKey: ({ stats }: SystemStatsRecord) => stats?.ups?.[primaryName]?.iv,
							color: 2,
							opacity: 0.3,
						},
						{
							label: t`Output`,
							dataKey: ({ stats }: SystemStatsRecord) => stats?.ups?.[primaryName]?.ov,
							color: 5,
							opacity: 0.3,
						},
					]}
					legend={true}
					tickFormatter={(val) => `${val}V`}
					contentFormatter={({ value }) => `${value}V`}
					itemSorter={(a, b) => b.value - a.value}
				/>
			</ChartCard>

			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`UPS Runtime`}
				description={t`Estimated runtime remaining on battery`}
			>
				<AreaChartDefault
					chartData={chartData}
					maxToggled={maxValues}
					dataPoints={upsSeries(upsNames, "tl")}
					legend={multi}
					tickFormatter={(val) => `${val}m`}
					contentFormatter={({ value }) => `${value}m`}
					itemSorter={(a, b) => b.value - a.value}
				/>
			</ChartCard>
		</>
	)
}
