import { type ReactNode, useEffect, useMemo, useState } from "react"
import { CartesianGrid, Line, LineChart, YAxis } from "recharts"
import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
	xAxis,
} from "@/components/ui/chart"
import { chartMargin, cn, formatShortDate } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { useYAxisWidth } from "./hooks"
import type { AxisDomain } from "recharts/types/util/types"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"

export type DataPoint<T = SystemStatsRecord> = {
	label: string
	dataKey: (data: T) => number | null | undefined
	color: number | string
	stackId?: string | number
	order?: number
	strokeOpacity?: number
	activeDot?: boolean
	dot?: boolean
	/** Which Y axis this series plots against. Defaults to "left". */
	yAxisId?: "left" | "right"
	strokeDasharray?: string
}

export default function LineChartDefault({
	chartData,
	customData,
	max,
	maxToggled,
	tickFormatter,
	tickFormatter2,
	contentFormatter,
	dataPoints,
	domain,
	domain2,
	max2,
	legend,
	itemSorter,
	showTotal = false,
	reverseStackOrder = false,
	hideYAxis = false,
	filter,
	truncate = false,
	chartProps,
	connectNulls,
}: {
	chartData: ChartData
	// biome-ignore lint/suspicious/noExplicitAny: accepts different data source types (systemStats or containerData)
	customData?: any[]
	max?: number
	max2?: number
	maxToggled?: boolean
	tickFormatter: (value: number, index: number) => string
	/** Tick formatter for the right ("right"-yAxisId) axis, when any dataPoint uses it. */
	tickFormatter2?: (value: number, index: number) => string
	// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item interop
	contentFormatter: (item: any, key: string) => ReactNode
	// biome-ignore lint/suspicious/noExplicitAny: accepts DataPoint with different generic types
	dataPoints?: DataPoint<any>[]
	domain?: AxisDomain
	/** Domain for the right axis, when any dataPoint uses it. */
	domain2?: AxisDomain
	legend?: boolean
	showTotal?: boolean
	// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item interop
	itemSorter?: (a: any, b: any) => number
	reverseStackOrder?: boolean
	hideYAxis?: boolean
	filter?: string
	truncate?: boolean
	chartProps?: Omit<React.ComponentProps<typeof LineChart>, "data" | "margin">
	connectNulls?: boolean
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const hasRightAxis = !!dataPoints?.some((dp) => dp.yAxisId === "right")
	// fixed width for the secondary axis rather than measured, since its labels (e.g. loss %) are short
	// and predictable, and this avoids depending on a second async width-measurement pass to settle
	const rightAxisWidth = 38
	const { isIntersecting, ref } = useIntersectionObserver({ freeze: false })
	const sourceData = customData ?? chartData.systemStats ?? []
	const [displayData, setDisplayData] = useState(sourceData)
	const [displayMaxToggled, setDisplayMaxToggled] = useState(maxToggled)

	// Reduce chart redraws by only updating while visible or when chart time changes
	useEffect(() => {
		const shouldPrimeData = sourceData.length && !displayData.length
		const sourceChanged = sourceData !== displayData
		const shouldUpdate = shouldPrimeData || (sourceChanged && isIntersecting)
		if (shouldUpdate) {
			setDisplayData(sourceData)
		}
		if (isIntersecting && maxToggled !== displayMaxToggled) {
			setDisplayMaxToggled(maxToggled)
		}
	}, [displayData, displayMaxToggled, isIntersecting, maxToggled, sourceData])

	// Use a stable key derived from data point identities and visual properties
	const linesKey = dataPoints?.map((d) => `${d.label}:${d.strokeOpacity}${d.dot}${d.yAxisId}${d.strokeDasharray}`).join("\0")

	const XAxis = xAxis(chartData.chartTime, displayData.at(-1)?.created)

	const Lines = useMemo(() => {
		return dataPoints?.map((dataPoint, i) => {
			let { color } = dataPoint
			if (typeof color === "number") {
				color = `var(--chart-${color})`
			}
			return (
				<Line
					key={dataPoint.label}
					yAxisId={dataPoint.yAxisId ?? "left"}
					dataKey={dataPoint.dataKey}
					name={dataPoint.label}
					type="monotoneX"
					dot={dataPoint.dot || false}
					strokeWidth={1.5}
					stroke={color}
					strokeOpacity={dataPoint.strokeOpacity}
					strokeDasharray={dataPoint.strokeDasharray}
					isAnimationActive={false}
					// stackId={dataPoint.stackId}
					order={dataPoint.order || i}
					activeDot={dataPoint.activeDot ?? true}
					connectNulls={connectNulls}
				/>
			)
		})
	}, [linesKey, displayMaxToggled])

	return useMemo(() => {
		if (displayData.length === 0) {
			return null
		}
		// if (logRender) {
		// console.log("Rendered", dataPoints?.map((d) => d.label).join(", "), new Date())
		// }
		return (
			<ChartContainer
				ref={ref}
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth || hideYAxis,
					"ps-4": hideYAxis,
				})}
			>
				<LineChart
					reverseStackOrder={reverseStackOrder}
					accessibilityLayer
					data={displayData}
					margin={hideYAxis ? { ...chartMargin, left: 5 } : chartMargin}
					{...chartProps}
				>
					<CartesianGrid vertical={false} />
					{!hideYAxis && (
						<YAxis
							yAxisId="left"
							direction="ltr"
							orientation={chartData.orientation}
							className="tracking-tighter"
							width={yAxisWidth}
							domain={domain ?? [0, max ?? "auto"]}
							tickFormatter={(value, index) => updateYAxisWidth(tickFormatter(value, index))}
							tickLine={false}
							axisLine={false}
						/>
					)}
					{!hideYAxis && hasRightAxis && (
						<YAxis
							yAxisId="right"
							direction="ltr"
							orientation={chartData.orientation === "left" ? "right" : "left"}
							className="tracking-tighter"
							width={rightAxisWidth}
							domain={domain2 ?? [0, max2 ?? "auto"]}
							tickFormatter={tickFormatter2 ?? tickFormatter}
							tickLine={false}
							axisLine={false}
						/>
					)}
					{XAxis}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// @ts-expect-error
						itemSorter={itemSorter}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={contentFormatter}
								showTotal={showTotal}
								filter={filter}
								truncate={truncate}
							/>
						}
					/>
					{Lines}
					{legend && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		)
	}, [displayData, yAxisWidth, hasRightAxis, filter, Lines, XAxis])
}
