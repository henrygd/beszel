'use client'

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { useMemo } from 'react'
import { formatShortDate, hourWithMinutes } from '@/lib/utils'
import Spinner from '../spinner'

export default function ({
	chartData,
	ticks,
}: {
	chartData: Record<string, number | string>[]
	ticks: number[]
}) {
	if (!chartData.length || !ticks.length) {
		return <Spinner />
	}

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

	return (
		<ChartContainer config={chartConfig} className="h-full w-full absolute aspect-auto">
			<AreaChart
				accessibilityLayer
				data={chartData}
				reverseStackOrder={true}
				margin={{
					top: 10,
				}}

				// reverseStackOrder={true}
			>
				<CartesianGrid vertical={false} />
				<YAxis
					domain={[0, (max: number) => Math.ceil(max)]}
					// tickCount={9}
					allowDecimals={false}
					tickLine={false}
					axisLine={false}
					unit={' GB'}
					tickFormatter={(x) => {
						x = x / 1024
						return x % 1 === 0 ? x : x.toFixed(1)
					}}
				/>
				<XAxis
					dataKey="time"
					domain={[ticks[0], ticks.at(-1)!]}
					ticks={ticks}
					type="number"
					scale={'time'}
					tickLine={true}
					axisLine={false}
					tickMargin={8}
					minTickGap={30}
					tickFormatter={hourWithMinutes}
				/>
				<ChartTooltip
					// cursor={false}
					animationEasing="ease-out"
					animationDuration={150}
					labelFormatter={(_, data) => formatShortDate(data[0].payload.time)}
					// @ts-ignore
					itemSorter={(a, b) => b.value - a.value}
					content={<ChartTooltipContent unit=" MiB" indicator="line" />}
				/>
				{Object.keys(chartConfig).map((key) => (
					<Area
						key={key}
						isAnimationActive={chartData.length < 20}
						animateNewValues={false}
						dataKey={key}
						type="monotoneX"
						fill={chartConfig[key].color}
						fillOpacity={0.4}
						stroke={chartConfig[key].color}
						stackId="a"
					/>
				))}
			</AreaChart>
		</ChartContainer>
	)
}
