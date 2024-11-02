import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"

import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, toFixedFloat, decimalString, formatShortDate, chartMargin } from "@/lib/utils"
import { memo } from "react"
import { ChartData } from "@/types"
import { t } from "@lingui/macro"
import { useLingui } from "@lingui/react"

export default memo(function MemChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { _ } = useLingui()

	const totalMem = toFixedFloat(chartData.systemStats.at(-1)?.stats.m ?? 0, 1)

	// console.log('rendered at', new Date())

	if (chartData.systemStats.length === 0) {
		return null
	}

	return (
		<div>
			{/* {!yAxisSet && <Spinner />} */}
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart accessibilityLayer data={chartData.systemStats} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					{totalMem && (
						<YAxis
							direction="ltr"
							orientation={chartData.orientation}
							// use "ticks" instead of domain / tickcount if need more control
							domain={[0, totalMem]}
							tickCount={9}
							className="tracking-tighter"
							width={yAxisWidth}
							tickLine={false}
							axisLine={false}
							tickFormatter={(value) => {
								const val = toFixedFloat(value, 1)
								return updateYAxisWidth(val + " GB")
							}}
						/>
					)}
					{xAxis(chartData)}
					<ChartTooltip
						// cursor={false}
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								// @ts-ignore
								itemSorter={(a, b) => a.order - b.order}
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => decimalString(item.value) + " GB"}
								// indicator="line"
							/>
						}
					/>
					<Area
						name={_(t`Used`)}
						order={3}
						dataKey="stats.mu"
						type="monotoneX"
						fill="hsl(var(--chart-2))"
						fillOpacity={0.4}
						stroke="hsl(var(--chart-2))"
						stackId="1"
						isAnimationActive={false}
					/>
					{chartData.systemStats.at(-1)?.stats.mz && (
						<Area
							name="ZFS ARC"
							order={2}
							dataKey="stats.mz"
							type="monotoneX"
							fill="hsla(175 60% 45% / 0.8)"
							fillOpacity={0.5}
							stroke="hsla(175 60% 45% / 0.8)"
							stackId="1"
							isAnimationActive={false}
						/>
					)}
					<Area
						name={_(t`Cache / Buffers`)}
						order={1}
						dataKey="stats.mb"
						type="monotoneX"
						fill="hsla(160 60% 45% / 0.5)"
						fillOpacity={0.4}
						// strokeOpacity={1}
						stroke="hsla(160 60% 45% / 0.5)"
						stackId="1"
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
