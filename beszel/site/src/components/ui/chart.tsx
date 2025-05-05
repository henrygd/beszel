import * as React from "react"
import * as RechartsPrimitive from "recharts"

import { chartTimeData, cn } from "@/lib/utils"
import { ChartData } from "@/types"

// Format: { THEME_NAME: CSS_SELECTOR }
const THEMES = { light: "", dark: ".dark" } as const

export type ChartConfig = {
	[k in string]: {
		label?: React.ReactNode
		icon?: React.ComponentType
	} & ({ color?: string; theme?: never } | { color?: never; theme: Record<keyof typeof THEMES, string> })
}

// type ChartContextProps = {
// 	config: ChartConfig
// }

// const ChartContext = React.createContext<ChartContextProps | null>(null)

// function useChart() {
// 	const context = React.useContext(ChartContext)

// 	if (!context) {
// 		throw new Error('useChart must be used within a <ChartContainer />')
// 	}

// 	return context
// }

const ChartContainer = React.forwardRef<
	HTMLDivElement,
	React.ComponentProps<"div"> & {
		// config: ChartConfig
		children: React.ComponentProps<typeof RechartsPrimitive.ResponsiveContainer>["children"]
	}
>(({ id, className, children, ...props }, ref) => {
	const uniqueId = React.useId()
	const chartId = `chart-${id || uniqueId.replace(/:/g, "")}`

	return (
		//<ChartContext.Provider value={{ config }}>
		<div
			data-chart={chartId}
			ref={ref}
			className={cn(
				"text-xs [&_.recharts-cartesian-axis-tick_text]:fill-muted-foreground [&_.recharts-cartesian-grid_line]:stroke-border/50 [&_.recharts-curve.recharts-tooltip-cursor]:stroke-border [&_.recharts-dot[stroke='#fff']]:stroke-transparent [&_.recharts-layer]:outline-none [&_.recharts-polar-grid_[stroke='#ccc']]:stroke-border [&_.recharts-radial-bar-background-sector]:fill-muted [&_.recharts-rectangle.recharts-tooltip-cursor]:fill-muted [&_.recharts-reference-line-line]:stroke-border [&_.recharts-sector[stroke='#fff']]:stroke-transparent [&_.recharts-sector]:outline-none [&_.recharts-surface]:outline-none",
				className
			)}
			{...props}
		>
			{/* <ChartStyle id={chartId} config={config} /> */}
			<RechartsPrimitive.ResponsiveContainer>{children}</RechartsPrimitive.ResponsiveContainer>
		</div>
		//</ChartContext.Provider>
	)
})
ChartContainer.displayName = "Chart"

// const ChartStyle = ({ id, config }: { id: string; config: ChartConfig }) => {
// 	const colorConfig = Object.entries(config).filter(([_, config]) => config.theme || config.color)

// 	if (!colorConfig.length) {
// 		return null
// 	}

// 	return (
// 		<style
// 			dangerouslySetInnerHTML={{
// 				__html: Object.entries(THEMES).map(
// 					([theme, prefix]) => `
// ${prefix} [data-chart=${id}] {
// ${colorConfig
// 	.map(([key, itemConfig]) => {
// 		const color = itemConfig.theme?.[theme as keyof typeof itemConfig.theme] || itemConfig.color
// 		return color ? `  --color-${key}: ${color};` : null
// 	})
// 	.join('\n')}
// }
// `
// 				),
// 			}}
// 		/>
// 	)
// }

const ChartTooltip = RechartsPrimitive.Tooltip

const ChartTooltipContent = React.forwardRef<
	HTMLDivElement,
	React.ComponentProps<typeof RechartsPrimitive.Tooltip> &
		React.ComponentProps<"div"> & {
			hideLabel?: boolean
			indicator?: "line" | "dot" | "dashed"
			nameKey?: string
			labelKey?: string
			unit?: string
			filter?: string
			contentFormatter?: (item: any, key: string) => React.ReactNode | string
			truncate?: boolean
		}
