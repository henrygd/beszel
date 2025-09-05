import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { cn, decimalString, formatShortDate, chartMargin, formatBytes, toFixedFloat } from "@/lib/utils"
import { memo } from "react"
import { ChartData } from "@/types"
import { useLingui } from "@lingui/react/macro"
import { Unit } from "@/lib/enums"
import { useYAxisWidth } from "./hooks"

export default memo(function MemChart({ chartData, showMax }: { chartData: ChartData; showMax: boolean }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t } = useLingui()

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
								const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
								return updateYAxisWidth(toFixedFloat(convertedValue, value >= 10 ? 0 : 1) + " " + unit)
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
								contentFormatter={({ value }) => {
									// mem values are supplied as GB
									const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
									return decimalString(convertedValue, convertedValue >= 100 ? 1 : 2) + " " + unit
								}}
							/>
						}
					/>
					<Area
						name={t`Used`}
						order={3}
						dataKey={({ stats }) => (showMax ? stats?.mm : stats?.mu)}
						type="monotoneX"
						fill="var(--chart-2)"
						fillOpacity={0.4}
						stroke="var(--chart-2)"
						stackId="1"
						isAnimationActive={false}
					/>
					{/* {chartData.systemStats.at(-1)?.stats.mz && ( */}
					<Area
						name="ZFS ARC"
						order={2}
						dataKey={({ stats }) => (showMax ? null : stats?.mz)}
						type="monotoneX"
						fill="hsla(175 60% 45% / 0.8)"
						fillOpacity={0.5}
						stroke="hsla(175 60% 45% / 0.8)"
						stackId="1"
						isAnimationActive={false}
					/>
					{/* )} */}
					<Area
						name={t`Cache / Buffers`}
						order={1}
						dataKey={({ stats }) => (showMax ? null : stats?.mb)}
						type="monotoneX"
						fill="hsla(160 60% 45% / 0.5)"
						fillOpacity={0.4}
						stroke="hsla(160 60% 45% / 0.5)"
						stackId="1"
						isAnimationActive={false}
					/>
					{/* <ChartLegend content={<ChartLegendContent />} /> */}
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
