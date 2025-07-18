import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, decimalString, chartMargin, formatBytes, toFixedFloat } from "@/lib/utils"
import { ChartData } from "@/types"
import { memo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Unit } from "@/lib/enums"

export default memo(function DiskChart({
	dataKey,
	diskSize,
	chartData,
}: {
	dataKey: string
	diskSize: number
	chartData: ChartData
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t } = useLingui()

	// round to nearest GB
	if (diskSize >= 100) {
		diskSize = Math.round(diskSize)
	}

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
						width={yAxisWidth}
						domain={[0, diskSize]}
						tickCount={9}
						minTickGap={6}
						tickLine={false}
						axisLine={false}
						tickFormatter={(val) => {
							const { value, unit } = formatBytes(val * 1024, false, Unit.Bytes, true)
							return updateYAxisWidth(toFixedFloat(value, value >= 10 ? 0 : 1) + " " + unit)
						}}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={({ value }) => {
									const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
									return decimalString(convertedValue) + " " + unit
								}}
							/>
						}
					/>
					<Area
						dataKey={dataKey}
						name={t`Disk Usage`}
						type="monotoneX"
						fill="hsl(var(--chart-4))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-4))"
						// animationDuration={1200}
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
