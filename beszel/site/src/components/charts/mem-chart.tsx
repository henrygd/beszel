import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, toFixedFloat, decimalString, formatShortDate, chartMargin } from "@/lib/utils"
import { memo } from "react"
import { ChartData } from "@/types"
import { useLingui } from "@lingui/react/macro"
import { ChartLegend, ChartLegendContent } from "@/components/ui/chart"

type MemChartProps = { chartData: ChartData, showLegend?: boolean }
export default memo(function MemChart({ chartData, showLegend = true }: MemChartProps) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t } = useLingui()

	const totalMem = toFixedFloat(chartData.systemStats.at(-1)?.stats.m ?? 0, 1)

	// Compute free memory for each data point
	const memDataWithFree = chartData.systemStats.map((point) => {
		const m = point.stats && typeof point.stats.m === 'number' ? point.stats.m : 0;
		const mu = point.stats && typeof point.stats.mu === 'number' ? point.stats.mu : 0;
		const mb = point.stats && typeof point.stats.mb === 'number' ? point.stats.mb : 0;
		const mz = point.stats && typeof point.stats.mz === 'number' ? point.stats.mz : 0;
		const free = m - mu - mb - (mz || 0);
		return {
			...point,
			stats: {
				...point.stats,
				mf: free > 0 ? free : 0, // mf = memory free
			},
		}
	})

	// console.log('rendered at', new Date())

	if (chartData.systemStats.length === 0) {
		return null
	}

	// Define memory area keys, labels, colors, and opacities
	const dataKeys = [
		{ label: t`Free`, key: "mf", color: 1, opacity: 0.3 },
		{ label: t`Used`, key: "mu", color: 2, opacity: 0.4 },
		{ label: t`Cache / Buffers`, key: "mb", color: 3, opacity: 0.4 },
	]
	if (chartData.systemStats.at(-1)?.stats.mz) {
		dataKeys.splice(2, 0, { label: "ZFS ARC", key: "mz", color: 4, opacity: 0.5 })
	}

	return (
		<div>
			{/* {!yAxisSet && <Spinner />} */}
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart accessibilityLayer data={memDataWithFree} margin={chartMargin}>
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
					{dataKeys.map((item, i) => (
						<Area
							key={item.key}
							name={item.label}
							order={i}
							dataKey={`stats.${item.key}`}
							type="monotoneX"
							fill={`hsl(var(--chart-${item.color}))`}
							fillOpacity={item.opacity}
							stroke={`hsl(var(--chart-${item.color}))`}
							stackId="1"
							isAnimationActive={false}
						/>
					))}
					{showLegend && <ChartLegend content={<ChartLegendContent />} wrapperStyle={{ marginTop: 16 }} />}
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
