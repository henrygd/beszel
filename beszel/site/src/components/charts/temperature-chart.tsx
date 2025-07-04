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
	convertTemperature,
} from "@/lib/utils"
import { ChartData } from "@/types"
import { memo, useMemo } from "react"
import { $temperatureFilter, $userSettings } from "@/lib/stores"
import { useStore } from "@nanostores/react"

export default memo(function TemperatureChart({ chartData }: { chartData: ChartData }) {
	const filter = useStore($temperatureFilter)
	const userSettings = useStore($userSettings)
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
		const tempSums = {} as Record<string, number>
		const unit = userSettings.temperatureUnit || "celsius"
		
		for (let data of chartData.systemStats) {
			let newData = { created: data.created } as Record<string, number | string>
			let keys = Object.keys(data.stats?.t ?? {})
			for (let i = 0; i < keys.length; i++) {
				let key = keys[i]
				const celsiusTemp = data.stats.t![key]
				const { value } = convertTemperature(celsiusTemp, unit)
				newData[key] = value
				tempSums[key] = (tempSums[key] ?? 0) + value
			}
			newChartData.data.push(newData)
		}
		const keys = Object.keys(tempSums).sort((a, b) => tempSums[b] - tempSums[a])
		for (let key of keys) {
			newChartData.colors[key] = `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
		}
		return newChartData
	}, [chartData, userSettings.temperatureUnit])

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
							const { symbol } = convertTemperature(0, userSettings.temperatureUnit || "celsius")
							return updateYAxisWidth(val + " " + symbol)
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
								contentFormatter={(item) => {
									const { symbol } = convertTemperature(0, userSettings.temperatureUnit || "celsius")
									return decimalString(item.value) + " " + symbol
								}}
								filter={filter}
							/>
						}
					/>
					{colors.map((key) => {
						const filtered = filter && !key.toLowerCase().includes(filter.toLowerCase())
						let strokeOpacity = filtered ? 0.1 : 1
						return (
							<Line
								key={key}
								dataKey={key}
								name={key}
								type="monotoneX"
								dot={false}
								strokeWidth={1.5}
								stroke={newChartData.colors[key]}
								strokeOpacity={strokeOpacity}
								activeDot={{ opacity: filtered ? 0 : 1 }}
								isAnimationActive={false}
							/>
						)
					})}
					{colors.length < 12 && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		</div>
	)
})
