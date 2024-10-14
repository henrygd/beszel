import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { memo, useMemo } from 'react'
import {
	useYAxisWidth,
	chartTimeData,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	decimalString,
	chartMargin,
} from '@/lib/utils'
import { useStore } from '@nanostores/react'
import { $containerFilter } from '@/lib/stores'
import { Separator } from '@/components/ui/separator'
import { ChartTimes, ContainerStats } from '@/types'

export default memo(function ContainerCpuChart({
	containerChartData,
}: {
	containerChartData: {
		containerData: Record<string, ContainerStats | number>[]
		ticks: number[]
		domain: number[]
		chartTime: ChartTimes
	}
}) {
	const filter = useStore($containerFilter)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	const { containerData, ticks, domain, chartTime } = containerChartData

	const chartConfig = useMemo(() => {
		let config = {} as Record<
			string,
			{
				label: string
				color: string
			}
		>
		const totalUsage = {} as Record<string, number>
		for (let stats of containerData) {
			for (let key in stats) {
				// continue if number and not container stats
				if (!key || typeof stats[key] === 'number') {
					continue
				}
				if (!(key in totalUsage)) {
					totalUsage[key] = 0
				}
				totalUsage[key] += stats[key]?.ns ?? 0 + stats[key]?.nr ?? 0
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
	}, [containerChartData])

	// console.log('rendered at', new Date())

	return (
		<div>
			{/* {!yAxisSet && <Spinner />} */}
			<ChartContainer
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisWidth,
				})}
			>
				<AreaChart
					accessibilityLayer
					data={containerData}
					margin={chartMargin}
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
						dataKey="created"
						domain={domain}
						allowDataOverflow
						ticks={ticks}
						type="number"
						scale="time"
						minTickGap={35}
						tickMargin={8}
						axisLine={false}
						tickFormatter={chartTimeData[chartTime].format}
					/>
					<ChartTooltip
						// cursor={false}
						animationEasing="ease-out"
						animationDuration={150}
						labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
						// @ts-ignore
						itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								filter={filter}
								// indicator="line"
								contentFormatter={(item, key) => {
									try {
										const sent = item?.payload?.[key]?.ns ?? 0
										const received = item?.payload?.[key]?.nr ?? 0
										return (
											<span className="flex">
												{decimalString(received)} MB/s
												<span className="opacity-70 ml-0.5"> rx </span>
												<Separator orientation="vertical" className="h-3 mx-1.5 bg-primary/40" />
												{decimalString(sent)} MB/s<span className="opacity-70 ml-0.5"> tx</span>
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
								isAnimationActive={false}
								dataKey={(data) => data?.[key]?.ns ?? 0 + data?.[key]?.nr ?? 0}
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
})
