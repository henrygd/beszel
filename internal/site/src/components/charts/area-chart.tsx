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
import { AxisDomain } from "recharts/types/util/types"

export type DataPoint = {
	label: string
	dataKey: (data: SystemStatsRecord) => number | undefined
	color: number | string
	opacity: number
	stackId?: string | number
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
	reverseStackOrder = false,
	hideYAxis = false,
}: // logRender = false,
	{
		chartData: ChartData
		max?: number
		maxToggled?: boolean
		tickFormatter: (value: number, index: number) => string
		contentFormatter: ({ value, payload }: { value: number; payload: SystemStatsRecord }) => string
		dataPoints?: DataPoint[]
		domain?: AxisDomain
		legend?: boolean
		showTotal?: boolean
		itemSorter?: (a: any, b: any) => number
		reverseStackOrder?: boolean
		hideYAxis?: boolean
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
						"opacity-100": yAxisWidth || hideYAxis,
						"ps-4": hideYAxis,
					})}
				>
					<AreaChart
						reverseStackOrder={reverseStackOrder}
						accessibilityLayer
						data={chartData.systemStats}
						margin={hideYAxis ? { ...chartMargin, left: 5 } : chartMargin}
					>
						<CartesianGrid vertical={false} />
						{!hideYAxis && (
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
						)}
						{xAxis(chartData)}
						<ChartTooltip
							animationEasing="ease-out"
							animationDuration={150}
							// @ts-expect-error
							itemSorter={itemSorter}
							content={
								<ChartTooltipContent
									labelFormatter={(_, data) => formatShortDate(data[0].payload.timestamp)}
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
									stackId={dataPoint.stackId}
								/>
							)
						})}
						{legend && <ChartLegend content={<ChartLegendContent reverse={reverseStackOrder} />} />}
					</AreaChart>
				</ChartContainer>
			</div>
		)
	}, [chartData.systemStats.at(-1), yAxisWidth, maxToggled, showTotal])
}
