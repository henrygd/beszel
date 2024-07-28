import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { chartTimeData, formatShortDate } from '@/lib/utils'
import { useMemo } from 'react'
import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'

export default function MemChart({
	chartData,
	ticks,
}: {
	chartData: { time: number; mem: number; memUsed: number; memCache: number }[]
	ticks: number[]
}) {
	const chartTime = useStore($chartTime)

	const totalMem = useMemo(() => {
		const maxMem = Math.ceil(chartData[0]?.mem)
		return maxMem > 2 && maxMem % 2 !== 0 ? maxMem + 1 : maxMem
	}, [chartData])

	const chartConfig = useMemo(
		() => ({
			memCache: {
				label: 'Cache / Buffers',
				color: 'hsl(var(--chart-2))',
			},
			memUsed: {
				label: 'Used',
				color: 'hsl(var(--chart-2))',
			},
		}),
		[]
	) satisfies ChartConfig

	if (!chartData.length || !ticks.length) {
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
			>
				<CartesianGrid vertical={false} />
				<YAxis
					// use "ticks" instead of domain / tickcount if need more control
					domain={[0, totalMem]}
					tickLine={false}
					width={totalMem >= 100 ? 65 : 58}
					// allowDecimals={false}
					axisLine={false}
					unit={' GB'}
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
					content={
						<ChartTooltipContent
							unit=" GB"
							// @ts-ignore
							itemSorter={(a, b) => a.name.localeCompare(b.name)}
							labelFormatter={(_, data) => formatShortDate(data[0].payload.time)}
							indicator="line"
						/>
					}
				/>
				<Area
					dataKey="memUsed"
					type="monotoneX"
					fill="var(--color-memUsed)"
					fillOpacity={0.4}
					stroke="var(--color-memUsed)"
					stackId="a"
					animationDuration={1200}
				/>
				<Area
					dataKey="memCache"
					type="monotoneX"
					fill="var(--color-memCache)"
					fillOpacity={0.2}
					strokeOpacity={0.3}
					stroke="var(--color-memCache)"
					stackId="a"
					animationDuration={1200}
				/>
			</AreaChart>
		</ChartContainer>
	)
}
