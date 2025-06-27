import { t } from "@lingui/core/macro"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Line, LineChart, CartesianGrid, YAxis } from "recharts"
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
		// Generate colors for each interface - each interface gets a unique hue
		// and sent/received use different shades of that hue
		const interfaceColors = networkInterfaces.map((iface, index) => {
			const hue = ((index * 360) / Math.max(networkInterfaces.length, 1)) % 360
			return {
				interface: iface,
				sentColor: `hsl(${hue}, 70%, 45%)`, // Darker shade for sent
				receivedColor: `hsl(${hue}, 70%, 65%)`, // Lighter shade for received
			}
		})
		
		return interfaceColors.flatMap(({ interface: iface, sentColor, receivedColor }) => [
			{
				name: `${iface} Sent`,
				dataKey: `${iface}.ns`,
				color: sentColor,
				type: 'sent' as const,
				interface: iface,
			},
			{
				name: `${iface} Received`,
				dataKey: `${iface}.nr`,
				color: receivedColor,
				type: 'received' as const,
				interface: iface,
			}
		])
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
				<LineChart accessibilityLayer data={chartData.systemStats} margin={chartMargin}>
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
								contentFormatter={({ value, name }: any) => {
									const isReceived = (name as string)?.includes("Received")
									return (
										<span className="flex">
											{decimalString(value)} MB/s
											<span className="opacity-70 ms-0.5">
												{isReceived ? " rx" : " tx"}
											</span>
										</span>
									)
								}}
							/>
						}
					/>
					{dataKeys.map((key, i) => {
						const filtered = networkInterfaceFilter && !key.interface.toLowerCase().includes(networkInterfaceFilter.toLowerCase())
						return (
							<Line
								key={i}
								dataKey={getNestedValue.bind(null, key.dataKey, showMax)}
								name={key.name}
								type="monotoneX"
								stroke={key.color}
								strokeWidth={1.5}
								dot={false}
								activeDot={{ r: 3, strokeWidth: 1 }}
								isAnimationActive={false}
								strokeOpacity={filtered ? 0.3 : 1}
							/>
						)
					})}
				</LineChart>
			</ChartContainer>
		</div>
	)
}) 