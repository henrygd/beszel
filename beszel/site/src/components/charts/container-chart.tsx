import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartConfig, ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { memo, useMemo } from "react"
import { useYAxisWidth, cn, formatShortDate, chartMargin, toFixedFloat, formatBytes, decimalString } from "@/lib/utils"
// import Spinner from '../spinner'
import { useStore } from "@nanostores/react"
import { $containerFilter, $userSettings } from "@/lib/stores"
import { ChartData } from "@/types"
import { Separator } from "../ui/separator"
import { ChartType, Unit } from "@/lib/enums"

export default memo(function ContainerChart({
	dataKey,
	chartData,
	chartType,
	unit = "%",
}: {
	dataKey: string
	chartData: ChartData
	chartType: ChartType
	unit?: string
}) {
	const filter = useStore($containerFilter)
	const userSettings = useStore($userSettings)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	const { containerData } = chartData

	const isNetChart = chartType === ChartType.Network

	const chartConfig = useMemo(() => {
		const config = {} as Record<string, { label: string; color: string }>
		const totalUsage = new Map<string, number>()

		// calculate total usage of each container
		for (const stats of containerData) {
			for (const key in stats) {
				if (!key || key === "created") continue

				const currentTotal = totalUsage.get(key) ?? 0
				const increment = isNetChart
					? (stats[key]?.nr ?? 0) + (stats[key]?.ns ?? 0)
					: // @ts-ignore
					  stats[key]?.[dataKey] ?? 0

				totalUsage.set(key, currentTotal + increment)
			}
		}

		// Sort keys and generate colors based on usage
		const sortedEntries = Array.from(totalUsage.entries()).sort(([, a], [, b]) => b - a)

		const length = sortedEntries.length
		sortedEntries.forEach(([key], i) => {
			const hue = ((i * 360) / length) % 360
			config[key] = {
				label: key,
				color: `hsl(${hue}, 60%, 55%)`,
			}
		})

		return config satisfies ChartConfig
	}, [chartData])

	const { toolTipFormatter, dataFunction, tickFormatter } = useMemo(() => {
		const obj = {} as {
			toolTipFormatter: (item: any, key: string) => React.ReactNode | string
			dataFunction: (key: string, data: any) => number | null
			tickFormatter: (value: any) => string
		}
		// tick formatter
		if (chartType === ChartType.CPU) {
			obj.tickFormatter = (value) => {
				const val = toFixedFloat(value, 2) + unit
				return updateYAxisWidth(val)
			}
		} else {
			const chartUnit = isNetChart ? userSettings.unitNet : Unit.Bytes
			obj.tickFormatter = (val) => {
				const { value, unit } = formatBytes(val, isNetChart, chartUnit, true)
				return updateYAxisWidth(toFixedFloat(value, value >= 10 ? 0 : 1) + " " + unit)
			}
		}
		// tooltip formatter
		if (isNetChart) {
			obj.toolTipFormatter = (item: any, key: string) => {
				try {
					const sent = item?.payload?.[key]?.ns ?? 0
					const received = item?.payload?.[key]?.nr ?? 0
					const { value: receivedValue, unit: receivedUnit } = formatBytes(received, true, userSettings.unitNet, true)
					const { value: sentValue, unit: sentUnit } = formatBytes(sent, true, userSettings.unitNet, true)
					return (
						<span className="flex">
							{decimalString(receivedValue)} {receivedUnit}
							<span className="opacity-70 ms-0.5"> rx </span>
							<Separator orientation="vertical" className="h-3 mx-1.5 bg-primary/40" />
							{decimalString(sentValue)} {sentUnit}
							<span className="opacity-70 ms-0.5"> tx</span>
						</span>
					)
				} catch (e) {
					return null
				}
			}
		} else if (chartType === ChartType.Memory) {
			obj.toolTipFormatter = (item: any) => {
				const { value, unit } = formatBytes(item.value, false, Unit.Bytes, true)
				return decimalString(value) + " " + unit
			}
		} else {
			obj.toolTipFormatter = (item: any) => decimalString(item.value) + unit
		}
		// data function
		if (isNetChart) {
			obj.dataFunction = (key: string, data: any) => (data[key] ? data[key].nr + data[key].ns : null)
		} else {
			obj.dataFunction = (key: string, data: any) => data[key]?.[dataKey] ?? null
		}
		return obj
	}, [])

	const filterLower = filter?.toLowerCase()

	// console.log('rendered at', new Date())

	if (containerData.length === 0) {
		return null
	}

	return (
		<div>
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart
					accessibilityLayer
					// syncId={'cpu'}
					data={containerData}
					margin={chartMargin}
					reverseStackOrder={true}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						width={yAxisWidth}
						tickFormatter={tickFormatter}
						tickLine={false}
						axisLine={false}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						truncate={true}
						labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
						// @ts-ignore
						itemSorter={(a, b) => b.value - a.value}
						content={<ChartTooltipContent filter={filter} contentFormatter={toolTipFormatter} />}
					/>
					{Object.keys(chartConfig).map((key) => {
						const filtered = filterLower && !key.toLowerCase().includes(filterLower)
						let fillOpacity = filtered ? 0.05 : 0.4
						let strokeOpacity = filtered ? 0.1 : 1
						return (
							<Area
								key={key}
								isAnimationActive={false}
								dataKey={dataFunction.bind(null, key)}
								name={key}
								type="monotoneX"
								fill={chartConfig[key].color}
								fillOpacity={fillOpacity}
								stroke={chartConfig[key].color}
								strokeOpacity={strokeOpacity}
								activeDot={{ opacity: filtered ? 0 : 1 }}
								stackId="a"
							/>
						)
					})}
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
