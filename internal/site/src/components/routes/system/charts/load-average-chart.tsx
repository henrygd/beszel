import { t } from "@lingui/core/macro"
import type { ChartData } from "@/types"
import { ChartCard } from "../chart-card"
import LineChartDefault from "@/components/charts/line-chart"
import { decimalString, toFixedFloat } from "@/lib/utils"

export function LoadAverageChart({
	chartData,
	grid,
	dataEmpty,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	const { major, minor } = chartData.agentVersion
	if (major === 0 && minor <= 12) {
		return null
	}
	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Load Average`}
			description={t`System load averages over time`}
			legend={true}
		>
			<LineChartDefault
				chartData={chartData}
				contentFormatter={(item) => decimalString(item.value)}
				tickFormatter={(value) => {
					return String(toFixedFloat(value, 2))
				}}
				legend={true}
				dataPoints={[
					{
						label: t({ message: `1 min`, comment: "Load average" }),
						color: "hsl(271, 81%, 60%)", // Purple
						dataKey: ({ stats }) => stats?.la?.[0],
					},
					{
						label: t({ message: `5 min`, comment: "Load average" }),
						color: "hsl(217, 91%, 60%)", // Blue
						dataKey: ({ stats }) => stats?.la?.[1],
					},
					{
						label: t({ message: `15 min`, comment: "Load average" }),
						color: "hsl(25, 95%, 53%)", // Orange
						dataKey: ({ stats }) => stats?.la?.[2],
					},
				]}
			></LineChartDefault>
		</ChartCard>
	)
}
