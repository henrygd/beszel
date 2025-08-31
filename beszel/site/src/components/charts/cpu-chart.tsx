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
import { ChartData, SystemStatsRecord, CpuCoreStats } from "@/types"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { $cpuViewMode } from "@/lib/stores"

const getNestedValue = (path: string, max = false, data: any): number | null => {
	const value = `stats.${path}${max ? "m" : ""}`
		.split(".")
		.reduce((acc: any, key: string) => acc?.[key] ?? (data.stats?.cpum ? 0 : null), data)
	
	// For CPU metrics, return 0 if the value is undefined or null
	if (path.startsWith('cpu') && (value === null || value === undefined)) {
		return 0
	}
	
	return value
}

type CpuChartProps = {
	maxToggled?: boolean
	chartData: ChartData
	showLegend?: boolean
}

export default memo(function CpuChart({
	chartData,
	maxToggled,
	showLegend = true,
}: CpuChartProps) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t, i18n } = useLingui()
	const cpuViewMode = useStore($cpuViewMode)

	const { chartTime } = chartData
	const showMax = chartTime !== "1h" && maxToggled

	// Get the list of CPU cores from the latest data point
	const cpuCores = useMemo(() => {
		const latestStats = chartData.systemStats.at(-1)?.stats
		if (!latestStats?.cpuc || cpuViewMode === "total") {
			return []
		}
		return Object.keys(latestStats.cpuc).sort()
	}, [chartData.systemStats, cpuViewMode])

	// Transform data for per-core view
	const transformedData = useMemo(() => {
		if (cpuViewMode === "total") {
			return chartData.systemStats
		}

		// Transform data to show per-core metrics
		return chartData.systemStats.map(point => {
			const transformed = { ...point }
			if (point.stats?.cpuc) {
				// Add per-core data as flattened properties
				Object.entries(point.stats.cpuc).forEach(([coreId, coreStats]) => {
					transformed[`core_${coreId}_user`] = coreStats.u
					transformed[`core_${coreId}_system`] = coreStats.s
					transformed[`core_${coreId}_iowait`] = coreStats.i
					transformed[`core_${coreId}_steal`] = coreStats.st
				})
			}
			return transformed
		})
	}, [chartData.systemStats, cpuViewMode])

	// Generate areas based on view mode
	const areas = useMemo(() => {
		if (cpuViewMode === "total") {
			return [
				{ label: t`User`, dataKey: getNestedValue.bind(null, "cpuu", showMax), color: 1, opacity: 0.4 },
				{ label: t`System`, dataKey: getNestedValue.bind(null, "cpus", showMax), color: 2, opacity: 0.4 },
				{ label: t`I/O Wait`, dataKey: getNestedValue.bind(null, "cpui", showMax), color: 3, opacity: 0.4 },
				{ label: t`Steal`, dataKey: getNestedValue.bind(null, "cpusl", showMax), color: 4, opacity: 0.4 },
			]
		} else {
			// Per-core view - show all cores with their user + system combined
			return cpuCores.map((coreId, index) => ({
				label: `Core ${coreId}`,
				dataKey: (data: any) => {
					const user = data[`core_${coreId}_user`] || 0
					const system = data[`core_${coreId}_system`] || 0
					const iowait = data[`core_${coreId}_iowait`] || 0
					const steal = data[`core_${coreId}_steal`] || 0
					return user + system + iowait + steal
				},
				color: (index % 6) + 1, // Cycle through available colors
				opacity: 0.3,
			}))
		}
	}, [cpuViewMode, cpuCores, showMax, t, i18n.locale])

	return useMemo(() => {
		if (transformedData.length === 0) {
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
							domain={[0, "auto"]}
							tickFormatter={(value, index) => {
								const val = toFixedFloat(value, 2) + "%"
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
									contentFormatter={({ value }) => {
										return decimalString(value) + "%"
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
									stackId={cpuViewMode === "total" ? "cpu" : undefined}
								/>
							)
						})}
						{showLegend && areas.length > 1 && (
							<ChartLegend content={<ChartLegendContent />} />
						)}
					</AreaChart>
				</ChartContainer>
			</div>
		)
	}, [transformedData.at(-1), yAxisWidth, maxToggled, areas, cpuViewMode])
})