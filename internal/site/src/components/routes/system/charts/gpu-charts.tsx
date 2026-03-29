import { t } from "@lingui/core/macro"
import { useRef, useMemo } from "react"
import AreaChartDefault, { type DataPoint } from "@/components/charts/area-chart"
import LineChartDefault from "@/components/charts/line-chart"
import { Unit } from "@/lib/enums"
import { cn, decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData, GPUData, SystemStatsRecord } from "@/types"
import { ChartCard } from "../chart-card"

/** GPU power draw chart for the main grid */
export function GpuPowerChart({
	chartData,
	grid,
	dataEmpty,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	const packageKey = " package"
	const statsRef = useRef(chartData.systemStats)
	statsRef.current = chartData.systemStats

	// Derive GPU power config key (cheap per render)
	let gpuPowerKey = ""
	for (let i = chartData.systemStats.length - 1; i >= 0; i--) {
		const gpus = chartData.systemStats[i].stats?.g
		if (gpus) {
			const parts: string[] = []
			for (const id in gpus) {
				const gpu = gpus[id] as GPUData
				if (gpu.p !== undefined) parts.push(`${id}:${gpu.n}`)
				if (gpu.pp !== undefined) parts.push(`${id}:${gpu.n}${packageKey}`)
			}
			gpuPowerKey = parts.sort().join("\0")
			break
		}
	}

	const dataPoints = useMemo((): DataPoint[] => {
		if (!gpuPowerKey) return []
		const totals = new Map<string, { label: string; gpuId: string; isPackage: boolean; total: number }>()
		for (const record of statsRef.current) {
			const gpus = record.stats?.g
			if (!gpus) continue
			for (const id in gpus) {
				const gpu = gpus[id] as GPUData
				const key = gpu.n
				const existing = totals.get(key)
				if (existing) {
					existing.total += gpu.p ?? 0
				} else {
					totals.set(key, { label: gpu.n, gpuId: id, isPackage: false, total: gpu.p ?? 0 })
				}
				if (gpu.pp !== undefined) {
					const pkgKey = `${gpu.n}${packageKey}`
					const existingPkg = totals.get(pkgKey)
					if (existingPkg) {
						existingPkg.total += gpu.pp
					} else {
						totals.set(pkgKey, { label: pkgKey, gpuId: id, isPackage: true, total: gpu.pp })
					}
				}
			}
		}
		const sorted = Array.from(totals.values()).sort((a, b) => b.total - a.total)
		return sorted.map(
			(entry, i): DataPoint => ({
				label: entry.label,
				dataKey: (data: SystemStatsRecord) => {
					const gpu = data.stats?.g?.[entry.gpuId]
					return entry.isPackage ? (gpu?.pp ?? 0) : (gpu?.p ?? 0)
				},
				color: `hsl(${226 + (((i * 360) / sorted.length) % 360)}, 65%, 52%)`,
				opacity: 1,
			})
		)
	}, [gpuPowerKey])

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`GPU Power Draw`}
			description={t`Average power consumption of GPUs`}
		>
			<LineChartDefault
				legend={dataPoints.length > 1}
				chartData={chartData}
				dataPoints={dataPoints}
				itemSorter={(a: { value: number }, b: { value: number }) => b.value - a.value}
				tickFormatter={(val) => `${toFixedFloat(val, 2)}W`}
				contentFormatter={({ value }) => `${decimalString(value)}W`}
			/>
		</ChartCard>
	)
}

/** GPU detail grid (engines + per-GPU usage/VRAM) — rendered outside the main 2-col grid */
export function GpuDetailCharts({
	chartData,
	grid,
	dataEmpty,
	lastGpus,
	hasGpuEnginesData,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	lastGpus: Record<string, GPUData>
	hasGpuEnginesData: boolean
}) {
	return (
		<div className="grid xl:grid-cols-2 gap-4">
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
			{Object.keys(lastGpus).map((id) => {
				const gpu = lastGpus[id] as GPUData
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

function GpuEnginesChart({ chartData }: { chartData: ChartData }) {
	// Derive stable engine config key (cheap per render)
	let enginesKey = ""
	for (let i = chartData.systemStats.length - 1; i >= 0; i--) {
		const gpus = chartData.systemStats[i].stats?.g
		if (!gpus) continue
		for (const id in gpus) {
			if (gpus[id].e) {
				enginesKey = id + "\0" + Object.keys(gpus[id].e).sort().join("\0")
				break
			}
		}
		if (enginesKey) break
	}

	const { gpuId, dataPoints } = useMemo((): { gpuId: string | null; dataPoints: DataPoint[] } => {
		if (!enginesKey) return { gpuId: null, dataPoints: [] }
		const parts = enginesKey.split("\0")
		const gId = parts[0]
		const engineNames = parts.slice(1)
		return {
			gpuId: gId,
			dataPoints: engineNames.map((engine, i) => ({
				label: engine,
				dataKey: ({ stats }: SystemStatsRecord) => stats?.g?.[gId]?.e?.[engine] ?? 0,
				color: `hsl(${140 + (((i * 360) / engineNames.length) % 360)}, 65%, 52%)`,
				opacity: 0.35,
			})),
		}
	}, [enginesKey])

	if (!gpuId) {
		return null
	}

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
