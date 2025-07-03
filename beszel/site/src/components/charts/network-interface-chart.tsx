import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis, ChartLegend, ChartLegendContent } from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, chartMargin, formatSpeed } from "@/lib/utils"
import { ChartData } from "@/types"
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
	max,
}: {
	chartData: ChartData
	maxToggled?: boolean
	max?: number
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

	const colors = dataKeys.map((key) => key.name)

	// Calculate the maximum value from all network interface data and round it up
	const calculatedMax = useMemo(() => {
		if (chartData.systemStats.length === 0) return undefined
		
		let maxValue = 0
		
		// Find the maximum value across all network interfaces and all data points
		for (const stats of chartData.systemStats) {
			if (stats.stats?.ni) {
				for (const iface of Object.values(stats.stats.ni)) {
					const sent = showMax ? (iface.nsm ?? iface.ns ?? 0) : (iface.ns ?? 0)
					const received = showMax ? (iface.nrm ?? iface.nr ?? 0) : (iface.nr ?? 0)
					maxValue = Math.max(maxValue, sent, received)
				}
			}
		}
		
		if (maxValue === 0) return undefined
		
		// Round up to a nice value based on magnitude
		// Convert to bits per second for easier rounding
		const bitsPerSec = maxValue * 8_000_000
		
		let roundedBitsPerSec: number
		if (bitsPerSec >= 1_000_000_000) {
			// For Gbit/s range, round to nearest 0.1 Gbit/s
			roundedBitsPerSec = Math.ceil(bitsPerSec / 100_000_000) * 100_000_000
		} else if (bitsPerSec >= 1_000_000) {
			// For Mbit/s range, round to nearest 1 Mbit/s
			roundedBitsPerSec = Math.ceil(bitsPerSec / 1_000_000) * 1_000_000
		} else {
			// For kbit/s range, round to nearest 100 kbit/s
			roundedBitsPerSec = Math.ceil(bitsPerSec / 100_000) * 100_000
		}
		
		// Convert back to MB/s for the domain
		return roundedBitsPerSec / 8_000_000
	}, [chartData.systemStats, showMax])

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
					//	domain={[0, calculatedMax ?? "auto"]}
						tickFormatter={(value) => updateYAxisWidth((value * 8).toFixed(2) + " Mbit/s")}
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
								contentFormatter={({ value }: any) => <span className="flex">{formatSpeed(value)}</span>}
							/>
						}
					/>
					{dataKeys.map((key, i) => {
						const filtered = networkInterfaceFilter && !key.interface.toLowerCase().includes(networkInterfaceFilter.toLowerCase())
						let fillOpacity = filtered ? 0.05 : 0.4
						let strokeOpacity = filtered ? 0.1 : 1
						return (
							<Area
								key={i}
								dataKey={getNestedValue.bind(null, key.dataKey, showMax)}
								name={key.name}
								type="monotoneX"
								fill={key.color}
								fillOpacity={fillOpacity}
								stroke={key.color}
								strokeOpacity={strokeOpacity}
								activeDot={{ opacity: filtered ? 0 : 1 }}
								isAnimationActive={false}
							/>
						)
					})}
					{colors.length < 12 && <ChartLegend content={<ChartLegendContent />} />}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}) 