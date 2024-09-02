import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { chartTimeData, cn, formatShortDate, useYaxisWidth } from '@/lib/utils'
import { useMemo, useRef } from 'react'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'

export default function ExFsDiskChart({
	ticks,
	systemData,
	fs,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
	fs: string
}) {
	const chartTime = useStore($chartTime)
	const chartRef = useRef<HTMLDivElement>(null)
	const yAxisWidth = useYaxisWidth(chartRef)

	const yAxisSet = useMemo(() => yAxisWidth !== 180, [yAxisWidth])

	const diskSize = useMemo(() => {
		const size = systemData.at(-1)?.stats.efs?.[fs].d ?? 0
		return size > 10 ? Math.round(size) : size
	}, [systemData])

	return (
		<div ref={chartRef}>
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
						className="tracking-tighter"
						width={yAxisWidth}
						domain={[0, diskSize]}
						tickCount={9}
						minTickGap={6}
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
						dataKey={`stats.efs.${fs}.du`}
						name="Disk Usage"
						type="monotoneX"
						fill="hsl(var(--chart-4))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-4))"
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
