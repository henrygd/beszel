import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
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
	chartMargin,
} from "@/lib/utils"
import { ChartData } from "@/types"
import { memo, useMemo } from "react"

export default memo(function ProcessStateChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	if (chartData.systemStats.length === 0) {
		return null
	}

	// Format process state data for chart and assign colors
	const newChartData = useMemo(() => {
		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}
		const stateSums = {} as Record<string, number>
		for (let data of chartData.systemStats) {
			let newData = { created: data.created } as Record<string, number | string>
			let states = Object.keys(data.stats?.ps_states ?? {})
			for (let i = 0; i < states.length; i++) {
				let state = states[i]
				newData[state] = data.stats.ps_states![state]
				stateSums[state] = (stateSums[state] ?? 0) + (newData[state] as number)
			}
			newChartData.data.push(newData)
		}
		const keys = Object.keys(stateSums).sort((a, b) => stateSums[b] - stateSums[a])
		for (let key of keys) {
			newChartData.colors[key] = `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
		}
		return newChartData
	}, [chartData])

	const colors = Object.keys(newChartData.colors)

	return (
		<div>
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart accessibilityLayer data={newChartData.data} margin={chartMargin} reverseStackOrder={true}>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						domain={[0, "auto"]}
						width={yAxisWidth}
						tickFormatter={(value: number) => {
							const val = toFixedWithoutTrailingZeros(value, 0)
							return updateYAxisWidth(val + "")
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
								contentFormatter={(item) => `${Math.round(item.value)}`}
							/>
						}
					/>
					{colors.map((key) => (
						<Area
							key={key}
							dataKey={key}
							name={key}
							type="monotoneX"
							fill={newChartData.colors[key]}
							fillOpacity={0.4}
							stroke={newChartData.colors[key]}
							strokeOpacity={1}
							stackId="a"
							isAnimationActive={false}
						/>
					))}
					{colors.length > 1 && <ChartLegend content={<ChartLegendContent />} />}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}) 