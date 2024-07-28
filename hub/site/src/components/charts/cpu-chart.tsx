import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { chartTimeData, formatShortDate } from '@/lib/utils'
import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'

const chartConfig = {
	cpu: {
		label: 'CPU Usage',
		color: 'hsl(var(--chart-1))',
	},
} satisfies ChartConfig

export default function CpuChart({
	chartData,
	ticks,
}: {
	chartData: { time: number; cpu: number }[]
	ticks: number[]
}) {
	const chartTime = useStore($chartTime)

	if (!chartData.length || !ticks.length) {
		return <Spinner />
	}

	return (
		<ChartContainer config={chartConfig} className="h-full w-full absolute aspect-auto">
			<AreaChart accessibilityLayer data={chartData} margin={{ top: 10 }}>
				<CartesianGrid vertical={false} />
				<YAxis
					// domain={[0, (max: number) => Math.ceil(max)]}
					width={48}
					tickLine={false}
					axisLine={false}
					unit={'%'}
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
					type="monotoneX"
					fill="var(--color-cpu)"
					fillOpacity={0.4}
					stroke="var(--color-cpu)"
					animationDuration={1200}
					// animationEasing="ease-out"
					// animateNewValues={false}
				/>
			</AreaChart>
		</ChartContainer>
	)
}
