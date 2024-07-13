'use client'

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { useMemo } from 'react'
import { formatShortDate, formatShortTime } from '@/lib/utils'
import Spinner from '../spinner'

export default function ({ chartData }: { chartData: Record<string, number | string>[] }) {
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
				color: `hsl(${hue}, 60%, 60%)`,
			}
		}
		return config satisfies ChartConfig
	}, [chartData])

	if (!chartData.length) {
		return <Spinner />
	}

	return (
		<ChartContainer config={chartConfig} className="h-full w-full absolute aspect-auto">
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
					domain={[0, (max: number) => Math.max(Math.ceil(max), 0.4)]}
					// tickCount={5}
					tickLine={false}
					axisLine={false}
					unit={'%'}
					tickFormatter={(x) => (x % 1 === 0 ? x : x.toFixed(1))}
				/>
				<XAxis
					dataKey="time"
					tickLine={true}
					axisLine={false}
					tickMargin={8}
					minTickGap={30}
					tickFormatter={formatShortTime}
				/>
				<ChartTooltip
					cursor={false}
					labelFormatter={formatShortDate}
					// @ts-ignore
					itemSorter={(a, b) => b.value - a.value}
					content={<ChartTooltipContent unit="%" indicator="line" />}
				/>
				{Object.keys(chartConfig).map((key) => (
					<Area
						key={key}
						animateNewValues={false}
						dataKey={key}
						type="monotone"
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
