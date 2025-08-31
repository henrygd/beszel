import { CartesianGrid, Line, LineChart, YAxis } from "recharts"

import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
	xAxis,
} from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, toFixedFloat, decimalString, chartMargin } from "@/lib/utils"
import { ChartData, SystemStats } from "@/types"
import { memo } from "react"
import { t } from "@lingui/core/macro"

export default memo(function LoadAverageChart({ 
	chartData, 
	showLegend = true 
}: { 
	chartData: ChartData
	showLegend?: boolean 
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	const keys: { legacy: keyof SystemStats; color: string; label: string }[] = [
		{
			legacy: "l1",
			color: "hsl(271, 81%, 60%)", // Purple
			label: t({ message: `1 min`, comment: "Load average" }),
		},
		{
			legacy: "l5",
			color: "hsl(217, 91%, 60%)", // Blue
			label: t({ message: `5 min`, comment: "Load average" }),
		},
		{
			legacy: "l15",
			color: "hsl(25, 95%, 53%)", // Orange
			label: t({ message: `15 min`, comment: "Load average" }),
		},
	]

	return (
		<div>
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<LineChart accessibilityLayer data={chartData.systemStats} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						domain={[0, "auto"]}
						width={yAxisWidth}
						tickFormatter={(value) => {
							return updateYAxisWidth(String(toFixedFloat(value, 2)))
						}}
						tickLine={false}
						axisLine={false}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// @ts-ignore
						// itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => decimalString(item.value)}
							/>
						}
					/>
					{keys.map(({ legacy, color, label }, i) => {
						const dataKey = (value: { stats: SystemStats }) => {
							if (chartData.agentVersion.patch < 1) {
								return value.stats?.[legacy]
							}
							return value.stats?.la?.[i] ?? value.stats?.[legacy]
						}
						return (
							<Line
								key={i}
								dataKey={dataKey}
								name={label}
								type="monotoneX"
								dot={false}
								strokeWidth={1.5}
								stroke={color}
								isAnimationActive={false}
							/>
						)
					})}
					{showLegend && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		</div>
	)
})
