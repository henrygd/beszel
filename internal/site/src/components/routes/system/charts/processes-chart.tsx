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
				contentFormatter={(item) => String(Math.round(item.value))}
				tickFormatter={(value) => String(Math.round(value))}
				legend={true}
				dataPoints={[
					{
						label: t`Running`,
						color: "hsl(142, 72%, 36%)",
						dataKey: ({ stats }) => stats?.ps?.[1],
					},
					{
						label: t`Sleeping`,
						color: "hsl(271, 81%, 60%)",
						dataKey: ({ stats }) => stats?.ps?.[2],
					},
					{
						label: t`Idle`,
						color: "hsl(25, 95%, 53%)",
						dataKey: ({ stats }) => stats?.ps?.[3],
					},
					{
						label: t`Zombie`,
						color: "hsl(340, 82%, 52%)",
						dataKey: ({ stats }) => stats?.ps?.[5],
					},
				]}
			></LineChartDefault>
		</ChartCard>
	)
}
