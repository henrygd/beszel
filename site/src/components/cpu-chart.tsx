import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { formatDateShort } from '@/lib/utils'

const chartData = [
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
	{ month: '2024-07-09 23:29:08.976Z', cpu: 6.2 },
	{ month: '2024-07-09 23:28:08.976Z', cpu: 2.8 },
	{ month: '2024-07-09 23:27:08.976Z', cpu: 9.5 },
	{ month: '2024-07-09 23:26:08.976Z', cpu: 23.4 },
	{ month: '2024-07-09 23:25:08.976Z', cpu: 4.3 },
	{ month: '2024-07-09 23:24:08.976Z', cpu: 9.1 },
]

// for (const data of chartData) {
//   data.month = formatDateShort(data.month)
// }

const chartConfig = {
	cpu: {
		label: 'cpu',
		color: 'hsl(var(--chart-1))',
	},
} satisfies ChartConfig

export default function () {
	return (
		<ChartContainer config={chartConfig} className="h-full w-full absolute aspect-auto">
			<AreaChart
				accessibilityLayer
				data={chartData}
				margin={{
					left: 0,
					right: 0,
					top: 7,
					bottom: 7,
				}}
			>
				<CartesianGrid vertical={false} />
				<YAxis domain={[0, 100]} tickCount={5} tickLine={false} axisLine={false} tickMargin={8} />
				<XAxis
					dataKey="month"
					tickLine={true}
					axisLine={false}
					tickMargin={8}
					minTickGap={30}
					tickFormatter={(value) => formatDateShort(value)}
				/>
				<ChartTooltip cursor={false} content={<ChartTooltipContent indicator="line" />} />
				<Area
					dataKey="cpu"
					type="natural"
					fill="var(--color-cpu)"
					fillOpacity={0.4}
					stroke="var(--color-cpu)"
				/>
			</AreaChart>
		</ChartContainer>
	)
}
