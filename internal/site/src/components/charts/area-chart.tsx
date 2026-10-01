import { type ReactNode, useEffect, useMemo, useState } from "react"
import { Area, AreaChart, CartesianGrid, ReferenceLine, YAxis } from "recharts"
import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
	fixedDomainTicks,
	xAxis,
} from "@/components/ui/chart"
import { chartMargin, cn, formatShortDate } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { useYAxisWidth } from "./hooks"
import type { AxisDomain } from "recharts/types/util/types"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"

export type DataPoint<T = SystemStatsRecord> = {
	/** Unique key when several data points share a label, e.g. segments of one line */
	id?: string
	label: string
	dataKey: (data: T) => number | null | undefined
	color: number | string
	opacity: number
	stackId?: string | number
	order?: number
	strokeOpacity?: number
	activeDot?: boolean
	/** Draws a dot on each value, so points without neighbors (e.g. between gaps) are visible. */
	dot?: boolean
	/** Set to false to leave the data point out of the legend, e.g. later segments of one line */
	legend?: boolean
}

export default function AreaChartDefault({
	chartData,
	customData,
	max,
	maxToggled,
	tickFormatter,
	contentFormatter,
	dataPoints,
	domain,
	legend,
	itemSorter,
	showTotal = false,
	reverseStackOrder = false,
	hideYAxis = false,
	filter,
	truncate = false,
	chartProps,
	connectNulls,
	markers,
	tooltipNote,
}: {
	chartData: ChartData
	// biome-ignore lint/suspicious/noExplicitAny: accepts different data source types (systemStats or containerData)
	customData?: any[]
	max?: number
	maxToggled?: boolean
	tickFormatter: (value: number, index: number) => string
	// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item interop
	contentFormatter: (item: any, key: string) => ReactNode
	// biome-ignore lint/suspicious/noExplicitAny: accepts DataPoint with different generic types
	dataPoints?: DataPoint<any>[]
	domain?: AxisDomain
	legend?: boolean
	showTotal?: boolean
	// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item interop
	itemSorter?: (a: any, b: any) => number
	reverseStackOrder?: boolean
	hideYAxis?: boolean
	filter?: string
	truncate?: boolean
	chartProps?: Omit<React.ComponentProps<typeof AreaChart>, "data" | "margin">
	connectNulls?: boolean
	/** Times in Unix ms marked with a dashed line in the destructive color, e.g. failed runs */
	markers?: number[]
	/** Extra tooltip line for the hovered row */
	// biome-ignore lint/suspicious/noExplicitAny: row type depends on the chart's data
	tooltipNote?: (row: any) => ReactNode
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
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
	const areasKey = dataPoints?.map((d) => `${d.id ?? d.label}:${d.opacity}${d.dot}`).join("\0")

	const Areas = useMemo(() => {
		return dataPoints?.map((dataPoint, i) => {
			let { color } = dataPoint
			if (typeof color === "number") {
				color = `var(--chart-${color})`
			}
			return (
				<Area
					key={dataPoint.id ?? dataPoint.label}
					legendType={dataPoint.legend === false ? "none" : undefined}
					dataKey={dataPoint.dataKey}
					name={dataPoint.label}
					type="monotoneX"
					fill={color}
					fillOpacity={dataPoint.opacity}
					stroke={color}
					strokeOpacity={dataPoint.strokeOpacity}
					isAnimationActive={false}
					stackId={dataPoint.stackId}
					order={dataPoint.order || i}
					activeDot={dataPoint.activeDot ?? true}
					dot={dataPoint.dot || false}
					connectNulls={connectNulls}
				/>
			)
		})
	}, [areasKey, displayMaxToggled, connectNulls])

	const XAxis = xAxis(chartData.chartTime, displayData.at(-1)?.created)

	// Without any values recharts draws no y-axis ticks, so the axis width is never measured and
	// the chart would stay hidden. Hide the axis instead, e.g. when every speedtest run failed.
	const hasValues = useMemo(
		() => !dataPoints || displayData.some((row) => dataPoints.some((point) => typeof point.dataKey(row) === "number")),
		[displayData, areasKey]
	)
	const noYAxis = hideYAxis || !hasValues

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
					"opacity-100": yAxisWidth || noYAxis,
					"ps-4": noYAxis,
				})}
			>
				<AreaChart
					reverseStackOrder={reverseStackOrder}
					accessibilityLayer
					data={displayData}
					margin={noYAxis ? { ...chartMargin, left: 5 } : chartMargin}
					{...chartProps}
				>
					<CartesianGrid vertical={false} />
					{!noYAxis && (
						<YAxis
							direction="ltr"
							orientation={chartData.orientation}
							className="tracking-tighter"
							width={yAxisWidth}
							domain={domain ?? [0, max ?? "auto"]}
							ticks={fixedDomainTicks(domain ?? [0, max ?? "auto"])}
							tickFormatter={(value, index) => updateYAxisWidth(tickFormatter(value, index))}
							tickLine={false}
							axisLine={false}
						/>
					)}
					{XAxis}
					{markers?.map((time) => (
						<ReferenceLine
							key={time}
							x={time}
							// inline, since ChartContainer styles reference lines with the border color
							style={{ stroke: "var(--destructive)" }}
							strokeDasharray="3 3"
							// a marker can fall just outside the visible time span
							ifOverflow="hidden"
						/>
					))}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// Keep rows without values, e.g. a failed run, so their note can show.
						// ChartTooltipContent drops the empty values itself.
						filterNull={!tooltipNote}
						// @ts-expect-error
						itemSorter={itemSorter}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={contentFormatter}
								showTotal={showTotal}
								filter={filter}
								truncate={truncate}
								note={tooltipNote}
							/>
						}
					/>
					{Areas}
					{legend && <ChartLegend content={<ChartLegendContent />} />}
				</AreaChart>
			</ChartContainer>
		)
	}, [displayData, yAxisWidth, filter, Areas, XAxis, markers, tooltipNote, noYAxis])
}
