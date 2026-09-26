import { t } from "@lingui/core/macro"
import LineChartDefault from "@/components/charts/line-chart"
import { connectedWiFi, wifiColor } from "@/lib/wifi"
import type { ChartData, SystemRecord, SystemStatsRecord } from "@/types"
import { ChartCard } from "../chart-card"

export function WiFiChart({
	system,
	chartData,
	grid,
	dataEmpty,
}: {
	system: SystemRecord
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	const interfaces = connectedWiFi(system)
	// Associated interfaces may not report RSSI; without any readings the chart would never render.
	const hasSignal = interfaces.some(
		([id, wifi]) => wifi.r !== undefined || chartData.systemStats.some((record) => record.stats?.wf?.[id] !== undefined)
	)
	if (!hasSignal) return null
	const dataPoints = interfaces.map(([id, current]) => ({
		label: current.s ? `${id} (${current.s})` : id,
		color: wifiColor(id),
		dataKey: ({ stats }: SystemStatsRecord) => stats?.wf?.[id],
	}))
	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Wi-Fi signal`}
			description={t`Signal strength of connected Wi-Fi interfaces`}
		>
			<LineChartDefault
				chartData={chartData}
				dataPoints={dataPoints}
				domain={["auto", "auto"]}
				legend={true}
				tickFormatter={(value) => `${value} dBm`}
				contentFormatter={({ value }) => `${value} dBm`}
			/>
		</ChartCard>
	)
}
