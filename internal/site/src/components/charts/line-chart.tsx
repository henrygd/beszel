import { type ReactNode, useEffect, useMemo, useState } from "react"
import { CartesianGrid, Line, LineChart, ReferenceLine, YAxis } from "recharts"
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
import type { ChartOptions, SystemStatsRecord } from "@/types"
import { hasChartValues, useYAxisWidth } from "./hooks"
import { ChartNoValues } from "./chart-no-values"
import type { AxisDomain } from "recharts/types/util/types"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"

export type DataPoint<T = SystemStatsRecord> = {
	/** Unique key when several data points share a label, e.g. segments of one line */
	id?: string
	label: string
	dataKey: (data: T) => number | null | undefined
	color: number | string
	stackId?: string | number
	order?: number
	strokeOpacity?: number
	activeDot?: boolean
	dot?: boolean | typeof isolatedDot
	/** Which Y axis this series plots against. Defaults to "left". */
	yAxisId?: "left" | "right"
	strokeDasharray?: string
	/** Set to false to leave the data point out of the legend, e.g. later segments of one line */
	legend?: boolean
}

type IsolatedDotProps = {
	key: string
	cx: number
	cy: number
	stroke: string
	index: number
	points: { value: unknown }[]
}

const hasValue = (point?: { value: unknown }) => typeof point?.value === "number"

/**
 * Dot renderer that only draws points with no value on either side. Without connectNulls
 * those points have no line segment, so they would otherwise only be visible on hover.
 */
export function isolatedDot({ key, cx, cy, stroke, index, points }: IsolatedDotProps) {
	if (!hasValue(points[index]) || hasValue(points[index - 1]) || hasValue(points[index + 1])) {
		return <g key={key} />
	}
	return <circle key={key} cx={cx} cy={cy} r={2} fill={stroke} />
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
	markers,
	tooltipNote,
}: {
	chartData: ChartOptions & { systemStats?: SystemStatsRecord[] }
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
	/** Times in Unix ms marked with a dashed line in the destructive color, e.g. failed runs */
	markers?: number[]
	/** Extra tooltip line for the hovered row */
	// biome-ignore lint/suspicious/noExplicitAny: row type depends on the chart's data
	tooltipNote?: (row: any) => ReactNode
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
	const linesKey = dataPoints
		?.map((d) => `${d.id ?? d.label}:${d.strokeOpacity}${d.dot}${d.yAxisId}${d.strokeDasharray}`)
		.join("\0")

	const XAxis = xAxis(chartData.chartTime, displayData.at(-1)?.created)

	// Without any values an "auto" domain has no ticks, so the axis width is never measured and
	// the chart would stay hidden. Fall back to a fixed domain, e.g. when a speedtest server
	// doesn't report packet loss or every run failed.
	const hasValues = useMemo(
		() => !dataPoints || displayData.some((row) => dataPoints.some((point) => typeof point.dataKey(row) === "number")),
		[displayData, linesKey]
	)
	const leftDomain: AxisDomain = hasValues ? (domain ?? [0, max ?? "auto"]) : [0, 100]

	const Lines = useMemo(() => {
		return dataPoints?.map((dataPoint, i) => {
			let { color } = dataPoint
			if (typeof color === "number") {
				color = `var(--chart-${color})`
			}
			return (
				<Line
					key={dataPoint.id ?? dataPoint.label}
					legendType={dataPoint.legend === false ? "none" : undefined}
					yAxisId={dataPoint.yAxisId ?? "left"}
					dataKey={dataPoint.dataKey}
					name={dataPoint.label}
					type="monotoneX"
					// recharts' default dots are white with a colored outline; fill them like isolatedDot
					dot={
						dataPoint.dot === true
							? { r: 2, fill: color, stroke: color }
							: (dataPoint.dot ?? (connectNulls ? false : isolatedDot))
					}
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
		if (!hasChartValues(displayData, dataPoints)) {
			return <ChartNoValues ref={ref} />
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
							domain={leftDomain}
							ticks={fixedDomainTicks(leftDomain)}
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
							ticks={fixedDomainTicks(domain2 ?? [0, max2 ?? "auto"])}
							tickFormatter={tickFormatter2 ?? tickFormatter}
							tickLine={false}
							axisLine={false}
						/>
					)}
					{XAxis}
					{markers?.map((time) => (
						<ReferenceLine
							key={time}
							x={time}
							yAxisId="left"
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
					{Lines}
					{legend && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		)
	}, [displayData, yAxisWidth, hasRightAxis, filter, Lines, XAxis, markers, tooltipNote, hasValues])
}
