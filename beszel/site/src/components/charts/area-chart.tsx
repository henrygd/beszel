import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"

import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	decimalString,
	chartMargin,
} from "@/lib/utils"
// import Spinner from '../spinner'
import { ChartData } from "@/types"
import { memo, useMemo } from "react"
import { t } from "@lingui/macro"
import { useLingui } from "@lingui/react"

/** [label, key, color, opacity] */
type DataKeys = [string, string, number, number]

const getNestedValue = (path: string, max = false, data: any): number | null => {
	// fallback value (obj?.stats?.cpum ? 0 : null) should only come into play when viewing
	// a max value which doesn't exist, or the value was zero and omitted from the stats object.
	// so we check if cpum is present. if so, return 0 to make sure the zero value is displayed.
	// if not, return null - there is no max data so do not display anything.
	return `stats.${path}${max ? "m" : ""}`
		.split(".")
		.reduce((acc: any, key: string) => acc?.[key] ?? (data.stats?.cpum ? 0 : null), data)
}

export default memo(function AreaChartDefault({
	maxToggled = false,
	unit = " MB/s",
	chartName,
	chartData,
	max,
	tickFormatter,
}: {
	maxToggled?: boolean
	unit?: string
	chartName: string
	chartData: ChartData
	max?: number
	tickFormatter?: (value: number) => string
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { i18n } = useLingui()

	const { chartTime } = chartData

	const showMax = chartTime !== "1h" && maxToggled

	const dataKeys: DataKeys[] = useMemo(() => {
		// [label, key, color, opacity]
		if (chartName === "CPU Usage") {
			return [[t`CPU Usage`, "cpu", 1, 0.4]]
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
		return []
	}, [chartName, i18n.locale])

	// console.log('Rendered at', new Date())

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
						domain={[0, max ?? "auto"]}
						tickFormatter={(value) => {
							let val: string
							if (tickFormatter) {
								val = tickFormatter(value)
							} else {
								val = toFixedWithoutTrailingZeros(value, 2) + unit
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
