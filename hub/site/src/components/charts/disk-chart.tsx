import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { chartTimeData, formatShortDate } from '@/lib/utils'
import { useMemo } from 'react'
import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'

export default function DiskChart({
	ticks,
	systemData,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
}) {
	const chartTime = useStore($chartTime)

	const diskSize = useMemo(() => {
		return Math.round(systemData[0]?.stats.d)
	}, [systemData])

	// const ticks = useMemo(() => {
	// 	let ticks = [0]
	// 	for (let i = 1; i < diskSize; i += diskSize / 5) {
	// 		ticks.push(Math.trunc(i))
	// 	}
	// 	ticks.push(diskSize)
	// 	return ticks
	// }, [diskSize])

	if (!systemData.length || !ticks.length) {
		return <Spinner />
	}

	return (
		<ChartContainer config={{}} className="h-full w-full absolute aspect-auto">
			<AreaChart
				accessibilityLayer
				data={systemData}
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
					dataKey="created"
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
							labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
							indicator="line"
						/>
					}
				/>
				<Area
					dataKey="stats.du"
					name="Disk Usage"
					type="monotoneX"
					fill="hsl(var(--chart-4))"
					fillOpacity={0.4}
					stroke="hsl(var(--chart-4))"
					animationDuration={1200}
				/>
			</AreaChart>
		</ChartContainer>
	)
}
