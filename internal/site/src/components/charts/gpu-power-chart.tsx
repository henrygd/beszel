import { memo, useMemo } from "react"
import { CartesianGrid, Line, LineChart, YAxis } from "recharts"
import {
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
	xAxis,
} from "@/components/ui/chart"
import { chartMargin, cn, decimalString, formatShortDate, toFixedFloat } from "@/lib/utils"
import type { ChartData, GPUData } from "@/types"
import { useYAxisWidth } from "./hooks"
import type { DataPoint } from "./line-chart"

export default memo(function GpuPowerChart({ chartData }: { chartData: ChartData }) {
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()
	const packageKey = " package"

	const { gpuData, dataPoints } = useMemo(() => {
		const dataPoints = [] as DataPoint[]
		const gpuData = [] as Record<string, GPUData | string>[]
		const addedKeys = new Map<string, number>()

		const addKey = (key: string, value: number) => {
			addedKeys.set(key, (addedKeys.get(key) ?? 0) + value)
		}

		for (const stats of chartData.systemStats) {
			const gpus = stats.stats?.g ?? {}
			const data = { created: stats.created } as Record<string, GPUData | string>
			for (const id in gpus) {
				const gpu = gpus[id] as GPUData
				data[gpu.n] = gpu
				addKey(gpu.n, gpu.p ?? 0)
				if (gpu.pp) {
					data[`${gpu.n}${packageKey}`] = gpu
					addKey(`${gpu.n}${packageKey}`, gpu.pp ?? 0)
				}
			}
			gpuData.push(data)
		}
		const sortedKeys = Array.from(addedKeys.entries())
			.sort(([, a], [, b]) => b - a)
			.map(([key]) => key)

		for (let i = 0; i < sortedKeys.length; i++) {
			const id = sortedKeys[i]
			dataPoints.push({
				label: id,
				dataKey: (gpuData: Record<string, GPUData>) => {
					return id.endsWith(packageKey) ? (gpuData[id]?.pp ?? 0) : (gpuData[id]?.p ?? 0)
				},
				color: `hsl(${226 + (((i * 360) / addedKeys.size) % 360)}, 65%, 52%)`,
			})
		}
		return { gpuData, dataPoints }
	}, [chartData])

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
				<LineChart accessibilityLayer data={gpuData} margin={chartMargin}>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						domain={[0, "auto"]}
						width={yAxisWidth}
						tickFormatter={(value) => {
							const val = toFixedFloat(value, 2)
							return updateYAxisWidth(`${val}W`)
						}}
						tickLine={false}
						axisLine={false}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						// @ts-expect-error
						itemSorter={(a, b) => b.value - a.value}
						content={
							<ChartTooltipContent
								labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
								contentFormatter={(item) => `${decimalString(item.value)}W`}
								// indicator="line"
							/>
						}
					/>
					{dataPoints.map((dataPoint) => (
						<Line
							key={dataPoint.label}
							dataKey={dataPoint.dataKey}
							name={dataPoint.label}
							type="monotoneX"
							dot={false}
							strokeWidth={1.5}
							stroke={dataPoint.color as string}
							isAnimationActive={false}
						/>
					))}
					{dataPoints.length > 1 && <ChartLegend content={<ChartLegendContent />} />}
				</LineChart>
			</ChartContainer>
		</div>
	)
})
