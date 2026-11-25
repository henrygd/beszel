import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import LineChartDefault from "@/components/charts/line-chart"
import { fanControlChannels, fanControlColor, fanControlLevel } from "@/lib/fan-control"
import { $fanControlFilter } from "@/lib/stores"
import { decimalString, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, FilterBar } from "../chart-card"

export function FanControlChart({
	chartData,
	grid,
	dataEmpty,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	const filter = useStore($fanControlFilter)
	const channels = fanControlChannels(chartData.systemStats)
	if (!channels.length) return null
	const terms = filter.toLowerCase().split(" ").filter(Boolean)
	const dataPoints = channels.map((channel) => {
		const filtered = terms.length > 0 && !terms.some((term) => channel.toLowerCase().includes(term))
		return {
			label: channel,
			color: fanControlColor(channel),
			dataKey: (record: SystemStatsRecord) => fanControlLevel(record, channel),
			strokeOpacity: filtered ? 0.1 : 1,
			activeDot: !filtered,
		}
	})
	const legend = channels.length < 12
	const averaged = !["1m", "1h"].includes(chartData.chartTime)
	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Fan control`}
			description={averaged ? t`Average reported control level (%)` : t`Reported control level (%)`}
			cornerEl={<FilterBar store={$fanControlFilter} />}
			legend={legend}
		>
			<LineChartDefault
				chartData={chartData}
				dataPoints={dataPoints}
				domain={[0, 100]}
				interpolation="stepAfter"
				connectNulls={false}
				legend={legend}
				filter={filter}
				tickFormatter={(value) => `${toFixedFloat(value, 0)}%`}
				contentFormatter={({ value }) => `${decimalString(value, 2)}%`}
			/>
		</ChartCard>
	)
}
