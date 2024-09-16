import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { useMemo } from 'react'
import {
	useYAxisWidth,
	chartTimeData,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	twoDecimalString,
} from '@/lib/utils'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime, $containerFilter } from '@/lib/stores'

export default function ContainerMemChart({
	chartData,
	ticks,
}: {
	chartData: Record<string, number | string>[]
	ticks: number[]
}) {
	const chartTime = useStore($chartTime)
	const filter = useStore($containerFilter)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	const chartConfig = useMemo(() => {
		let config = {} as Record<
			string,
			{
				label: string
				color: string
			}
		>
		const totalUsage = {} as Record<string, number>
		for (let stats of chartData) {
			for (let key in stats) {
				if (key === 'time') {
					continue
				}
				if (!(key in totalUsage)) {
					totalUsage[key] = 0
				}
				// @ts-ignore
				totalUsage[key] += stats[key]
			}
		}
		let keys = Object.keys(totalUsage)
		keys.sort((a, b) => (totalUsage[a] > totalUsage[b] ? -1 : 1))
		const length = keys.length
		for (let i = 0; i < length; i++) {
			const key = keys[i]
			const hue = ((i * 360) / length) % 360
			config[key] = {
				label: key,
				color: `hsl(${hue}, 60%, 55%)`,
			}
		}
		return config satisfies ChartConfig
	}, [chartData])

	// if (!chartData.length || !ticks.length) {
	// 	return <Spinner />
	// }

	return (
		<div>
			{/* {!yAxisSet && <Spinner />} */}
			<ChartContainer
				config={{}}
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisWidth,
				})}
			>
				<AreaChart
					accessibilityLayer
					data={chartData}
					reverseStackOrder={true}
					margin={{
						top: 10,
					}}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						// domain={[0, (max: number) => Math.ceil(max)]}
						tickLine={false}
						axisLine={false}
						width={yAxisWidth}
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value / 1024, 2) + ' GB'
							return updateYAxisWidth(val)
						}}
					/>
					<XAxis
						dataKey="time"
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
						// cursor={false}
						animationEasing="ease-out"
						animationDuration={150}
						labelFormatter={(_, data) => formatShortDate(data[0].payload.time)}
						// @ts-ignore
						itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								filter={filter}
								contentFormatter={(item) => twoDecimalString(item.value) + ' MB'}
								indicator="line"
							/>
						}
					/>
					{Object.keys(chartConfig).map((key) => {
						const filtered = filter && !key.includes(filter)
						let fillOpacity = filtered ? 0.05 : 0.4
						let strokeOpacity = filtered ? 0.1 : 1
						return (
							<Area
								key={key}
								isAnimationActive={false}
								dataKey={key}
								type="monotoneX"
								fill={chartConfig[key].color}
								strokeOpacity={strokeOpacity}
								fillOpacity={fillOpacity}
								stroke={chartConfig[key].color}
								activeDot={filtered ? false : {}}
								stackId="a"
							/>
						)
					})}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
