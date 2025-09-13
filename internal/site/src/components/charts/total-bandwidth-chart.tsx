import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, ChartLegend, ChartLegendContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, chartMargin, formatBytes, toFixedFloat, decimalString } from "@/lib/utils"
import { ChartData } from "@/types"
import { useStore } from "@nanostores/react"
import { $userSettings } from "@/lib/stores"
import { Unit } from "@/lib/enums"

const getPerInterfaceBandwidth = (data: any): Record<string, { sent: number; recv: number }> | null => {
	const networkInterfaces = data?.stats?.ni
	if (!networkInterfaces) {
		return null
	}
	
	const interfaceData: Record<string, { sent: number; recv: number }> = {}
	let hasData = false
	
	Object.entries(networkInterfaces).forEach(([name, iface]: [string, any]) => {
		if (iface.tbs || iface.tbr) {
			interfaceData[name] = {
				sent: iface.tbs || 0,
				recv: iface.tbr || 0
			}
			hasData = true
		}
	})
	
	return hasData ? interfaceData : null
}

export default memo(function TotalBandwidthChart({
	chartData,
}: {
	chartData: ChartData
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { i18n } = useLingui()
	const userSettings = useStore($userSettings)

	// Transform data to include per-interface bandwidth
	const { transformedData, interfaceNames } = useMemo(() => {
		const allInterfaces = new Set<string>()
		
		// First pass: collect all interface names
		chartData.systemStats.forEach(dataPoint => {
			const interfaceData = getPerInterfaceBandwidth(dataPoint)
			if (interfaceData) {
				Object.keys(interfaceData).forEach(name => allInterfaces.add(name))
			}
		})
		
		const interfaceNames = Array.from(allInterfaces).sort()
		
		// Second pass: transform data with per-interface values
		const transformedData = chartData.systemStats.map(dataPoint => {
			const interfaceData = getPerInterfaceBandwidth(dataPoint)
			const result: any = { ...dataPoint }
			
			interfaceNames.forEach(interfaceName => {
				const data = interfaceData?.[interfaceName]
				result[`${interfaceName}_sent`] = data?.sent || 0
				result[`${interfaceName}_recv`] = data?.recv || 0
			})
			
			return result
		})
		
		return { transformedData, interfaceNames }
	}, [chartData.systemStats])

	// Generate dynamic data keys for each interface using same color scheme as NetworkInterfaceChart
	const dataKeys = useMemo(() => {
		const keys: Array<{ name: string; dataKey: string; color: string }> = []
		
		interfaceNames.forEach((interfaceName, index) => {
			// Use the same color calculation as NetworkInterfaceChart
			const hue = ((index * 360) / Math.max(interfaceNames.length, 1)) % 360
			
			keys.push({
				name: `${interfaceName} Sent`,
				dataKey: `${interfaceName}_sent`,
				color: `hsl(${hue}, 70%, 45%)`, // Darker shade for sent (same as NetworkInterfaceChart)
			})
			
			keys.push({
				name: `${interfaceName} Received`,
				dataKey: `${interfaceName}_recv`, 
				color: `hsl(${hue}, 70%, 65%)`, // Lighter shade for received (same as NetworkInterfaceChart)
			})
		})
		
		return keys
	}, [interfaceNames])

	return (
		<div>
			<ChartContainer
				className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart accessibilityLayer data={transformedData} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						width={yAxisWidth}
						tickFormatter={(value) => {
							const { value: formattedValue, unit } = formatBytes(value, false, userSettings.unitNet ?? Unit.Bytes)
							const rounded = toFixedFloat(formattedValue, formattedValue >= 10 ? 1 : 2)
							return updateYAxisWidth(`${rounded} ${unit}`)
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
								contentFormatter={({ value }: any) => {
									const { value: formattedValue, unit } = formatBytes(value, false, userSettings.unitNet ?? Unit.Bytes)
									return <span className="flex">{decimalString(formattedValue, formattedValue >= 10 ? 1 : 2)} {unit}</span>
								}}
							/>
						}
					/>
					<ChartLegend content={<ChartLegendContent />} />
					{dataKeys.map((key, i) => (
						<Area
							key={i}
							dataKey={key.dataKey}
							name={key.name}
							type="monotoneX"
							fill={key.color}
							fillOpacity={0.3}
							stroke={key.color}
							strokeOpacity={1}
							isAnimationActive={false}
						/>
					))}
				</AreaChart>
			</ChartContainer>
		</div>
	)
}) 