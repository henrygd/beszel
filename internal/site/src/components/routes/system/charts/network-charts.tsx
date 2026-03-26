import { useMemo } from "react"
import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartConfig } from "@/components/ui/chart"
import { pinnedAxisDomain } from "@/components/ui/chart"
import type { ChartData, SystemStatsRecord } from "@/types"
import { Separator } from "@/components/ui/separator"
import NetworkSheet from "../network-sheet"
import { ChartCard, FilterBar, SelectAvgMax } from "../chart-card"
import { dockerOrPodman } from "../chart-data"

export function BandwidthChart({
	chartData,
	grid,
	dataEmpty,
	showMax,
	isLongerChart,
	maxValues,
	systemStats,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	showMax: boolean
	isLongerChart: boolean
	maxValues: boolean
	systemStats: SystemStatsRecord[]
}) {
	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null
	const userSettings = $userSettings.get()

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Bandwidth`}
			cornerEl={
				<div className="flex gap-2">
					{maxValSelect}
					<NetworkSheet chartData={chartData} dataEmpty={dataEmpty} grid={grid} maxValues={maxValues} />
				</div>
			}
			description={t`Network traffic of public interfaces`}
		>
			<AreaChartDefault
				chartData={chartData}
				maxToggled={showMax}
				dataPoints={[
					{
						label: t`Sent`,
						dataKey(data: SystemStatsRecord) {
							if (showMax) {
								return data?.stats?.bm?.[0] ?? (data?.stats?.nsm ?? 0) * 1024 * 1024
							}
							return data?.stats?.b?.[0] ?? (data?.stats?.ns ?? 0) * 1024 * 1024
						},
						color: 5,
						opacity: 0.2,
					},
					{
						label: t`Received`,
						dataKey(data: SystemStatsRecord) {
							if (showMax) {
								return data?.stats?.bm?.[1] ?? (data?.stats?.nrm ?? 0) * 1024 * 1024
							}
							return data?.stats?.b?.[1] ?? (data?.stats?.nr ?? 0) * 1024 * 1024
						},
						color: 2,
						opacity: 0.2,
					},
				]
					// try to place the lesser number in front for better visibility
					.sort(() => (systemStats.at(-1)?.stats.b?.[1] ?? 0) - (systemStats.at(-1)?.stats.b?.[0] ?? 0))}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
					return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={(data) => {
					const { value, unit } = formatBytes(data.value, true, userSettings.unitNet, false)
					return `${decimalString(value, value >= 100 ? 1 : 2)} ${unit}`
				}}
				showTotal={true}
			/>
		</ChartCard>
	)
}

export function ContainerNetworkChart({
	chartData,
	grid,
	dataEmpty,
	isPodman,
	networkConfig,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	isPodman: boolean
	networkConfig: ChartConfig
}) {
	const userSettings = $userSettings.get()
	const { filter, dataPoints, filteredKeys } = useContainerDataPoints(networkConfig, (key, data) => {
		const payload = data[key]
		if (!payload) return null
		const sent = payload?.b?.[0] ?? (payload?.ns ?? 0) * 1024 * 1024
		const recv = payload?.b?.[1] ?? (payload?.nr ?? 0) * 1024 * 1024
		return sent + recv
	})

	const contentFormatter = useMemo(() => {
		const getRxTxBytes = (record?: { b?: [number, number]; ns?: number; nr?: number }) => {
			if (record?.b?.length && record.b.length >= 2) {
				return [Number(record.b[0]) || 0, Number(record.b[1]) || 0]
			}
			return [(record?.ns ?? 0) * 1024 * 1024, (record?.nr ?? 0) * 1024 * 1024]
		}
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
				if (key === "__total__") {
					let totalSent = 0
					let totalRecv = 0
					const payloadData = item?.payload && typeof item.payload === "object" ? item.payload : {}
					for (const [containerKey, value] of Object.entries(payloadData)) {
						if (!value || typeof value !== "object") continue
						if (filteredKeys.has(containerKey)) continue
						const [sent, recv] = getRxTxBytes(value as { b?: [number, number]; ns?: number; nr?: number })
						totalSent += sent
						totalRecv += recv
					}
					return formatRxTx(totalRecv, totalSent)
				}
				const [sent, recv] = getRxTxBytes(item?.payload?.[key])
				return formatRxTx(recv, sent)
			} catch {
				return null
			}
		}
	}, [filteredKeys, userSettings.unitNet])

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={dockerOrPodman(t`Docker Network I/O`, isPodman)}
			description={dockerOrPodman(t`Network traffic of docker containers`, isPodman)}
			cornerEl={<FilterBar />}
		>
			<AreaChartDefault
				chartData={chartData}
				customData={chartData.containerData}
				dataPoints={dataPoints}
				tickFormatter={(val) => {
					const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
					return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
				}}
				contentFormatter={contentFormatter}
				domain={pinnedAxisDomain()}
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}
