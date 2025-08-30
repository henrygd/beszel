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
import { ChartData, SystemStatsRecord } from "@/types"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"

export type DataPoint = {
	label: string
	dataKey: (data: SystemStatsRecord) => number | undefined
	color: number | string
	opacity: number
}

/** [label, key, color, opacity] */
type DataKeys = [string, string, number, number]

const getNestedValue = (path: string, max = false, data: any): number | null => {
	// fallback value (obj?.stats?.cpum ? 0 : null) should only come into play when viewing
	// a max value which doesn't exist, or the value was zero and omitted from the stats object.
	// so we check if cpum is present. if so, return 0 to make sure the zero value is displayed.
	// if not, return null - there is no max data so do not display anything.
	const value = `stats.${path}${max ? "m" : ""}`
		.split(".")
		.reduce((acc: any, key: string) => acc?.[key] ?? (data.stats?.cpum ? 0 : null), data)
	
	// For CPU metrics, return 0 if the value is undefined or null
	if (path.startsWith('cpu') && (value === null || value === undefined)) {
		return 0
	}
	
	return value
}

type AreaChartDefaultProps = {
	maxToggled?: boolean
	unit?: string
	chartName?: string
	chartData: ChartData
	max?: number
	tickFormatter?: (value: number, index?: number) => string
	contentFormatter?: ((value: number) => string) | (({ value, payload }: { value: number; payload: SystemStatsRecord }) => string)
	showLegend?: boolean
	dataPoints?: DataPoint[]
	domain?: [number, number]
}

export default memo(function AreaChartDefault({
	chartData,
	max,
	maxToggled,
	tickFormatter,
	contentFormatter,
	showLegend = true,
	chartName,
	unit,
	dataPoints,
	domain,
}: AreaChartDefaultProps) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { t, i18n } = useLingui()

	const { chartTime } = chartData
	const showMax = chartTime !== "1h" && maxToggled

	const dataKeys: DataKeys[] = useMemo(() => {
		// Only use legacy chart name logic if dataPoints are not provided
		if (!dataPoints && chartName) {
			// [label, key, color, opacity]
			if (chartName === "CPU Usage") {
				return [
					[t`User`, "cpuu", 1, 0.4],
					[t`System`, "cpus", 2, 0.4],
					[t`I/O Wait`, "cpui", 3, 0.4],
					[t`Steal`, "cpusl", 4, 0.4],
				]
			} else if (chartName === "dio") {
				return [
					[t({ message: "Write", comment: "Disk write" }), "dw", 3, 0.3],
					[t({ message: "Read", comment: "Disk read" }), "dr", 1, 0.3],
				]
			} else if (chartName === "bw") {
				return [
					[t({ message: "Sent", comment: "Network bytes sent (upload)" }), "ns", 5, 0.2],
					[t({ message: "Received", comment: "Network bytes received (download)" }), "nr", 2, 0.2],
				]
			} else if (chartName.startsWith("efs")) {
				return [
					[t`Write`, `${chartName}.w`, 3, 0.3],
					[t`Read`, `${chartName}.r`, 1, 0.3],
				]
			} else if (chartName.startsWith("g.")) {
				return [chartName.includes("mu") ? [t`Used`, chartName, 2, 0.25] : [t`Usage`, chartName, 1, 0.4]]
			}
		}
		return []
	}, [chartName, dataPoints, i18n.locale, t])

	return useMemo(() => {
		if (chartData.systemStats.length === 0) {
			return null
		}

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
							domain={domain ?? [0, max ?? "auto"]}
							tickFormatter={(value, index) => {
								let val: string
								if (tickFormatter) {
									val = tickFormatter(value, index)
								} else {
									val = toFixedFloat(value, 2) + (unit || "")
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
									contentFormatter={contentFormatter ? contentFormatter : ({ value }) => {
										return decimalString(value) + (unit || "")
									}}
								/>
							}
						/>
						{/* Use dataPoints if provided, otherwise use legacy dataKeys */}
						{dataPoints ? 
							dataPoints.map((dataPoint, i) => {
								const color = `var(--chart-${dataPoint.color})`
								return (
									<Area
										key={i}
										dataKey={dataPoint.dataKey}
										name={dataPoint.label}
										type="monotoneX"
										fill={color}
										fillOpacity={dataPoint.opacity}
										stroke={color}
										isAnimationActive={false}
									/>
								)
							})
							:
							dataKeys.map((key, i) => {
								const color = `var(--chart-${key[2]})`
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
										stackId={chartName === "CPU Usage" ? "a" : undefined}
									/>
								)
							})
						}
						{showLegend && (
							(dataPoints && dataPoints.length > 1) || 
							(chartName === "CPU Usage" || chartName === "dio" || chartName?.startsWith("efs"))
						) && <ChartLegend content={<ChartLegendContent />} />}
					</AreaChart>
				</ChartContainer>
			</div>
		)
	}, [chartData.systemStats.at(-1), yAxisWidth, maxToggled, dataKeys, dataPoints])
})
