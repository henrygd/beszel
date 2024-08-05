import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { chartTimeData, cn, formatShortDate, useYaxisWidth } from '@/lib/utils'
import { useMemo, useRef } from 'react'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'

export default function MemChart({
	ticks,
	systemData,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
}) {
	const chartTime = useStore($chartTime)
	const chartRef = useRef<HTMLDivElement>(null)
	const yAxisWidth = useYaxisWidth(chartRef)

	const yAxisSet = useMemo(() => yAxisWidth !== 180, [yAxisWidth])

	const totalMem = useMemo(() => {
		const maxMem = Math.ceil(systemData[0]?.stats.m)
		return maxMem > 2 && maxMem % 2 !== 0 ? maxMem + 1 : maxMem
	}, [systemData])

	// if (!systemData.length || !ticks.length) {
	// 	return <Spinner />
	// }

	return (
		<div ref={chartRef}>
			{/* {!yAxisSet && <Spinner />} */}
			<ChartContainer
				config={{}}
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisSet,
				})}
			>
				<AreaChart
					accessibilityLayer
					data={systemData}
					margin={{
						top: 10,
					}}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						// use "ticks" instead of domain / tickcount if need more control
						domain={[0, totalMem]}
						width={yAxisWidth}
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
						// cursor={false}
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								unit=" GB"
								// @ts-ignore
								itemSorter={(a, b) => a.name.localeCompare(b.name)}
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								indicator="line"
							/>
						}
					/>
					<Area
						dataKey="stats.mu"
						name="Used"
						type="monotoneX"
						fill="hsl(var(--chart-2))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-2))"
						stackId="a"
						// animationDuration={1200}
						isAnimationActive={false}
					/>
					<Area
						dataKey="stats.mb"
						name="Cache / Buffers"
						type="monotoneX"
						fill="hsl(var(--chart-2))"
						fillOpacity={0.2}
						strokeOpacity={0.3}
						stroke="hsl(var(--chart-2))"
						stackId="a"
						// animationDuration={1200}
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
