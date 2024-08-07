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

export default function SwapChart({
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

	return (
		<div ref={chartRef}>
			<ChartContainer
				config={{}}
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisSet,
				})}
			>
				<AreaChart accessibilityLayer data={systemData} margin={{ top: 10 }}>
					<CartesianGrid vertical={false} />
					<YAxis
						className="tracking-tighter"
						domain={[0, () => toFixedWithoutTrailingZeros(systemData.at(-1)?.stats.s ?? 0.04, 2)]}
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
}
