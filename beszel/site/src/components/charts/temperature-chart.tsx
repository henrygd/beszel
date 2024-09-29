import { CartesianGrid, Line, LineChart, XAxis, YAxis } from 'recharts'

import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import {
	useYAxisWidth,
	chartTimeData,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	twoDecimalString,
} from '@/lib/utils'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'
import { useMemo } from 'react'

export default function TemperatureChart({
	ticks,
	systemData,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
}) {
	const chartTime = useStore($chartTime)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	/** Format temperature data for chart and assign colors */
	const newChartData = useMemo(() => {
		const chartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}
		const tempSums = {} as Record<string, number>
		for (let data of systemData) {
			let newData = { created: data.created } as Record<string, number | string>
			let keys = Object.keys(data.stats?.t ?? {})
			for (let i = 0; i < keys.length; i++) {
				let key = keys[i]
				newData[key] = data.stats.t![key]
				tempSums[key] = (tempSums[key] ?? 0) + newData[key]
			}
			chartData.data.push(newData)
		}
		const keys = Object.keys(tempSums).sort((a, b) => tempSums[b] - tempSums[a])
		for (let key of keys) {
			chartData.colors[key] = `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
		}
		return chartData
	}, [systemData])

	const colors = Object.keys(newChartData.colors)

	return (
		<div>
			{/* {!yAxisSet && <Spinner />} */}
			<ChartContainer
				config={{}}
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisWidth,
				})}
			>
				<LineChart
					accessibilityLayer
					data={newChartData.data}
					margin={{
						left: 0,
						right: 0,
						top: 10,
						bottom: 0,
					}}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						domain={[0, 'auto']}
						width={yAxisWidth}
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value, 2)
							return updateYAxisWidth(val + ' °C')
						}}
						tickLine={false}
						axisLine={false}
					/>
					<XAxis
						dataKey="created"
						domain={[ticks[0], ticks.at(-1)!]}
						ticks={ticks}
						type="number"
						scale={'time'}
						minTickGap={35}
						tickMargin={8}
						axisLine={false}
						tickFormatter={chartTimeData[chartTime].format}
					/>
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// @ts-ignore
						itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => twoDecimalString(item.value) + ' °C'}
								indicator="line"
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
					{colors.length < 12 && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		</div>
	)
}
