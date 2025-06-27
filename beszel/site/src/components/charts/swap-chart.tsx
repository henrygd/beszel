import { t } from "@lingui/core/macro";

import { Area, AreaChart, CartesianGrid, Line, YAxis } from "recharts"
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	decimalString,
	chartMargin,
} from "@/lib/utils"
import { ChartData } from "@/types"
import { memo } from "react"

export default memo(function SwapChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	if (chartData.systemStats.length === 0) {
		return null
	}

	const lastStats = chartData.systemStats.at(-1)?.stats
	const swapTotal = lastStats?.st ?? lastStats?.s ?? 0.04

	// Debug: log the swap data
	console.log('Swap chart data:', {
		swapTotal: lastStats?.st,
		swapFree: lastStats?.sf,
		swapUsed: lastStats?.su,
		legacySwap: lastStats?.s
	})

	return (
		<div>
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart accessibilityLayer data={chartData.systemStats} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						domain={[0, () => toFixedWithoutTrailingZeros(swapTotal, 2)]}
						width={yAxisWidth}
						tickLine={false}
						axisLine={false}
						tickFormatter={(value) => updateYAxisWidth(value + " GB")}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => decimalString(item.value) + " GB"}
							/>
						}
					/>
					<Area
						dataKey="stats.su"
						name={t`Used`}
						type="monotoneX"
						fill="hsl(var(--chart-2))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-2))"
						isAnimationActive={false}
						stackId="swap"
					/>
					<Area
						dataKey="stats.sf"
						name={t`Free`}
						type="monotoneX"
						fill="hsl(var(--chart-1))"
						fillOpacity={0.3}
						stroke="hsl(var(--chart-1))"
						isAnimationActive={false}
						stackId="swap"
					/>
					<ChartLegend content={<ChartLegendContent />} />
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
