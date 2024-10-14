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
	decimalString,
	chartMargin,
	toFixedWithoutTrailingZeros,
} from '@/lib/utils'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $containerFilter } from '@/lib/stores'
import { ChartTimes, ContainerStats } from '@/types'

export default memo(function ContainerCpuChart({
	dataKey,
	unit = '%',
	containerChartData,
}: {
	dataKey: string
	unit?: string
	containerChartData: {
		containerData: Record<string, number | ContainerStats>[]
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
				if (!key || typeof stats[key] === 'number') {
					continue
				}
				if (!(key in totalUsage)) {
					totalUsage[key] = 0
				}
				// @ts-ignore
				totalUsage[key] += stats[key]?.[dataKey] ?? 0
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
			<ChartContainer
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisWidth,
				})}
			>
				<AreaChart
					accessibilityLayer
					// syncId={'cpu'}
					data={containerData}
					margin={chartMargin}
					reverseStackOrder={true}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						width={yAxisWidth}
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value, 2) + unit
							return updateYAxisWidth(val)
						}}
						tickLine={false}
						axisLine={false}
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
						animationEasing="ease-out"
						animationDuration={150}
						labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
						// @ts-ignore
						itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								filter={filter}
								contentFormatter={(item) => decimalString(item.value) + unit}
								// indicator="line"
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
								dataKey={`${key}.${dataKey}`}
								name={key}
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
