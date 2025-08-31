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

type ProcessChartProps = {
	chartData: ChartData
	showLegend?: boolean
}

export default memo(function ProcessChart({
	chartData,
	showLegend = true,
}: ProcessChartProps) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t } = useLingui()

	// Transform data to extract process states
	const transformedData = useMemo(() => {
		return chartData.systemStats.map(point => {
			const processes = point.stats?.ps || {}
			return {
				...point,
				running: processes.running || 0,
				sleeping: processes.sleeping || 0,
				disk_sleep: processes.disk_sleep || 0,
				zombie: processes.zombie || 0,
				stopped: processes.stopped || 0,
				idle: processes.idle || 0,
				other: processes.other || 0,
			}
		})
	}, [chartData.systemStats])

	// Color mapping for different process states
	const processColors = useMemo(() => ({
		running: "#10b981",     // green
		sleeping: "#3b82f6",    // blue  
		disk_sleep: "#f59e0b",  // amber
		zombie: "#ef4444",      // red
		stopped: "#8b5cf6",     // violet
		idle: "#bb650aff",        // orange
		other: "#ec4899",       // pink
	}), [])

	const areas = useMemo(() => [
		{ label: t`Running`, dataKey: "running", color: processColors.running, opacity: 0.4 },
		{ label: t`Sleeping`, dataKey: "sleeping", color: processColors.sleeping, opacity: 0.3 },
		{ label: t`Disk Sleep`, dataKey: "disk_sleep", color: processColors.disk_sleep, opacity: 0.4 },
		{ label: t`Zombie`, dataKey: "zombie", color: processColors.zombie, opacity: 0.5 },
		{ label: t`Stopped`, dataKey: "stopped", color: processColors.stopped, opacity: 0.4 },
		{ label: t`Idle`, dataKey: "idle", color: processColors.idle, opacity: 0.3 },
		{ label: t`Other`, dataKey: "other", color: processColors.other, opacity: 0.3 },
	].filter(area => {
		// Only show areas that have data
		return transformedData.some(point => point[area.dataKey] > 0)
	}), [transformedData, t, processColors])

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
								const val = Math.round(value).toString()
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
										return Math.round(value).toString()
									}}
								/>
							}
						/>
						{areas.map((area, i) => {
							return (
								<Area
									key={i}
									dataKey={area.dataKey}
									name={area.label}
									type="monotoneX"
									fill={area.color}
									fillOpacity={area.opacity}
									stroke={area.color}
									isAnimationActive={false}
									stackId="processes"
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
	}, [transformedData.at(-1), yAxisWidth, areas])
})