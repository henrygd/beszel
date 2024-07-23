import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { formatShortDate, hourWithMinutes } from '@/lib/utils'
import Spinner from '../spinner'

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
				{/* todo: short time if first date is same day, otherwise short date */}
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
