import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
	ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { chartTimeData, formatShortDate, hourWithMinutes } from '@/lib/utils'
import { useMemo } from 'react'
import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'

const chartConfig = {
	diskUsed: {
		label: 'Disk Usage',
		color: 'hsl(var(--chart-4))',
	},
} satisfies ChartConfig

export default function DiskChart({
	chartData,
	ticks,
}: {
	chartData: { time: number; disk: number; diskUsed: number }[]
	ticks: number[]
}) {
	const chartTime = useStore($chartTime)

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
					width={diskSize >= 1000 ? 75 : 65}
					domain={[0, diskSize]}
					tickCount={9}
					tickLine={false}
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
					animationEasing="ease-out"
					animationDuration={150}
					content={
						<ChartTooltipContent
							unit=" GB"
							labelFormatter={(_, data) => formatShortDate(data[0].payload.time)}
							indicator="line"
						/>
					}
				/>
				<Area
					dataKey="diskUsed"
					type="monotoneX"
					fill="var(--color-diskUsed)"
					fillOpacity={0.4}
					stroke="var(--color-diskUsed)"
					animationDuration={1200}
				/>
			</AreaChart>
		</ChartContainer>
	)
}
