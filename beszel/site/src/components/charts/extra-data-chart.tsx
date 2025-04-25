import { t } from "@lingui/core/macro"

import { Line, LineChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	decimalString,
	chartMargin,
} from "@/lib/utils"
// import Spinner from '../spinner'
import { ChartData, EDataConfig, EDataKey } from "@/types"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"

/** [label, key, color, opacity] */
type DataKeys = [string, string, number, number]

export default memo(function ExtraDataChart({
	maxToggled = false,
	eDataConfig,
	chartData,
	max,
}: {
	maxToggled?: boolean
	eDataConfig: EDataConfig
	chartData: ChartData
	max?: number
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { i18n } = useLingui()

	const { chartTime } = chartData

	const showMax = chartTime !== "1h" && maxToggled

	const dataKeys: DataKeys[] = useMemo(() => {
		// [label, key, color, opacity]
		const dataKeysBuilder = []
		for (const [key, value] of eDataConfig.keys) {
			dataKeysBuilder.push([value.label, key, value.color, value.opacity])
		}
		return dataKeysBuilder
	}, [eDataConfig.name, i18n.locale])

	// console.log('Rendered at', new Date())

	if (chartData.systemStats.length === 0) {
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
						domain={[0, max ?? "auto"]}
						tickFormatter={(value) => {
							const val = toFixedWithoutTrailingZeros(value, 2)
							return updateYAxisWidth(val + unit)
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
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => decimalString(item) + eDataConfig.unit}
								// indicator="line"
							/>
						}
					/>
					{dataKeys.map((key, i) => {
						const color = `hsl(var(--chart-${key[2]}))`
						return (
							<Area
								key={i}
								dataKey={key[1]}
								name={key[0]}
								type="monotoneX"
								dot={false}
								strokeWidth={1.5}
								stroke={color}
								isAnimationActive={false}
							/>
						)
					})}
					{/* <ChartLegend content={<ChartLegendContent />} /> */}
				</LineChart>
			</ChartContainer>
		</div>
	)
})
