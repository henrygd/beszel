import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import {
	useYAxisWidth,
	chartTimeData,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	twoDecimalString,
} from '@/lib/utils'
// import Spinner from '../spinner'
import { useStore } from '@nanostores/react'
import { $chartTime } from '@/lib/stores'
import { SystemStatsRecord } from '@/types'

export default function DiskIoChart({
	ticks,
	systemData,
	dataKeys,
}: {
	ticks: number[]
	systemData: SystemStatsRecord[]
	dataKeys: string[]
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
						// domain={[0, (max: number) => (max <= 0.4 ? 0.4 : Math.ceil(max))]}
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value, 2) + ' MB/s'
							return updateYAxisWidth(val)
						}}
						tickLine={false}
						axisLine={false}
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
								contentFormatter={(item) => twoDecimalString(item.value) + ' MB/s'}
								indicator="line"
							/>
						}
					/>
					{dataKeys.map((dataKey, i) => {
						const action = i ? 'Read' : 'Write'
						const color = i ? 'hsl(var(--chart-1))' : 'hsl(var(--chart-3))'
						return (
							<Area
								key={i}
								dataKey={dataKey}
								name={action}
								type="monotoneX"
								fill={color}
								fillOpacity={0.3}
								stroke={color}
								isAnimationActive={false}
							/>
						)
					})}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}
