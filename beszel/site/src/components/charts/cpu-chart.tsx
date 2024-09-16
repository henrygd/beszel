import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { useYAxisWidth, chartTimeData, cn, formatShortDate, twoDecimalString } from '@/lib/utils'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'

export default function CpuChart({
	ticks,
	systemData,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
}) {
	const chartTime = useStore($chartTime)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	return (
		<div>
			<ChartContainer
				config={{}}
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisWidth,
				})}
			>
				<AreaChart
					accessibilityLayer
					data={systemData}
					margin={{ top: 10 }}
					// syncId={'cpu'}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						// domain={[0, (max: number) => Math.ceil(max)]}
						width={yAxisWidth}
						tickLine={false}
						axisLine={false}
						tickFormatter={(value) => updateYAxisWidth(value + '%')}
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
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => twoDecimalString(item.value) + '%'}
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
						isAnimationActive={false}
						// animationEasing="ease-out"
						// animationDuration={1200}
						// animateNewValues={true}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
