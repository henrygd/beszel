import { useMemo } from "react"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
	xAxis,
} from "@/components/ui/chart"
import { chartMargin, cn, formatShortDate } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { useYAxisWidth } from "./hooks"

export type DataPoint = {
	label: string
	dataKey: (data: SystemStatsRecord) => number | undefined
	color: number | string
	opacity: number
}

export default function AreaChartDefault({
	chartData,
	max,
	maxToggled,
	tickFormatter,
	contentFormatter,
	dataPoints,
	domain,
	legend,
	itemSorter,
	showTotal = false,
}: // logRender = false,
{
	chartData: ChartData
	max?: number
	maxToggled?: boolean
	tickFormatter: (value: number, index: number) => string
	contentFormatter: ({ value, payload }: { value: number; payload: SystemStatsRecord }) => string
	dataPoints?: DataPoint[]
	domain?: [number, number]
	legend?: boolean
	itemSorter?: (a: any, b: any) => number
	showTotal?: boolean
	// logRender?: boolean
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	// biome-ignore lint/correctness/useExhaustiveDependencies: ignore
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
							domain={domain ?? [0, max ?? "auto"]}
							tickFormatter={(value, index) => updateYAxisWidth(tickFormatter(value, index))}
							tickLine={false}
							axisLine={false}
						/>
						{xAxis(chartData)}
						<ChartTooltip
							animationEasing="ease-out"
							animationDuration={150}
							// @ts-expect-error
							itemSorter={itemSorter}
							content={
								<ChartTooltipContent
									labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
									contentFormatter={contentFormatter}
									showTotal={showTotal}
								/>
							}
						/>
						{dataPoints?.map((dataPoint) => {
							let { color } = dataPoint
							if (typeof color === "number") {
								color = `var(--chart-${color})`
							}
							return (
								<Area
									key={dataPoint.label}
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
						{legend && <ChartLegend content={<ChartLegendContent />} />}
					</AreaChart>
				</ChartContainer>
			</div>
		)
	}, [chartData.systemStats.at(-1), yAxisWidth, maxToggled, showTotal])
}
