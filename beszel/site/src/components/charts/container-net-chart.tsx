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
import { Separator } from '@/components/ui/separator'

export default function ContainerCpuChart({
	chartData,
	ticks,
}: {
	chartData: Record<string, number | number[]>[]
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
				if (!Array.isArray(stats[key])) {
					continue
				}
				if (!(key in totalUsage)) {
					totalUsage[key] = 0
				}
				totalUsage[key] += stats[key][2] ?? 0
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
					margin={{
						top: 10,
					}}
					reverseStackOrder={true}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						// domain={[0, (max: number) => Math.max(Math.ceil(max), 0.4)]}
						width={yAxisWidth}
						tickLine={false}
						axisLine={false}
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value, 2) + ' MB/s'
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
								indicator="line"
								contentFormatter={(item, key) => {
									try {
										const sent = item?.payload?.[key][0] ?? 0
										const received = item?.payload?.[key][1] ?? 0
										return (
											<span className="flex">
												{twoDecimalString(received)} MB/s
												<span className="opacity-70 ml-0.5"> rx </span>
												<Separator orientation="vertical" className="h-3 mx-1.5 bg-primary/40" />
												{twoDecimalString(sent)} MB/s<span className="opacity-70 ml-0.5"> tx</span>
											</span>
										)
									} catch (e) {
										return null
									}
								}}
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
								name={key}
								// animationDuration={1200}
								isAnimationActive={false}
								dataKey={(data) => data?.[key]?.[2] ?? 0}
								type="monotoneX"
								fill={chartConfig[key].color}
								fillOpacity={fillOpacity}
								stroke={chartConfig[key].color}
								strokeOpacity={strokeOpacity}
								activeDot={{ opacity: filtered ? 0 : 1 }}
								stackId="a"
							/>
						)
					})}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
