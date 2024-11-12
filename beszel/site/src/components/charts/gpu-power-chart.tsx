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

export default memo(function GpuPowerChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	if (chartData.systemStats.length === 0) {
		return null
	}

	/** Format temperature data for chart and assign colors */
	const newChartData = useMemo(() => {
		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}
		const powerSums = {} as Record<string, number>
		for (let data of chartData.systemStats) {
			let newData = { created: data.created } as Record<string, number | string>

			for (let gpu of Object.values(data.stats?.g ?? {})) {
				if (gpu.p) {
					const name = gpu.n
					newData[name] = gpu.p
					powerSums[name] = (powerSums[name] ?? 0) + newData[name]
				}
			}
			newChartData.data.push(newData)
		}
		const keys = Object.keys(powerSums).sort((a, b) => powerSums[b] - powerSums[a])
		for (let key of keys) {
			newChartData.colors[key] = `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
		}
		return newChartData
	}, [chartData])

	const colors = Object.keys(newChartData.colors)

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
							return updateYAxisWidth(val + "W")
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
								contentFormatter={(item) => decimalString(item.value) + "W"}
								// indicator="line"
							/>
						}
					/>
					{colors.map((key) => (
						<Line
							key={key}
							dataKey={key}
							name={key}
							type="monotoneX"
							dot={false}
							strokeWidth={1.5}
							stroke={newChartData.colors[key]}
							isAnimationActive={false}
						/>
					))}
					{colors.length > 1 && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		</div>
	)
})
