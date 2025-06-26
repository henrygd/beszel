import { t } from "@lingui/core/macro"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, decimalString, chartMargin } from "@/lib/utils"
import { ChartData } from "@/types"
import { Separator } from "@/components/ui/separator"
import { useStore } from "@nanostores/react"
import { $networkInterfaceFilter } from "@/lib/stores"

const getNestedValue = (path: string, max = false, data: any): number | null => {
	return `stats.ni.${path}${max ? "m" : ""}`
		.split(".")
		.reduce((acc: any, key: string) => acc?.[key] ?? (data.stats?.cpum ? 0 : null), data)
}

export default memo(function NetworkInterfaceChart({
	chartData,
	maxToggled = false,
}: {
	chartData: ChartData
	maxToggled?: boolean
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { i18n } = useLingui()
	const networkInterfaceFilter = useStore($networkInterfaceFilter)

	const { chartTime } = chartData
	const showMax = chartTime !== "1h" && maxToggled

	// Get network interface names from the latest stats
	const networkInterfaces = useMemo(() => {
		if (chartData.systemStats.length === 0) return []
		const latestStats = chartData.systemStats[chartData.systemStats.length - 1]
		const allInterfaces = Object.keys(latestStats.stats.ni || {})
		
		// Filter interfaces based on filter value
		if (networkInterfaceFilter) {
			return allInterfaces.filter(iface => 
				iface.toLowerCase().includes(networkInterfaceFilter.toLowerCase())
			)
		}
		
		return allInterfaces
	}, [chartData.systemStats, networkInterfaceFilter])

	const dataKeys = useMemo(() => {
		// Colors that work well in both light and dark themes (excluding black/white)
		const colors = [1, 2, 3, 4, 5, 6, 7, 8] // Skip problematic colors
		
		return networkInterfaces.map((iface, index) => [
			`${iface} Sent`,
			`${iface}.ns`,
			colors[index % colors.length], // Use safe colors
			0.2,
		]).concat(
			networkInterfaces.map((iface, index) => [
				`${iface} Received`,
				`${iface}.nr`,
				colors[(index + 4) % colors.length], // Offset for received lines
				0.2,
			])
		)
	}, [networkInterfaces, i18n.locale])

	if (chartData.systemStats.length === 0 || networkInterfaces.length === 0) {
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
						tickFormatter={(value) => {
							const val = decimalString(value, 2) + " MB/s"
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
								labelFormatter={(_: any, data: any) => formatShortDate(data[0].payload.created)}
								contentFormatter={({ value, name, payload }: any) => {
									const ifaceName = (name as string)?.replace(" Sent", "").replace(" Received", "")
									const isReceived = (name as string)?.includes("Received")
									const sent = payload?.stats?.ni?.[ifaceName]?.ns ?? 0
									const received = payload?.stats?.ni?.[ifaceName]?.nr ?? 0
									
									if (isReceived) {
										return (
											<span className="flex">
												{decimalString(received)} MB/s
												<span className="opacity-70 ms-0.5"> </span>
											</span>
										)
									} else {
										return (
											<span className="flex">
												{decimalString(sent)} MB/s
												<span className="opacity-70 ms-0.5"> </span>
											</span>
										)
									}
								}}
							/>
						}
					/>
					{dataKeys.map((key, i) => {
						const color = `hsl(var(--chart-${key[2]}))`
						return (
							<Area
								key={i}
								dataKey={getNestedValue.bind(null, key[1] as string, showMax)}
								name={key[0] as string}
								type="monotoneX"
								fill={color}
								fillOpacity={key[3] as number}
								stroke={color}
								isAnimationActive={false}
							/>
						)
					})}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}) 