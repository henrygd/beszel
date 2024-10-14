import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import {
	useYAxisWidth,
	chartTimeData,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	decimalString,
	chartMargin,
} from '@/lib/utils'
import { ChartTimes, SystemStatsRecord } from '@/types'
import { memo } from 'react'

export default memo(function SwapChart({
	systemChartData,
}: {
	systemChartData: {
		systemStats: SystemStatsRecord[]
		ticks: number[]
		domain: number[]
		chartTime: ChartTimes
	}
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

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
						domain={[
							0,
							() =>
								toFixedWithoutTrailingZeros(systemChartData.systemStats.at(-1)?.stats.s ?? 0.04, 2),
						]}
						width={yAxisWidth}
						tickLine={false}
						axisLine={false}
						tickFormatter={(value) => updateYAxisWidth(value + ' GB')}
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
								contentFormatter={(item) => decimalString(item.value) + ' GB'}
								// indicator="line"
							/>
						}
					/>
					<Area
						dataKey="stats.su"
						name="Swap Usage"
						type="monotoneX"
						fill="hsl(var(--chart-2))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-2))"
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
