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
// import Spinner from '../spinner'
import { ChartTimes, SystemStatsRecord } from '@/types'
import { memo, useMemo } from 'react'

/** [label, key, color, opacity] */
type DataKeys = [string, string, number, number]

const getNestedValue = (path: string, max = false, data: any): number | null => {
	// fallback value (obj?.stats?.cpum ? 0 : null) should only come into play when viewing
	// a max value which doesn't exist, or the value was zero and omitted from the stats object.
	// so we check if cpum is present. if so, return 0 to make sure the zero value is displayed.
	// if not, return null - there is no max data so do not display anything.
	return `stats.${path}${max ? 'm' : ''}`
		.split('.')
		.reduce((acc: any, key: string) => acc?.[key] ?? (data.stats?.cpum ? 0 : null), data)
}

export default memo(function AreaChartDefault({
	maxToggled = false,
	unit = ' MB/s',
	chartName,
	systemChartData,
}: {
	maxToggled?: boolean
	unit?: string
	chartName: string
	systemChartData: {
		systemStats: SystemStatsRecord[]
		ticks: number[]
		domain: number[]
		chartTime: ChartTimes
	}
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	const { chartTime } = systemChartData

	const showMax = chartTime !== '1h' && maxToggled

	const dataKeys: DataKeys[] = useMemo(() => {
		// [label, key, color, opacity]
		if (chartName === 'CPU Usage') {
			return [[chartName, 'cpu', 1, 0.4]]
		} else if (chartName === 'dio') {
			return [
				['Write', 'dw', 3, 0.3],
				['Read', 'dr', 1, 0.3],
			]
		} else if (chartName === 'bw') {
			return [
				['Sent', 'ns', 5, 0.2],
				['Received', 'nr', 2, 0.2],
			]
		} else if (chartName.startsWith('efs')) {
			return [
				['Write', `${chartName}.w`, 3, 0.3],
				['Read', `${chartName}.r`, 1, 0.3],
			]
		}
		return []
	}, [])

	// console.log('Rendered at', new Date())

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
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value, 2) + unit
							return updateYAxisWidth(val)
						}}
						tickLine={false}
						axisLine={false}
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
						tickFormatter={chartTimeData[chartTime].format}
					/>
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => decimalString(item.value) + unit}
								// indicator="line"
							/>
						}
					/>
					{dataKeys.map((key, i) => {
						const color = `hsl(var(--chart-${key[2]}))`
						return (
							<Area
								key={i}
								dataKey={getNestedValue.bind(null, key[1], showMax)}
								name={key[0]}
								type="monotoneX"
								fill={color}
								fillOpacity={key[3]}
								stroke={color}
								isAnimationActive={false}
							/>
						)
					})}
					{/* <ChartLegend content={<ChartLegendContent />} /> */}
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
