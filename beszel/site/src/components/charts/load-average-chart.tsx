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
import { ChartData } from "@/types"
import { memo } from "react"
import { t } from "@lingui/core/macro"

export default memo(function LoadAverageChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	if (chartData.systemStats.length === 0) {
		return null
	}

	const keys = {
		l1: {
			color: "hsl(271, 81%, 60%)", // Purple
			label: t`1 min`,
		},
		l5: {
			color: "hsl(217, 91%, 60%)", // Blue
			label: t`5 min`,
		},
		l15: {
			color: "hsl(25, 95%, 53%)", // Orange
			label: t`15 min`,
		},
	}

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
					{Object.entries(keys).map(([key, value]: [string, { color: string; label: string }]) => {
						return (
							<Line
								key={key}
								dataKey={`stats.${key}`}
								name={value.label}
								type="monotoneX"
								dot={false}
								strokeWidth={1.5}
								stroke={value.color}
								isAnimationActive={false}
							/>
						)
					})}
					<ChartLegend content={<ChartLegendContent />} />
				</LineChart>
			</ChartContainer>
		</div>
	)
})
