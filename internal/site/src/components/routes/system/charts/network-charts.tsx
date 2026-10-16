import { useMemo } from "react"
import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData, SystemStatsRecord } from "@/types"
import { Separator } from "@/components/ui/separator"
import NetworkSheet, { ErrorsSheet, PacketsSheet, packetContentFormatter, packetTickFormatter } from "../network-sheet"
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
	fullWidth = false,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	showMax: boolean
	isLongerChart: boolean
	maxValues: boolean
	systemStats: SystemStatsRecord[]
	fullWidth?: boolean
}) {
	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null
	const userSettings = $userSettings.get()

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Bandwidth`}
			className={fullWidth ? "col-span-full" : undefined}
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

/** Sum a `nip` rate index across all interfaces */
function sumPacketRate(data: SystemStatsRecord, index: number) {
	const nip = data?.stats?.nip
	if (!nip) return undefined
	let total = 0
	for (const rates of Object.values(nip)) {
		total += rates[index] ?? 0
	}
	return total
}

interface PacketChartProps {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	maxValues: boolean
	systemStats: SystemStatsRecord[]
}

export function PacketsChart({ chartData, grid, dataEmpty, maxValues, systemStats }: PacketChartProps) {
	// agents before packet stats were added don't send `nip`
	if (!systemStats.at(-1)?.stats?.nip) {
		return null
	}

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Packets`}
			cornerEl={<PacketsSheet chartData={chartData} dataEmpty={dataEmpty} grid={grid} maxValues={maxValues} />}
			description={t`Packets per second on public interfaces`}
			legend={true}
		>
			<AreaChartDefault
				chartData={chartData}
				dataPoints={[
					{ label: t`Sent`, dataKey: (data) => sumPacketRate(data, 0), color: 5, opacity: 0.2 },
					{ label: t`Received`, dataKey: (data) => sumPacketRate(data, 1), color: 2, opacity: 0.2 },
				]}
				tickFormatter={packetTickFormatter}
				contentFormatter={packetContentFormatter}
				legend={true}
				showTotal={true}
			/>
		</ChartCard>
	)
}

export function NetworkErrorsChart({ chartData, grid, dataEmpty, maxValues, systemStats }: PacketChartProps) {
	if (!systemStats.at(-1)?.stats?.nip) {
		return null
	}

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Errors & Discards`}
			cornerEl={<ErrorsSheet chartData={chartData} dataEmpty={dataEmpty} grid={grid} maxValues={maxValues} />}
			description={t`Packet errors and discards per second on public interfaces`}
			legend={true}
		>
			<AreaChartDefault
				chartData={chartData}
				dataPoints={[
					{ label: t`Errors Sent`, dataKey: (data) => sumPacketRate(data, 2), color: 5, opacity: 0.2 },
					{ label: t`Errors Received`, dataKey: (data) => sumPacketRate(data, 3), color: 2, opacity: 0.2 },
					{ label: t`Discards Sent`, dataKey: (data) => sumPacketRate(data, 4), color: 3, opacity: 0.2 },
					{ label: t`Discards Received`, dataKey: (data) => sumPacketRate(data, 5), color: 1, opacity: 0.2 },
				]}
				tickFormatter={packetTickFormatter}
				contentFormatter={packetContentFormatter}
				legend={true}
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
			description={t`Network traffic of containers`}
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
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}
