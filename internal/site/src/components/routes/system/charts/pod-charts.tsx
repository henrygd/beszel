import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { Unit } from "@/lib/enums"
import { $podFilter, $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData } from "@/types"
import { Separator } from "@/components/ui/separator"
import { useMemo } from "react"
import { ChartCard, FilterBar } from "../chart-card"

export function PodCpuChart({
	chartData,
	grid,
	dataEmpty,
	cpuConfig,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	cpuConfig: ChartConfig
}) {
	const { filter, dataPoints } = useContainerDataPoints(cpuConfig, (key, data) => data[key]?.c ?? null, $podFilter)

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Pod CPU Usage`}
			description={t`Average CPU utilization of pods`}
			cornerEl={<FilterBar store={$podFilter} />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.podData}
				dataPoints={dataPoints}
				tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
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

export function PodMemoryChart({
	chartData,
	grid,
	dataEmpty,
	memoryConfig,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	memoryConfig: ChartConfig
}) {
	const { filter, dataPoints } = useContainerDataPoints(memoryConfig, (key, data) => data[key]?.m ?? null, $podFilter)

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Pod Memory Usage`}
			description={t`Memory usage of pods`}
			cornerEl={<FilterBar store={$podFilter} />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.podData}
				dataPoints={dataPoints}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val, false, Unit.Bytes, true)
					return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={(item) => {
					const { value, unit } = formatBytes(item.value, false, Unit.Bytes, true)
					return `${decimalString(value)} ${unit}`
				}}
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}

export function PodNetworkChart({
	chartData,
	grid,
	dataEmpty,
	networkConfig,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	networkConfig: ChartConfig
}) {
	const userSettings = $userSettings.get()
	const { filter, dataPoints, filteredKeys } = useContainerDataPoints(
		networkConfig,
		(key, data) => {
			const payload = data[key]
			if (!payload) return null
			const sent = (payload?.ns ?? 0) * 1024 * 1024
			const recv = (payload?.nr ?? 0) * 1024 * 1024
			return sent + recv
		},
		$podFilter
	)

	const contentFormatter = useMemo(() => {
		const formatRxTx = (recv: number, sent: number) => {
			const { value: receivedValue, unit: receivedUnit } = formatBytes(recv, true, userSettings.unitNet, false)
			const { value: sentValue, unit: sentUnit } = formatBytes(sent, true, userSettings.unitNet, false)
			return (
				<span className="flex">
					{decimalString(receivedValue)} {receivedUnit}
					<span className="opacity-70 ms-0.5"> rx </span>
					<Separator orientation="vertical" className="h-3 mx-1.5 bg-primary/40" />
					{decimalString(sentValue)} {sentUnit}
					<span className="opacity-70 ms-0.5"> tx</span>
				</span>
			)
		}
		// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item
		return (item: any, key: string) => {
			try {
				const payloadData = item?.payload && typeof item.payload === "object" ? item.payload : {}
				if (key === "__total__") {
					let totalSent = 0
					let totalRecv = 0
					for (const [podKey, value] of Object.entries(payloadData)) {
						if (!value || typeof value !== "object") continue
						if (filteredKeys.has(podKey)) continue
						const record = value as { ns?: number; nr?: number }
						totalSent += (record.ns ?? 0) * 1024 * 1024
						totalRecv += (record.nr ?? 0) * 1024 * 1024
					}
					return formatRxTx(totalRecv, totalSent)
				}
				const record = item?.payload?.[key] as { ns?: number; nr?: number } | undefined
				return formatRxTx((record?.nr ?? 0) * 1024 * 1024, (record?.ns ?? 0) * 1024 * 1024)
			} catch {
				return null
			}
		}
	}, [filteredKeys, userSettings.unitNet])

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Pod Network I/O`}
			description={t`Network traffic of pods`}
			cornerEl={<FilterBar store={$podFilter} />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.podData}
				dataPoints={dataPoints}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
					return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={contentFormatter}
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}
