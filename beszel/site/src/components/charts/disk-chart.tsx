import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import {
	useYAxisWidth,
	chartTimeData,
	cn,
	formatShortDate,
	decimalString,
	toFixedFloat,
	getSizeVal,
	getSizeUnit,
	chartMargin,
} from '@/lib/utils'
import { ChartTimes, SystemStatsRecord } from '@/types'
import { memo } from 'react'

export default memo(function DiskChart({
	dataKey,
	diskSize,
	systemChartData,
}: {
	dataKey: string
	diskSize: number
	systemChartData: {
		systemStats: SystemStatsRecord[]
		ticks: number[]
		domain: number[]
		chartTime: ChartTimes
	}
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	// console.log('rendered at', new Date())

	return (
		<div>
			<ChartContainer
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisWidth,
				})}
			>
				<AreaChart accessibilityLayer data={systemChartData.systemStats} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						width={yAxisWidth}
						domain={[0, diskSize]}
						tickCount={9}
						minTickGap={6}
						tickLine={false}
						axisLine={false}
						tickFormatter={(value) =>
							updateYAxisWidth(toFixedFloat(getSizeVal(value), 2) + getSizeUnit(value))
						}
					/>
					<XAxis
						dataKey="created"
						domain={systemChartData.domain}
						ticks={systemChartData.ticks}
						allowDataOverflow
						type="number"
						scale="time"
						minTickGap={30}
						tickMargin={8}
						axisLine={false}
						tickFormatter={chartTimeData[systemChartData.chartTime].format}
					/>
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={({ value }) =>
									decimalString(getSizeVal(value)) + getSizeUnit(value)
								}
								// indicator="line"
							/>
						}
					/>
					<Area
						dataKey={dataKey}
						name="Disk Usage"
						type="monotoneX"
						fill="hsl(var(--chart-4))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-4))"
						// animationDuration={1200}
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
