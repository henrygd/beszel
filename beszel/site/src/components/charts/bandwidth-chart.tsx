import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import {
	chartTimeData,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	useYaxisWidth,
} from '@/lib/utils'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'
import { useMemo, useRef } from 'react'

export default function BandwidthChart({
	ticks,
	systemData,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
}) {
	const chartRef = useRef<HTMLDivElement>(null)
	const yAxisWidth = useYaxisWidth(chartRef)
	const chartTime = useStore($chartTime)

	const yAxisSet = useMemo(() => yAxisWidth !== 180, [yAxisWidth])

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
						left: 0,
						right: 0,
						top: 10,
						bottom: 0,
					}}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						width={yAxisWidth}
						// domain={[0, (max: number) => (max <= 0.4 ? 0.4 : Math.ceil(max))]}
						tickFormatter={(value) => toFixedWithoutTrailingZeros(value, 2)}
						tickLine={false}
						axisLine={false}
						unit={' MB/s'}
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
								unit=" MB/s"
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								indicator="line"
							/>
						}
					/>
					<Area
						dataKey="stats.ns"
						name="Sent"
						type="monotoneX"
						fill="hsl(var(--chart-5))"
						fillOpacity={0.2}
						stroke="hsl(var(--chart-5))"
						// animationDuration={1200}
						isAnimationActive={false}
					/>
					<Area
						dataKey="stats.nr"
						name="Received"
						type="monotoneX"
						fill="hsl(var(--chart-2))"
						fillOpacity={0.2}
						stroke="hsl(var(--chart-2))"
						// animationDuration={1200}
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
