import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	toFixedFloat,
	decimalString,
	chartMargin,
} from "@/lib/utils"
import { ChartData } from "@/types"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"

type InodeChartProps = {
	chartData: ChartData
	showLegend?: boolean
	inodeTotal?: number
}

export default memo(function InodeChart({
	chartData,
	showLegend = true,
	inodeTotal,
}: InodeChartProps) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t } = useLingui()

	// Transform data to show inode usage
	const transformedData = useMemo(() => {
		return chartData.systemStats.map(point => {
			const inodeUsed = point.stats?.iu || 0
			const inodeTotal = point.stats?.it || 0
			const inodeFree = Math.max(0, inodeTotal - inodeUsed)
			
			return {
				...point,
				inodeUsed,
				inodeFree,
				inodeTotal,
			}
		})
	}, [chartData.systemStats])

	const areas = useMemo(() => [
		{ 
			label: t`Used`, 
			dataKey: "inodeUsed", 
			color: 2, 
			opacity: 0.35 
		},
	], [t])

	const maxInodes = inodeTotal || Math.max(...transformedData.map(d => d.inodeTotal))

	return useMemo(() => {
		if (transformedData.length === 0 || maxInodes === 0) {
			return null
		}

		return (
			<div>
				<ChartContainer
					className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
						"opacity-100": yAxisWidth,
					})}
				>
					<AreaChart accessibilityLayer data={transformedData} margin={chartMargin}>
						<CartesianGrid vertical={false} />
						<YAxis
							direction="ltr"
							orientation={chartData.orientation}
							className="tracking-tighter"
							width={yAxisWidth}
							domain={[0, maxInodes]}
							type="number"
							allowDecimals={false}
							tickFormatter={(value, index) => {
								// Format large numbers (K, M) - ensure whole numbers
								const roundedValue = Math.round(value)
								let val: string
								if (roundedValue >= 1000000) {
									val = (roundedValue / 1000000).toFixed(roundedValue % 1000000 === 0 ? 0 : 1) + "M"
								} else if (roundedValue >= 1000) {
									val = (roundedValue / 1000).toFixed(roundedValue % 1000 === 0 ? 0 : 1) + "K"
								} else {
									val = roundedValue.toString()
								}
								return updateYAxisWidth(val)
							}}
							tickLine={false}
							axisLine={false}
						/>
						{xAxis(chartData)}
						<ChartTooltip
							animationEasing="ease-out"
							animationDuration={150}
							content={
								<ChartTooltipContent
									labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
									contentFormatter={({ value, name, payload }) => {
										const inodeTotal = payload.inodeTotal
										const percent = inodeTotal > 0 ? ((value / inodeTotal) * 100).toFixed(1) : "0"
										
										// Format the number
										let formattedValue: string
										if (value >= 1000000) {
											formattedValue = (value / 1000000).toFixed(2) + "M"
										} else if (value >= 1000) {
											formattedValue = (value / 1000).toFixed(1) + "K"
										} else {
											formattedValue = Math.round(value).toString()
										}
										
										return `${formattedValue} (${percent}%)`
									}}
								/>
							}
						/>
						{areas.map((area, i) => {
							const color = `var(--chart-${area.color})`
							return (
								<Area
									key={i}
									dataKey={area.dataKey}
									name={area.label}
									type="monotoneX"
									fill={color}
									fillOpacity={area.opacity}
									stroke={color}
									isAnimationActive={false}
								/>
							)
						})}
						{showLegend && (
							<ChartLegend content={<ChartLegendContent />} />
						)}
					</AreaChart>
				</ChartContainer>
			</div>
		)
	}, [transformedData.at(-1), yAxisWidth, maxInodes])
})