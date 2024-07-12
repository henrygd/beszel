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
// for (const data of chartData) {
//   data.month = formatDateShort(data.month)
// }

const chartConfig = {
	diskUsed: {
		label: 'Disk Use',
		color: 'hsl(var(--chart-3))',
	},
} satisfies ChartConfig

export default function ({
	chartData,
}: {
	chartData: { time: string; disk: number; diskUsed: number }[]
}) {
	const diskSize = useMemo(() => {
		return Math.round(chartData[0]?.disk)
	}, [chartData])

	// const ticks = useMemo(() => {
	// 	let ticks = [0]
	// 	for (let i = 1; i < diskSize; i += diskSize / 5) {
	// 		ticks.push(Math.trunc(i))
	// 	}
	// 	ticks.push(diskSize)
	// 	return ticks
	// }, [diskSize])

	if (!chartData.length) {
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
					width={75}
					domain={[0, diskSize]}
					// ticks={ticks}
					tickCount={9}
					minTickGap={8}
					tickLine={false}
					axisLine={false}
					unit={' GiB'}
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
						<ChartTooltipContent unit=" GiB" labelFormatter={formatShortDate} indicator="line" />
					}
				/>
				<Area
					dataKey="diskUsed"
					type="monotone"
					fill="var(--color-diskUsed)"
					fillOpacity={0.4}
					stroke="var(--color-diskUsed)"
				/>
			</AreaChart>
		</ChartContainer>
	)
}
