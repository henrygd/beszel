import { CartesianGrid, Line, LineChart, YAxis } from "recharts"

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
	toFixedWithoutTrailingZeros,
	decimalString,
	chartMargin,
} from "@/lib/utils"
import { ChartData } from "@/types"
import { memo, useMemo } from "react"
import { t } from "@lingui/core/macro"

export default memo(function LoadAverageChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	if (chartData.systemStats.length === 0) {
		return null
	}

	/** Format load average data for chart */
	const newChartData = useMemo(() => {
		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}
		
		// Define colors for the three load average lines
		const colors = {
			"1m": "hsl(25, 95%, 53%)",   // Orange for 1-minute
			"5m": "hsl(217, 91%, 60%)",  // Blue for 5-minute  
			"15m": "hsl(271, 81%, 56%)", // Purple for 15-minute
		}

		for (let data of chartData.systemStats) {
			let newData = { created: data.created } as Record<string, number | string>
			
			// Add load average values if they exist and stats is not null
			if (data.stats && data.stats.l1 !== undefined) {
				newData["1m"] = data.stats.l1
			}
			if (data.stats && data.stats.l5 !== undefined) {
				newData["5m"] = data.stats.l5
			}
			if (data.stats && data.stats.l15 !== undefined) {
				newData["15m"] = data.stats.l15
			}
			
			newChartData.data.push(newData)
		}
		
		newChartData.colors = colors
		return newChartData
	}, [chartData])

	const loadKeys = ["1m", "5m", "15m"].filter(key => 
		newChartData.data.some(data => data[key] !== undefined)
	)

	// console.log('rendered at', new Date())

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
						orientation={chartData.orientation}
						className="tracking-tighter"
						domain={[0, "auto"]}
						width={yAxisWidth}
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value, 2)
							return updateYAxisWidth(val)
						}}
						tickLine={false}
						axisLine={false}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// @ts-ignore
						itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => decimalString(item.value)}
							/>
						}
					/>
					{loadKeys.map((key) => (
						<Line
							key={key}
							dataKey={key}
							name={key === "1m" ? t`1 min` : key === "5m" ? t`5 min` : t`15 min`}
							type="monotoneX"
							dot={false}
							strokeWidth={1.5}
							stroke={newChartData.colors[key]}
							isAnimationActive={false}
						/>
					))}
					<ChartLegend content={<ChartLegendContent />} />
				</LineChart>
			</ChartContainer>
		</div>
	)
}) 