import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { calculateXaxisTicks, formatShortDate, formatShortTime } from '@/lib/utils'
import Spinner from '../spinner'
import { useMemo } from 'react'

const chartConfig = {
	cpu: {
		label: 'CPU Usage',
		color: 'hsl(var(--chart-1))',
	},
} satisfies ChartConfig

export default function CpuChart({ chartData }: { chartData: { time: number; cpu: number }[] }) {
	if (!chartData?.length) {
		return <Spinner />
	}

	const ticks = useMemo(() => calculateXaxisTicks(chartData), [chartData])

	return (
		<ChartContainer config={chartConfig} className="h-full w-full absolute aspect-auto">
			<AreaChart accessibilityLayer data={chartData} margin={{ top: 10 }}>
				<CartesianGrid vertical={false} />
				<YAxis
					domain={[0, (max: number) => Math.ceil(max)]}
					width={47}
					tickLine={false}
					axisLine={false}
					unit={'%'}
				/>
				{/* todo: short time if first date is same day, otherwise short date */}
				<XAxis
					dataKey="time"
					domain={[ticks[0], ticks.at(-1)!]}
					ticks={ticks}
					type="number"
					scale={'time'}
					axisLine={false}
					tickMargin={8}
					minTickGap={35}
					tickFormatter={formatShortTime}
				/>
				<ChartTooltip
					animationEasing="ease-out"
					animationDuration={150}
					content={
						<ChartTooltipContent
							unit="%"
							labelFormatter={(_, data) => formatShortDate(data[0].payload.time)}
							indicator="line"
						/>
					}
				/>
				<Area
					dataKey="cpu"
					type="monotone"
					fill="var(--color-cpu)"
					fillOpacity={0.4}
					stroke="var(--color-cpu)"
					animateNewValues={false}
				/>
			</AreaChart>
		</ChartContainer>
	)
}
