import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, chartMargin } from "@/lib/utils"
import { ChartData, SystemStatsRecord } from "@/types"
import { useMemo } from "react"

export type DataPoint = {
	label: string
	dataKey: (data: SystemStatsRecord) => number | undefined
	color: string
	opacity: number
}

export default function AreaChartDefault({
	chartData,
	max,
	maxToggled,
	tickFormatter,
	contentFormatter,
	dataPoints,
}: // logRender = false,
{
	chartData: ChartData
	max?: number
	maxToggled?: boolean
	tickFormatter: (value: number, index: number) => string
	contentFormatter: ({ value, payload }: { value: number; payload: SystemStatsRecord }) => string
	dataPoints?: DataPoint[]
	// logRender?: boolean
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	return useMemo(() => {
		if (chartData.systemStats.length === 0) {
			return null
		}
		// if (logRender) {
		// 	console.log("Rendered at", new Date())
		// }
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
							domain={[0, max ?? "auto"]}
							tickFormatter={(value, index) => updateYAxisWidth(tickFormatter(value, index))}
							tickLine={false}
							axisLine={false}
						/>
						{xAxis(chartData)}
						<ChartTooltip
							animationEasing="ease-out"
							animationDuration={150}
							content={
								<ChartTooltipContent
									labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
									contentFormatter={contentFormatter}
								/>
							}
						/>
						{dataPoints?.map((dataPoint, i) => {
							const color = `hsl(var(--chart-${dataPoint.color}))`
							return (
								<Area
									key={i}
									dataKey={dataPoint.dataKey}
									name={dataPoint.label}
									type="monotoneX"
									fill={color}
									fillOpacity={dataPoint.opacity}
									stroke={color}
									isAnimationActive={false}
								/>
							)
						})}
						{/* <ChartLegend content={<ChartLegendContent />} /> */}
					</AreaChart>
				</ChartContainer>
			</div>
		)
	}, [chartData.systemStats.at(-1), yAxisWidth, maxToggled])
}
