import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartLegend, ChartLegendContent, ChartTooltipContent, xAxis } from "@/components/ui/chart"
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
import { useLingui } from "@lingui/react/macro"

type DiskChartProps = { dataKey: string, diskSize: number, chartData: ChartData, showLegend?: boolean }
export default memo(function DiskChart({ dataKey, diskSize, chartData, showLegend = true }: DiskChartProps) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t } = useLingui()

	// round to nearest GB
	if (diskSize >= 100) {
		diskSize = Math.round(diskSize)
	}

	// Compute free disk for each data point
	const diskDataWithFree = chartData.systemStats.map((point) => {
		const used = point.stats && typeof point.stats.du === 'number' ? point.stats.du : 0
		const total = point.stats && typeof point.stats.d === 'number' ? point.stats.d : diskSize
		const free = total - used
		return {
			...point,
			stats: {
				...point.stats,
				df: free > 0 ? free : 0, // df = disk free
			},
		}
	})

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
				<AreaChart accessibilityLayer data={diskDataWithFree} margin={chartMargin}>
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
						name={t`Used`}
						type="monotoneX"
						fill="hsl(var(--chart-4))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-4))"
						isAnimationActive={false}
						stackId="disk"
					/>
					{showLegend && <ChartLegend content={<ChartLegendContent />} wrapperStyle={{ marginTop: 16 }} />}
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
