import { t } from "@lingui/core/macro"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis, ChartLegend, ChartLegendContent } from "@/components/ui/chart"
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

// Utility to convert MB/s to kbit/s, Mbit/s, or Gbit/s
function formatBitsPerSecond(mbPerSec: number): string {
	const bitsPerSec = mbPerSec * 8_000_000
	if (bitsPerSec >= 1_000_000_000) {
		return (bitsPerSec / 1_000_000_000).toFixed(2) + ' Gbit/s'
	} else if (bitsPerSec >= 1_000_000) {
		return (bitsPerSec / 1_000_000).toFixed(2) + ' Mbit/s'
	} else {
		return (bitsPerSec / 1_000).toFixed(2) + ' kbit/s'
	}
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

	const colors = dataKeys.map((key) => key.name)

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
							const rounded = Math.ceil(value)
							const val = formatBitsPerSecond(rounded)
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
								contentFormatter={({ value }: any) => <span className="flex">{formatBitsPerSecond(value)}</span>}
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