import { t } from "@lingui/core/macro"
import { useMemo } from "react"
import AreaChartDefault, { type DataPoint } from "@/components/charts/area-chart"
import GpuPowerChart from "@/components/charts/gpu-power-chart"
import LineChartDefault from "@/components/charts/line-chart"
import { Unit } from "@/lib/enums"
import { cn, decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData, GPUData, SystemStatsRecord } from "@/types"
import { ChartCard } from "./shared"
import type { BaseTabProps } from "./types"

function GpuEnginesChart({ chartData }: { chartData: ChartData }) {
	const { gpuId, engines } = useMemo(() => {
		for (let i = chartData.systemStats.length - 1; i >= 0; i--) {
			const gpus = chartData.systemStats[i].stats?.g
			if (!gpus) continue
			for (const id in gpus) {
				if (gpus[id].e) {
					return { gpuId: id, engines: Object.keys(gpus[id].e).sort() }
				}
			}
		}
		return { gpuId: null, engines: [] }
	}, [chartData.systemStats])

	if (!gpuId) return null

	const dataPoints: DataPoint[] = engines.map((engine, i) => ({
		label: engine,
		dataKey: ({ stats }: SystemStatsRecord) => stats?.g?.[gpuId]?.e?.[engine] ?? 0,
		color: `hsl(${140 + (((i * 360) / engines.length) % 360)}, 65%, 52%)`,
		opacity: 0.35,
	}))

	return (
		<LineChartDefault
			legend={true}
			chartData={chartData}
			dataPoints={dataPoints}
			tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
			contentFormatter={({ value }) => `${decimalString(value)}%`}
		/>
	)
}

export function GpuTab({ chartData, grid }: BaseTabProps) {
	const systemStats = chartData.systemStats
	const dataEmpty = systemStats.length === 0

	// Compute GPU flags internally
	const { hasGpuEnginesData, hasGpuPowerData } = useMemo(() => {
		let engines = false
		let power = false
		for (let i = 0; i < systemStats.length && (!engines || !power); i++) {
			const gpus = systemStats[i].stats?.g
			if (!gpus) continue
			for (const id in gpus) {
				if (!engines && gpus[id].e !== undefined) engines = true
				if (!power && (gpus[id].p !== undefined || gpus[id].pp !== undefined)) power = true
				if (engines && power) break
			}
		}
		return { hasGpuEnginesData: engines, hasGpuPowerData: power }
	}, [systemStats])

	return (
		<div className="grid xl:grid-cols-2 gap-4">
			{hasGpuPowerData && (
				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={t`GPU Power Draw`}
					description={t`Average power consumption of GPUs`}
				>
					<GpuPowerChart chartData={chartData} />
				</ChartCard>
			)}

			{hasGpuEnginesData && (
				<ChartCard
					legend={true}
					empty={dataEmpty}
					grid={grid}
					title={t`GPU Engines`}
					description={t`Average utilization of GPU engines`}
				>
					<GpuEnginesChart chartData={chartData} />
				</ChartCard>
			)}

			{Object.keys(systemStats.at(-1)?.stats.g ?? {}).map((id) => {
				const gpu = systemStats.at(-1)?.stats.g?.[id] as GPUData
				return (
					<div key={id} className="contents">
						<ChartCard
							className={cn(grid && "!col-span-1")}
							empty={dataEmpty}
							grid={grid}
							title={`${gpu.n} ${t`Usage`}`}
							description={t`Average utilization of ${gpu.n}`}
						>
							<AreaChartDefault
								chartData={chartData}
								dataPoints={[
									{
										label: t`Usage`,
										dataKey: ({ stats }) => stats?.g?.[id]?.u ?? 0,
										color: 1,
										opacity: 0.35,
									},
								]}
								tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
								contentFormatter={({ value }) => `${decimalString(value)}%`}
							/>
						</ChartCard>

						{(gpu.mt ?? 0) > 0 && (
							<ChartCard
								empty={dataEmpty}
								grid={grid}
								title={`${gpu.n} VRAM`}
								description={t`Precise utilization at the recorded time`}
							>
								<AreaChartDefault
									chartData={chartData}
									dataPoints={[
										{
											label: t`Usage`,
											dataKey: ({ stats }) => stats?.g?.[id]?.mu ?? 0,
											color: 2,
											opacity: 0.25,
										},
									]}
									max={gpu.mt}
									tickFormatter={(val) => {
										const { value, unit } = formatBytes(val, false, Unit.Bytes, true)
										return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
									}}
									contentFormatter={({ value }) => {
										const { value: convertedValue, unit } = formatBytes(value, false, Unit.Bytes, true)
										return `${decimalString(convertedValue)} ${unit}`
									}}
								/>
							</ChartCard>
						)}
					</div>
				)
			})}
		</div>
	)
}
