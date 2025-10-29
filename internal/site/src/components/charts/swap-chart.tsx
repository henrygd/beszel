import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { memo } from "react"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { $userSettings } from "@/lib/stores"
import { chartMargin, cn, decimalString, formatBytes, formatShortDate, toFixedFloat } from "@/lib/utils"
import type { ChartData } from "@/types"
import { useYAxisWidth } from "./hooks"

export default memo(function SwapChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const userSettings = useStore($userSettings)

	if (chartData.systemStats.length === 0) {
		return null
	}

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
						domain={[0, () => toFixedFloat(chartData.systemStats.at(-1)?.stats.s ?? 0.04, 2)]}
						width={yAxisWidth}
						tickLine={false}
						axisLine={false}
						tickFormatter={(value) => {
							const { value: convertedValue, unit } = formatBytes(value * 1024, false, userSettings.unitDisk, true)
							return updateYAxisWidth(toFixedFloat(convertedValue, value >= 10 ? 0 : 1) + " " + unit)
						}}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.timestamp)}
								contentFormatter={({ value }) => {
									// mem values are supplied as GB
									const { value: convertedValue, unit } = formatBytes(value * 1024, false, userSettings.unitDisk, true)
									return decimalString(convertedValue, convertedValue >= 100 ? 1 : 2) + " " + unit
								}}
								// indicator="line"
							/>
						}
					/>
					<Area
						dataKey="stats.su"
						name={t`Used`}
						type="monotoneX"
						fill="var(--chart-2)"
						fillOpacity={0.4}
						stroke="var(--chart-2)"
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
