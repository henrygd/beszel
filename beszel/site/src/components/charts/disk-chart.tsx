import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import {
	useYAxisWidth,
	chartTimeData,
	cn,
	formatShortDate,
	twoDecimalString,
	toFixedFloat,
	getSizeVal,
	getSizeUnit,
} from '@/lib/utils'
// import { useMemo } from 'react'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'

export default function DiskChart({
	ticks,
	systemData,
	dataKey,
	diskSize,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
	dataKey: string
	diskSize: number
}) {
	const chartTime = useStore($chartTime)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	return (
		<div>
			{/* {!yAxisSet && <Spinner />} */}
			<ChartContainer
				config={{}}
				className={cn('h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity', {
					'opacity-100': yAxisWidth,
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
								contentFormatter={({ value }) =>
									twoDecimalString(getSizeVal(value)) + getSizeUnit(value)
								}
								indicator="line"
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
}
