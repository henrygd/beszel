import { useStore } from "@nanostores/react"
import { memo, useMemo } from "react"
import { CartesianGrid, Line, LineChart, YAxis } from "recharts"
import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
	xAxis,
} from "@/components/ui/chart"
import { $temperatureFilter, $userSettings } from "@/lib/stores"
import { chartMargin, cn, decimalString, formatShortDate, formatTemperature, toFixedFloat } from "@/lib/utils"
import type { ChartData } from "@/types"
import { useYAxisWidth } from "./hooks"

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
		for (const data of chartData.systemStats) {
			const newData = { created: data.created } as Record<string, number | string>
			const keys = Object.keys(data.stats?.t ?? {})
			for (let i = 0; i < keys.length; i++) {
				const key = keys[i]
				newData[key] = data.stats.t![key]
				tempSums[key] = (tempSums[key] ?? 0) + newData[key]
			}
			newChartData.data.push(newData)
		}
		const keys = Object.keys(tempSums).sort((a, b) => tempSums[b] - tempSums[a])
		for (const key of keys) {
			newChartData.colors[key] = `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
		}
		return newChartData
	}, [chartData])

	const colors = Object.keys(newChartData.colors)

	/** Calculate temperature domain to focus on operating range */
	const tempDomain = useMemo((): [number, number] | [number, "auto"] => {
		if (newChartData.data.length === 0) {
			return [0, "auto"] as [number, "auto"]
		}
		let minTemp = Infinity
		let maxTemp = -Infinity
		for (const data of newChartData.data) {
			for (const key of colors) {
				const temp = data[key] as number
				if (typeof temp === "number" && !isNaN(temp)) {
					minTemp = Math.min(minTemp, temp)
					maxTemp = Math.max(maxTemp, temp)
				}
			}
		}
		if (minTemp === Infinity || maxTemp === -Infinity) {
			return [0, "auto"] as [number, "auto"]
		}
		// Add 10% padding and round down/up to nice numbers
		const range = maxTemp - minTemp
		const padding = Math.max(range * 0.1, 5) // At least 5 degrees padding
		return [Math.floor(minTemp - padding), Math.ceil(maxTemp + padding)]
	}, [newChartData.data, colors])

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
						domain={tempDomain}
						width={yAxisWidth}
						tickFormatter={(val) => {
							const { value, unit } = formatTemperature(val, userSettings.unitTemp)
							return updateYAxisWidth(toFixedFloat(value, 2) + " " + unit)
						}}
						tickLine={false}
						axisLine={false}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// @ts-expect-error
						itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => {
									const { value, unit } = formatTemperature(item.value, userSettings.unitTemp)
									return decimalString(value) + " " + unit
								}}
								filter={filter}
							/>
						}
					/>
					{colors.map((key) => {
						const filterTerms = filter ? filter.toLowerCase().split(" ").filter(term => term.length > 0) : []
						const filtered = filterTerms.length > 0 && !filterTerms.some(term => key.toLowerCase().includes(term))
						const strokeOpacity = filtered ? 0.1 : 1
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
