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
	read: {
		label: 'Read',
		color: 'hsl(var(--chart-1))',
	},
	write: {
		label: 'Write',
		color: 'hsl(var(--chart-3))',
	},
} satisfies ChartConfig

export default function DiskIoChart({
	chartData,
	ticks,
}: {
	chartData: { time: number; read: number; write: number }[]
	ticks: number[]
}) {
	const chartTime = useStore($chartTime)

	if (!chartData.length || !ticks.length) {
		return <Spinner />
	}

	return (
		<ChartContainer config={chartConfig} className="h-full w-full absolute aspect-auto">
			<AreaChart
				accessibilityLayer
				data={chartData}
				margin={{
					left: 0,
					right: 0,
					top: 10,
					bottom: 0,
				}}
			>
				<CartesianGrid vertical={false} />
				<YAxis
					className="tracking-tighter"
					width={75}
					domain={[0, (max: number) => (max <= 0.4 ? 0.4 : Math.ceil(max))]}
					tickFormatter={(value) => {
						if (value >= 100) {
							return value.toFixed(0)
						}
						return value.toFixed((value * 100) % 1 === 0 ? 1 : 2)
					}}
					tickLine={false}
					axisLine={false}
					unit={' MB/s'}
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
							unit=" MB/s"
							labelFormatter={(_, data) => formatShortDate(data[0].payload.time)}
							indicator="line"
						/>
					}
				/>
				<Area
					dataKey="write"
					type="monotoneX"
					fill="var(--color-write)"
					fillOpacity={0.4}
					stroke="var(--color-write)"
					animationDuration={1200}
				/>
				<Area
					dataKey="read"
					type="monotoneX"
					fill="var(--color-read)"
					fillOpacity={0.4}
					stroke="var(--color-read)"
					animationDuration={1200}
				/>
			</AreaChart>
		</ChartContainer>
	)
}
