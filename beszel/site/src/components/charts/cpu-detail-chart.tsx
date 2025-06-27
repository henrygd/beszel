import { t } from "@lingui/core/macro";

import { Area, AreaChart, CartesianGrid, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	toFixedWithoutTrailingZeros,
	decimalString,
	chartMargin,
} from "@/lib/utils"
import { ChartData } from "@/types"
import { memo } from "react"

function roundUpToNext5(val: number) {
	return Math.ceil(val / 5) * 5;
}

export default memo(function CpuDetailChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	if (!Array.isArray(chartData.systemStats) || chartData.systemStats.length === 0) {
		return null
	}

	const maxCpu = chartData.systemStats.reduce((max, s) => {
		if (!s.stats || typeof s.stats !== 'object') return max;
		const cu = Number(s.stats.cu) || 0;
		const cs = Number(s.stats.cs) || 0;
		const ci = Number(s.stats.ci) || 0;
		const cst = Number(s.stats.cst) || 0;
		const sum = cu + cs + ci + cst;
		return Math.max(max, sum);
	}, 0);
	const yMax = Math.max(5, roundUpToNext5(maxCpu));

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
						domain={[0, yMax]}
						tickCount={6}
						width={yAxisWidth}
						tickLine={false}
						axisLine={false}
						tickFormatter={(value) => updateYAxisWidth(value + "%")}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => decimalString(item.value) + "%"}
								// indicator="line"
							/>
						}
					/>
					<Area
						dataKey="stats.cu"
						name={t`User`}
						order={4}
						type="monotoneX"
						fill="hsla(0 60% 45% / 0.8)"
						fillOpacity={0.4}
						stroke="hsla(0 60% 45% / 0.8)"
						stackId="1"
						isAnimationActive={false}
					/>
					<Area
						dataKey="stats.cs"
						name={t`System`}
						order={3}
						type="monotoneX"
						fill="hsla(200 60% 45% / 0.8)"
						fillOpacity={0.4}
						stroke="hsla(200 60% 45% / 0.8)"
						stackId="1"
						isAnimationActive={false}
					/>
					<Area
						dataKey="stats.ci"
						name={t`IOWait`}
						order={2}
						type="monotoneX"
						fill="hsla(60 60% 45% / 0.8)"
						fillOpacity={0.4}
						stroke="hsla(60 60% 45% / 0.8)"
						stackId="1"
						isAnimationActive={false}
					/>
					<Area
						dataKey="stats.cst"
						name={t`Steal`}
						order={1}
						type="monotoneX"
						fill="hsla(300 60% 45% / 0.8)"
						fillOpacity={0.4}
						stroke="hsla(300 60% 45% / 0.8)"
						stackId="1"
						isAnimationActive={false}
					/>
				</AreaChart>
			</ChartContainer>
		</div>
	)
}) 