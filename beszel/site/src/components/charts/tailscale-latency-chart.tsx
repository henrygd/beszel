import { CartesianGrid, Line, LineChart, YAxis } from "recharts"
import { memo, useMemo } from "react"
import { useStore } from "@nanostores/react"

import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
	xAxis,
} from "@/components/ui/chart"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	chartMargin,
} from "@/lib/utils"

interface LatencyDataPoint {
	created: number
	latency: Record<string, { latencyMs: number; preferred?: boolean }>
}

interface LatencyChartProps {
	data: LatencyDataPoint[]
	chartData: any // ChartData type from the main app
}

const LatencyChart = memo(({ data, chartData }: LatencyChartProps) => {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	if (data.length === 0) {
		return null
	}

	/** Format latency data for chart and assign colors */
	const newChartData = useMemo(() => {
		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}
		const latencySums = {} as Record<string, number>
		const latencyCounts = {} as Record<string, number>
		
		for (let dataPoint of data) {
			let newData = { created: dataPoint.created } as Record<string, number | string>
			let keys = Object.keys(dataPoint.latency ?? {})
			for (let i = 0; i < keys.length; i++) {
				let key = keys[i]
				newData[key] = dataPoint.latency[key].latencyMs / 1000 // Convert to seconds
				latencySums[key] = (latencySums[key] ?? 0) + newData[key]
				latencyCounts[key] = (latencyCounts[key] ?? 0) + 1
			}
			newChartData.data.push(newData)
		}
		
		// Calculate average latency and sort by fastest (lowest average)
		const averageLatencies = {} as Record<string, number>
		for (let key of Object.keys(latencySums)) {
			averageLatencies[key] = latencySums[key] / latencyCounts[key]
		}
		
		// Get top 10 fastest DERP servers
		const topKeys = Object.keys(averageLatencies)
			.sort((a, b) => averageLatencies[a] - averageLatencies[b])
			.slice(0, 10)
		
		// Only include top 10 in chart data
		for (let dataPoint of newChartData.data) {
			for (let key of Object.keys(dataPoint)) {
				if (key !== 'created' && !topKeys.includes(key)) {
					delete dataPoint[key]
				}
			}
		}
		
		// Assign colors to top 10
		for (let i = 0; i < topKeys.length; i++) {
			const key = topKeys[i]
			newChartData.colors[key] = `hsl(${((i * 360) / topKeys.length) % 360}, 60%, 55%)`
		}
		return newChartData
	}, [data])

	const colors = Object.keys(newChartData.colors)

	const formatLatency = (value: number) => {
		if (value < 1) {
			return `${(value * 1000).toFixed(0)}ms`
		}
		return `${value.toFixed(2)}s`
	}

	return (
		<div>
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<LineChart accessibilityLayer data={newChartData.data} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData?.orientation || "left"}
						className="tracking-tighter"
						domain={[0, "auto"]}
						width={yAxisWidth}
						tickFormatter={(val) => {
							return updateYAxisWidth(formatLatency(val))
						}}
						tickLine={false}
						axisLine={false}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// @ts-ignore
						itemSorter={(a, b) => a.value - b.value}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => formatLatency(item.value)}
							/>
						}
					/>
					{colors.map((key) => {
						return (
							<Line
								key={key}
								dataKey={key}
								name={key}
								type="monotoneX"
								dot={false}
								strokeWidth={1.5}
								stroke={newChartData.colors[key]}
								activeDot={{ r: 6, stroke: newChartData.colors[key], strokeWidth: 2, fill: "#ffffff" }}
								isAnimationActive={false}
							/>
						)
					})}
					{colors.length < 30 && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		</div>
	)
})

LatencyChart.displayName = "LatencyChart"

export default LatencyChart 