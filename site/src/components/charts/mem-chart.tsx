import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { formatShortDate, formatShortTime } from '@/lib/utils'
import { useMemo } from 'react'
import Spinner from '../spinner'

export default function ({
	chartData,
}: {
	chartData: { time: string; mem: number; memUsed: number; memCache: number }[]
}) {
	const totalMem = useMemo(() => {
		return Math.ceil(chartData[0]?.mem)
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
			>
				<CartesianGrid vertical={false} />
				<YAxis
					// use "ticks" instead of domain / tickcount if need more control
					domain={[0, totalMem]}
					tickCount={9}
					tickLine={false}
					allowDecimals={false}
					axisLine={false}
					tickFormatter={(v) => `${v} GiB`}
				/>
				{/* todo: short time if first date is same day, otherwise short date */}
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
					content={
						<ChartTooltipContent
							unit="GiB"
							// @ts-ignore
							itemSorter={(a, b) => a.name.localeCompare(b.name)}
							labelFormatter={formatShortDate}
							indicator="line"
						/>
					}
				/>
				<Area
					dataKey="memUsed"
					type="monotone"
					fill="var(--color-memUsed)"
					fillOpacity={0.4}
					stroke="var(--color-memUsed)"
					stackId="a"
				/>
				<Area
					dataKey="memCache"
					type="monotone"
					fill="var(--color-memCache)"
					fillOpacity={0.2}
					strokeOpacity={0.3}
					stroke="var(--color-memCache)"
					stackId="a"
				/>
			</AreaChart>
		</ChartContainer>
	)
}
