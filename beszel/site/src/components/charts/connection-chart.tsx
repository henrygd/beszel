import { memo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, ChartLegend, ChartLegendContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, chartMargin, decimalString } from "@/lib/utils"
import { ChartData } from "@/types"

export default memo(function ConnectionChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t } = useLingui()

	if (chartData.systemStats.length === 0) {
		return null
	}

	const dataKeys = [
		{
			name: t`Total`,
			dataKey: "stats.cc.t",
			color: "hsl(220, 70%, 50%)", // Blue
		},
		{
			name: t`TCP`,
			dataKey: "stats.cc.tcp",
			color: "hsl(142, 70%, 45%)", // Green
		},
		{
			name: t`UDP`,
			dataKey: "stats.cc.udp",
			color: "hsl(48, 96%, 53%)", // Yellow
		},
		{
			name: t`Established`,
			dataKey: "stats.cc.e",
			color: "hsl(271, 81%, 56%)", // Purple
		},
		{
			name: t`Listening`,
			dataKey: "stats.cc.l",
			color: "hsl(9, 78%, 56%)", // Red
		}
	]

	return (
		<div>
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart accessibilityLayer data={chartData.systemStats} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						width={yAxisWidth}
						tickFormatter={(value) => updateYAxisWidth(value.toString())}
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
								contentFormatter={({ value }) => value.toString()}
							/>
						}
					/>
					<ChartLegend content={<ChartLegendContent />} />
					{dataKeys.map((key, i) => (
						<Area
							key={i}
							dataKey={key.dataKey}
							name={key.name}
							type="monotoneX"
							fill={key.color}
							fillOpacity={0.3}
							stroke={key.color}
							strokeOpacity={1}
							isAnimationActive={false}
						/>
					))}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}) 
