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
import { ChartData, EDataConfig } from "@/types"
import { memo, useMemo } from "react"
import { useLingui } from "@lingui/react/macro"

/** [label, key, color, opacity] */
type DataKeys = [string, string, number, number]

export default memo(function ExtraDataChart({
	eDataConfig,
	chartData,
	max,
}: {
	eDataConfig: EDataConfig
	chartData: ChartData
	max?: number
}) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const { i18n } = useLingui()

	const dataKeys: DataKeys[] = useMemo(() => {
		// [label, key, color, opacity]
		const dataKeysBuilder: DataKeys[] = []
		for (const [key, value] of Object.entries(eDataConfig.keys)) {
			dataKeysBuilder.push([t`${value.label}`, key, value.color, value.opacity])
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
							return updateYAxisWidth(val + eDataConfig.unit)
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
							<Line
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
