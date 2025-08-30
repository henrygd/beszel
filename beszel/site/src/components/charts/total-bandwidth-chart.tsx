import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"
import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { useYAxisWidth, cn, formatShortDate, chartMargin, formatBytes, toFixedFloat, decimalString } from "@/lib/utils"
import { ChartData } from "@/types"
import { useStore } from "@nanostores/react"
import { $userSettings } from "@/lib/stores"
import { Unit } from "@/lib/enums"

const getTotalBandwidth = (data: any): { sent: number; recv: number } | null => {
	const networkInterfaces = data?.stats?.ni
	if (!networkInterfaces) return null
	
	let totalSent = 0, totalRecv = 0
	Object.values(networkInterfaces).forEach((iface: any) => {
		if (iface.tbs) totalSent += iface.tbs
		if (iface.tbr) totalRecv += iface.tbr
	})
	
	if (totalSent === 0 && totalRecv === 0) return null
	return { sent: totalSent, recv: totalRecv }
}

export default memo(function TotalBandwidthChart({
	chartData,
}: {
	chartData: ChartData
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { i18n } = useLingui()
	const userSettings = useStore($userSettings)

	// Transform data to include total bandwidth
	const transformedData = useMemo(() => {
		return chartData.systemStats.map(dataPoint => {
			const bandwidth = getTotalBandwidth(dataPoint)
			return {
				...dataPoint,
				totalSent: bandwidth?.sent || 0,
				totalRecv: bandwidth?.recv || 0,
			}
		})
	}, [chartData.systemStats])

	const dataKeys = [
		{
			name: "Total Sent",
			dataKey: "totalSent",
			color: "hsl(142, 70%, 45%)", // Green
		},
		{
			name: "Total Received", 
			dataKey: "totalRecv",
			color: "hsl(213, 70%, 55%)", // Blue
		}
	]

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
					{dataKeys.map((key, i) => (
						<Area
							key={i}
							dataKey={key.dataKey}
							name={key.name}
							type="monotoneX"
							fill={key.color}
							fillOpacity={0.4}
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