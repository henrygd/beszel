import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import LineChartDefault, { isolatedDot } from "@/components/charts/line-chart"
import { customMetricCharts, customMetricLegend, customMetricLines, formatCustomMetric } from "@/lib/custom-metrics"
import { $userSettings } from "@/lib/stores"
import { formatBytes, formatTemperature } from "@/lib/utils"
import type { ChartData, SystemRecord } from "@/types"
import { ChartCard } from "../chart-card"

const converters = { formatBytes, formatTemperature }

/**
 * One chart per title the agent resolved: a source's chart setting, or one per metric.
 * Renders nothing without custom metrics. What each chart shows is worked out in
 * lib/custom-metrics, where it is tested.
 */
export function CustomMetricCharts({
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
	const settings = useStore($userSettings, { keys: ["unitTemp", "unitDisk"] })
	const charts = customMetricCharts(system, chartData.customData)
	if (!charts.length) return null
	return (
		<>
			{charts.map((chart) => {
				const lines = customMetricLines(system, chart.keys)
				const units = new Map(lines.map((line) => [line.label, line.unit]))
				const legend = customMetricLegend(chart)
				return (
					<ChartCard
						key={chart.title}
						empty={dataEmpty}
						grid={grid}
						legend={legend}
						title={chart.title}
						description={chart.description ?? t`Values reported by scripts and exporters on this system`}
					>
						<LineChartDefault
							chartData={chartData}
							customData={chartData.customData}
							legend={legend}
							domain={["auto", "auto"]}
							tickFormatter={(value) => formatCustomMetric(value, chart.unit ?? "", settings, converters)}
							contentFormatter={({ value }, label) =>
								formatCustomMetric(value, units.get(label) ?? "", settings, converters)
							}
							dataPoints={lines.map((line) => ({
								label: line.label,
								color: line.color,
								dot: isolatedDot,
								dataKey: line.value,
							}))}
						/>
					</ChartCard>
				)
			})}
		</>
	)
}
