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
	if (!interfaces.length) return null
	const dataPoints = interfaces.map(([id, current]) => ({
		label: current.s ? `${id} (${current.s})` : id,
		color: wifiColor(id),
		dataKey: ({ stats }: SystemStatsRecord) => stats?.wifi?.[id]?.r,
	}))
	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Wi-Fi signal`}
			description={interfaces
				.map(
					([id, value]) =>
						`${id}${value.s ? ` (${value.s})` : ""}: ${value.r == null ? t`Unavailable` : `${value.r} dBm`}`,
				)
				.join(" · ")}
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
