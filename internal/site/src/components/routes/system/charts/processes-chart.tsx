import { t } from "@lingui/core/macro"
import type { ChartData } from "@/types"
import { ChartCard } from "../chart-card"
import LineChartDefault from "@/components/charts/line-chart"

export function ProcessesChart({
	chartData,
	grid,
	dataEmpty,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	// Process metrics were added after the first versions of the agent. Avoid
	// rendering an empty card when historical data comes from an older agent.
	if (!chartData.systemStats.some(({ stats }) => stats?.ps)) {
		return null
	}

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Processes`}
			description={t`Process counts by state`}
			legend={true}
		>
			<LineChartDefault
				chartData={chartData}
				itemSorter={(a, b) => b.value - a.value}
				contentFormatter={(item) => String(Math.round(item.value))}
				tickFormatter={(value) => String(Math.round(value))}
				legend={true}
				dataPoints={[
					{
						label: t`Running`,
						color: "hsl(142, 72%, 36%)",
						dataKey: ({ stats }) => stats?.ps?.[0],
					},
					{
						label: t`Sleeping`,
						color: "hsl(271, 81%, 60%)",
						dataKey: ({ stats }) => stats?.ps?.[1],
					},
					{
						label: t`Idle`,
						color: "hsl(25, 95%, 53%)",
						dataKey: ({ stats }) => stats?.ps?.[2],
					},
					{
						label: t`Stopped`,
						color: "hsl(45, 93%, 47%)",
						dataKey: ({ stats }) => stats?.ps?.[3],
					},
					{
						label: t`Zombie`,
						color: "hsl(340, 82%, 52%)",
						dataKey: ({ stats }) => stats?.ps?.[4],
					},
				]}
			></LineChartDefault>
		</ChartCard>
	)
}