>(
	(
		{
			active,
			payload,
			className,
			indicator = "line",
			hideLabel = false,
			label,
			labelFormatter,
			labelClassName,
			formatter,
			color,
			nameKey,
			labelKey,
			unit,
			filter,
			itemSorter,
			contentFormatter: content = undefined,
			truncate = false,
		},
		ref
	) => {
		// const { config } = useChart()
		const config = {}

		React.useMemo(() => {
			if (filter) {
				payload = payload?.filter((item) => (item.name as string)?.toLowerCase().includes(filter.toLowerCase()))
			}
			if (itemSorter) {
				// @ts-ignore
				payload?.sort(itemSorter)
			}
		}, [itemSorter, payload])

		const tooltipLabel = React.useMemo(() => {
			if (hideLabel || !payload?.length) {
				return null
			}

			const [item] = payload
			const key = `${labelKey || item.name || "value"}`
			const itemConfig = getPayloadConfigFromPayload(config, item, key)
			const value = !labelKey && typeof label === "string" ? label : itemConfig?.label

			if (labelFormatter) {
				return <div className={cn("font-medium", labelClassName)}>{labelFormatter(value, payload)}</div>
			}

			if (!value) {
				return null
			}

			return <div className={cn("font-medium", labelClassName)}>{value}</div>
		}, [label, labelFormatter, payload, hideLabel, labelClassName, config, labelKey])

		if (!active || !payload?.length) {
			return null
		}

		// const nestLabel = payload.length === 1 && indicator !== 'dot'
		const nestLabel = false

		return (
			<div
				ref={ref}
				className={cn(
					"grid min-w-[7rem] items-start gap-1.5 rounded-lg border border-border/50 bg-background px-2.5 py-1.5 text-xs shadow-xl",
					className
				)}
			>
				{!nestLabel ? tooltipLabel : null}
				<div className="grid gap-1.5">
					{payload.map((item, index) => {
						const key = `${nameKey || item.name || item.dataKey || "value"}`
						const itemConfig = getPayloadConfigFromPayload(config, item, key)
						const indicatorColor = color || item.payload.fill || item.color

						return (
							<div
								key={item?.name || item.dataKey}
								className={cn(
									"flex w-full items-stretch gap-2 [&>svg]:h-2.5 [&>svg]:w-2.5 [&>svg]:text-muted-foreground",
									indicator === "dot" && "items-center"
								)}
							>
								{formatter && item?.value !== undefined && item.name ? (
									formatter(item.value, item.name, item, index, item.payload)
								) : (
									<>
										{itemConfig?.icon ? (
											<itemConfig.icon />
										) : (
											<div
												className={cn("shrink-0 rounded-[2px] border-[--color-border] bg-[--color-bg]", {
													"h-2.5 w-2.5": indicator === "dot",
													"w-1": indicator === "line",
													"w-0 border-[1.5px] border-dashed bg-transparent": indicator === "dashed",
													"my-0.5": nestLabel && indicator === "dashed",
												})}
												style={
													{
														"--color-bg": indicatorColor,
														"--color-border": indicatorColor,
													} as React.CSSProperties
												}
											/>
										)}
										<div
											className={cn(
												"flex flex-1 justify-between leading-none gap-2",
												nestLabel ? "items-end" : "items-center"
											)}
										>
											{nestLabel ? tooltipLabel : null}
											<span
												className={cn(
													"text-muted-foreground",
													truncate ? "max-w-40 truncate leading-normal -my-1" : ""
												)}
											>
												{itemConfig?.label || item.name}
											</span>
											{item.value !== undefined && (
												<span className="font-medium tabular-nums text-foreground">
													{content && typeof content === "function"
														? content(item, key)
														: item.value.toLocaleString() + (unit ? unit : "")}
												</span>
											)}
										</div>
									</>
								)}
							</div>
						)
					})}
				</div>
			</div>
		)
	}
)
ChartTooltipContent.displayName = "ChartTooltip"

const ChartLegend = RechartsPrimitive.Legend

const ChartLegendContent = React.forwardRef<
	HTMLDivElement,
	React.ComponentProps<"div"> &
		Pick<RechartsPrimitive.LegendProps, "payload" | "verticalAlign"> & {
			hideIcon?: boolean
			nameKey?: string
		}
>(({ className, payload, verticalAlign = "bottom" }, ref) => {
	// const { config } = useChart()

	if (!payload?.length) {
		return null
	}

	return (
		<div
			ref={ref}
			className={cn(
				"flex items-center justify-center gap-4 gap-y-1 flex-wrap",
				verticalAlign === "top" ? "pb-3" : "pt-3",
				className
			)}
		>
			{payload.map((item) => {
				// const key = `${nameKey || item.dataKey || 'value'}`
				// const itemConfig = getPayloadConfigFromPayload(config, item, key)

				return (
					<div
						key={item.value}
						className={cn(
							// 'flex items-center gap-1.5 [&>svg]:h-3 [&>svg]:w-3 [&>svg]:text-muted-foreground text-muted-foreground'
							"flex items-center gap-1.5 text-muted-foreground"
						)}
					>
						{/* {itemConfig?.icon && !hideIcon ? (
							<itemConfig.icon />
						) : ( */}
						<div
							className="h-2 w-2 shrink-0 rounded-[2px]"
							style={{
								backgroundColor: item.color,
							}}
						/>
						{item.value}
						{/* )} */}
						{/* {itemConfig?.label} */}
					</div>
				)
			})}
		</div>
	)
})
ChartLegendContent.displayName = "ChartLegend"

// Helper to extract item config from a payload.
function getPayloadConfigFromPayload(config: ChartConfig, payload: unknown, key: string) {
	if (typeof payload !== "object" || payload === null) {
		return undefined
	}

	const payloadPayload =
		"payload" in payload && typeof payload.payload === "object" && payload.payload !== null
			? payload.payload
			: undefined

	let configLabelKey: string = key

	if (key in payload && typeof payload[key as keyof typeof payload] === "string") {
		configLabelKey = payload[key as keyof typeof payload] as string
	} else if (
		payloadPayload &&
		key in payloadPayload &&
		typeof payloadPayload[key as keyof typeof payloadPayload] === "string"
	) {
		configLabelKey = payloadPayload[key as keyof typeof payloadPayload] as string
	}

	return configLabelKey in config ? config[configLabelKey] : config[key as keyof typeof config]
}

let cachedAxis: JSX.Element
const xAxis = function ({ domain, ticks, chartTime }: ChartData) {
	if (cachedAxis && domain[0] === cachedAxis.props.domain[0]) {
		return cachedAxis
	}
	cachedAxis = (
		<RechartsPrimitive.XAxis
			dataKey="created"
			domain={domain}
			ticks={ticks}
			allowDataOverflow
			type="number"
			scale="time"
			minTickGap={12}
			tickMargin={8}
			axisLine={false}
			tickFormatter={chartTimeData[chartTime].format}
		/>
	)
	return cachedAxis
}

export {
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
	ChartLegend,
	ChartLegendContent,
	xAxis,
	// ChartStyle,
}
