import AreaChartDefault from "@/components/charts/area-chart"
import type { DataPoint } from "@/components/charts/area-chart"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import { Unit } from "@/lib/enums"
import { useLingui } from "@lingui/react/macro"
import { ChartCard } from "../chart-card"
import type { ChartData, SpeedtestStatsRecord } from "@/types"
import { useMemo } from "react"

/**
 * Format a bandwidth in bytes/s as bits per second, the unit speedtests are usually quoted in.
 * The short form is for axis ticks and uses a non-breaking space, since recharts measures tick
 * words without the axis font styles and would otherwise wrap wide labels like "840 Mbps".
 */
export function formatBandwidth(bytesPerSecond: number, short = false) {
	const { value, unit } = formatBytes(bytesPerSecond, true, Unit.Bits, false)
	if (short) return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
	return `${decimalString(value, value >= 100 ? 1 : 2)} ${unit}`
}

type SpeedtestChartProps = {
	stats: SpeedtestStatsRecord[]
	chartData: ChartData
	empty: boolean
}

// Series overlap rather than stack, so a translucent fill keeps each one visible.
// Failed runs arrive as gap markers (see useSpeedtestStats). Runs are sparse, and an area
// needs two neighboring values, so a dot marks each run.
function point(label: string, color: number | string, dataKey: DataPoint<SpeedtestStatsRecord>["dataKey"], order = 0) {
	return {
		label,
		color,
		dataKey,
		order,
		opacity: 0.2,
		dot: true,
	} satisfies DataPoint<SpeedtestStatsRecord>
}

export function SpeedtestBandwidthChart({ stats, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [point(t`Download`, 2, (record) => record.download, 0), point(t`Upload`, 5, (record) => record.upload, 1)],
		[t]
	)
	return (
		<ChartCard empty={empty} title={t`Bandwidth`} description={t`Download and upload speed`} grid={false} legend>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				legend
				tickFormatter={(value) => formatBandwidth(value, true)}
				contentFormatter={({ value }) => (typeof value === "number" ? formatBandwidth(value) : value)}
			/>
		</ChartCard>
	)
}

export function SpeedtestLatencyChart({ stats, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [point(t`Ping`, 1, (record) => record.ping, 0), point(t`Jitter`, 3, (record) => record.jitter, 1)],
		[t]
	)
	return (
		<ChartCard empty={empty} title={t`Latency`} description={t`Idle ping and jitter`} grid={false} legend>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				legend
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)} ms` : value)}
			/>
		</ChartCard>
	)
}

export function SpeedtestLoadedLatencyChart({ stats, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [
			point(t`Idle`, 1, (record) => record.ping, 0),
			point(t`Download`, 2, (record) => record.download_latency || null, 1),
			point(t`Upload`, 5, (record) => record.upload_latency || null, 2),
		],
		[t]
	)
	return (
		<ChartCard
			empty={empty}
			title={t`Loaded latency`}
			description={t`Latency while downloading and uploading (interquartile mean)`}
			grid={false}
			legend
		>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				legend
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)} ms` : value)}
			/>
		</ChartCard>
	)
}

export function SpeedtestLossChart({ stats, chartData, empty }: SpeedtestChartProps) {
	const { t } = useLingui()
	const dataPoints = useMemo(
		() => [
			// Servers that don't measure packet loss report -1.
			point(t({ message: "Loss", context: "Packet loss" }), "var(--destructive)", (record) =>
				record.loss >= 0 ? record.loss : null
			),
		],
		[t]
	)
	return (
		<ChartCard
			empty={empty}
			title={t({ message: "Loss", context: "Packet loss" })}
			description={t`Packet loss (%)`}
			grid={false}
		>
			<AreaChartDefault
				truncate
				chartData={chartData}
				customData={stats}
				dataPoints={dataPoints}
				domain={[0, "auto"]}
				tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)}%`}
				contentFormatter={({ value }) => (typeof value === "number" ? `${decimalString(value, 2)}%` : value)}
			/>
		</ChartCard>
	)
}
