import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { chartTimeData, formatShortDate, useYaxisWidth } from '@/lib/utils'
import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'
import { useRef } from 'react'

export default function CpuChart({
	ticks,
	systemData,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
}) {
	const chartRef = useRef<HTMLDivElement>(null)
	const yAxisWidth = useYaxisWidth(chartRef)
	const chartTime = useStore($chartTime)

	if (!systemData.length || !ticks.length) {
		return <Spinner />
	}

	return (
		<div ref={chartRef}>
			<ChartContainer config={{}} className="h-full w-full absolute aspect-auto">
				<AreaChart accessibilityLayer data={systemData} margin={{ top: 10 }}>
					<CartesianGrid vertical={false} />
					<YAxis
						// domain={[0, (max: number) => Math.ceil(max)]}
						width={yAxisWidth}
						tickLine={false}
						axisLine={false}
						unit={'%'}
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
								unit="%"
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								indicator="line"
							/>
						}
					/>
					<Area
						dataKey="stats.cpu"
						name="CPU Usage"
						type="monotoneX"
						fill="hsl(var(--chart-1))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-1))"
						animationDuration={1200}
						// animationEasing="ease-out"
						// animateNewValues={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
