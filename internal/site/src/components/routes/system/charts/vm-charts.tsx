import { t } from "@lingui/core/macro"
import type { ChartConfig } from "@/components/ui/chart"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { $userSettings, $vmFilter } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData } from "@/types"
import { Unit } from "@/lib/enums"
import { ChartCard, FilterBar } from "../chart-card"

type VMChartProps = {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	config: ChartConfig
}

export function VMCPUChart({ chartData, grid, dataEmpty, config }: VMChartProps) {
	const { filter, dataPoints } = useContainerDataPoints(config, (key, data) => data[key]?.c ?? null, $vmFilter)
	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Virtual Machine CPU Usage`}
			description={t`CPU usage normalized to total host capacity`}
			cornerEl={<FilterBar store={$vmFilter} />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.vmData}
				dataPoints={dataPoints}
				tickFormatter={(value) => `${toFixedFloat(value, 2)}%`}
				contentFormatter={({ value }) => `${decimalString(value)}%`}
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}

export function VMMemoryChart({ chartData, grid, dataEmpty, config }: VMChartProps) {
	const { filter, dataPoints } = useContainerDataPoints(
		config,
		(key, data) => (data[key]?.mf ? (data[key]?.m ?? 0) * 1024 * 1024 : null),
		$vmFilter
	)
	return (
		<VMByteChart
			chartData={chartData}
			grid={grid}
			dataEmpty={dataEmpty}
			filter={filter}
			dataPoints={dataPoints}
			title={t`Virtual Machine Memory Usage`}
			description={t`Guest memory usage reported by the balloon driver`}
		/>
	)
}

export function VMNetworkChart({ chartData, grid, dataEmpty, config }: VMChartProps) {
	const { filter, dataPoints } = useContainerDataPoints(
		config,
		(key, data) => (data[key]?.b?.[0] ?? 0) + (data[key]?.b?.[1] ?? 0),
		$vmFilter
	)
	return (
		<VMByteChart
			chartData={chartData}
			grid={grid}
			dataEmpty={dataEmpty}
			filter={filter}
			dataPoints={dataPoints}
			title={t`Virtual Machine Network I/O`}
			description={t`Combined transmit and receive throughput`}
			perSecond={true}
			unitKind="network"
		/>
	)
}

export function VMDiskChart({ chartData, grid, dataEmpty, config }: VMChartProps) {
	const { filter, dataPoints } = useContainerDataPoints(
		config,
		(key, data) => (data[key]?.d?.[0] ?? 0) + (data[key]?.d?.[1] ?? 0),
		$vmFilter
	)
	return (
		<VMByteChart
			chartData={chartData}
			grid={grid}
			dataEmpty={dataEmpty}
			filter={filter}
			dataPoints={dataPoints}
			title={t`Virtual Machine Disk I/O`}
			description={t`Combined read and write throughput`}
			perSecond={true}
			unitKind="disk"
		/>
	)
}

function VMByteChart({
	chartData,
	grid,
	dataEmpty,
	filter,
	dataPoints,
	title,
	description,
	perSecond = false,
	unitKind = "memory",
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	filter: string
	dataPoints: ReturnType<typeof useContainerDataPoints>["dataPoints"]
	title: string
	description: string
	perSecond?: boolean
	unitKind?: "memory" | "network" | "disk"
}) {
	const settings = $userSettings.get()
	const formatter = (raw: number) => {
		const preferredUnit =
			unitKind === "network" ? settings.unitNet : unitKind === "disk" ? settings.unitDisk : Unit.Bytes
		const { value, unit } = formatBytes(raw, perSecond, preferredUnit, false)
		return `${decimalString(value)} ${unit}`
	}
	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={title}
			description={description}
			cornerEl={<FilterBar store={$vmFilter} />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.vmData}
				dataPoints={dataPoints}
				tickFormatter={formatter}
				contentFormatter={({ value }) => formatter(value)}
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}
