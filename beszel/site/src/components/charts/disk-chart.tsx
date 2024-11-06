import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"

import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	decimalString,
	toFixedFloat,
	chartMargin,
	getSizeAndUnit,
} from "@/lib/utils"
import { ChartData } from "@/types"
import { memo } from "react"
import { t } from "@lingui/macro"
import { useLingui } from "@lingui/react"

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
	const { _ } = useLingui()

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
						tickFormatter={(value) => {
							const { v, u } = getSizeAndUnit(value)
							return updateYAxisWidth(toFixedFloat(v, 2) + u)
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
									const { v, u } = getSizeAndUnit(value)
									return decimalString(v) + u
								}}
							/>
						}
					/>
					<Area
						dataKey={dataKey}
						name={_(t`Disk Usage`)}
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
