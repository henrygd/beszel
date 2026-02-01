import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import LoadAverageChart from "@/components/charts/load-average-chart"
import MemChart from "@/components/charts/mem-chart"
import SwapChart from "@/components/charts/swap-chart"
import TemperatureChart from "@/components/charts/temperature-chart"
import { batteryStateTranslations } from "@/lib/i18n"
import { $temperatureFilter } from "@/lib/stores"
import { cn, decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { SystemStatsRecord } from "@/types"
import { pinnedAxisDomain } from "../../../ui/chart"
import CpuCoresSheet from "../cpu-sheet"
import NetworkSheet from "../network-sheet"
import { ChartCard, FilterBar } from "./shared"
import type { CoreMetricsTabProps } from "./types"

export function CoreMetricsTab({
	chartData,
	grid,
	dataEmpty,
	maxValSelect,
	showMax,
	systemStats,
	temperatureChartRef,
	maxValues,
	userSettings,
}: CoreMetricsTabProps) {
	return (
		<div className="grid xl:grid-cols-2 gap-4">
			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`CPU Usage`}
				description={t`Average system-wide CPU utilization`}
				cornerEl={
					<div className="flex gap-2">
						{maxValSelect}
						<CpuCoresSheet chartData={chartData} dataEmpty={dataEmpty} grid={grid} maxValues={maxValues} />
					</div>
				}
			>
				<AreaChartDefault
					chartData={chartData}
					maxToggled={maxValues}
					dataPoints={[
						{
							label: t`CPU Usage`,
							dataKey: ({ stats }) => (showMax ? stats?.cpum : stats?.cpu),
							color: 1,
							opacity: 0.4,
						},
					]}
					tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
					contentFormatter={({ value }) => `${decimalString(value)}%`}
					domain={pinnedAxisDomain()}
				/>
			</ChartCard>

			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`Memory Usage`}
				description={t`Precise utilization at the recorded time`}
				cornerEl={maxValSelect}
			>
				<MemChart chartData={chartData} showMax={showMax} />
			</ChartCard>

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
					maxToggled={maxValues}
					dataPoints={[
						{
							label: t`Sent`,
							dataKey(data: SystemStatsRecord) {
								if (showMax) {
									return data?.stats?.bm?.[0] ?? (data?.stats?.nsm ?? 0) * 1024 * 1024
								}
								return data?.stats?.b?.[0] ?? data?.stats?.ns * 1024 * 1024
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
								return data?.stats?.b?.[1] ?? data?.stats?.nr * 1024 * 1024
							},
							color: 2,
							opacity: 0.2,
						},
					].sort(() => (systemStats.at(-1)?.stats.b?.[1] ?? 0) - (systemStats.at(-1)?.stats.b?.[0] ?? 0))}
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

			{/* Swap chart */}
			{(systemStats.at(-1)?.stats.su ?? 0) > 0 && (
				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={t`Swap Usage`}
					description={t`Swap space used by the system`}
				>
					<SwapChart chartData={chartData} />
				</ChartCard>
			)}

			{/* Load Average chart */}
			{chartData.agentVersion?.minor >= 12 && (
				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={t`Load Average`}
					description={t`System load averages over time`}
					legend={true}
				>
					<LoadAverageChart chartData={chartData} />
				</ChartCard>
			)}

			{/* Temperature chart */}
			{systemStats.at(-1)?.stats.t && (
				<div
					ref={temperatureChartRef}
					className={cn("odd:last-of-type:col-span-full", { "col-span-full": !grid })}
				>
					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Temperature`}
						description={t`Temperatures of system sensors`}
						cornerEl={<FilterBar store={$temperatureFilter} />}
						legend={Object.keys(systemStats.at(-1)?.stats.t ?? {}).length < 12}
					>
						<TemperatureChart chartData={chartData} />
					</ChartCard>
				</div>
			)}

			{/* Battery chart */}
			{systemStats.at(-1)?.stats.bat && (
				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={t`Battery`}
					description={`${t({
						message: "Current state",
						comment: "Context: Battery state",
					})}: ${batteryStateTranslations[systemStats.at(-1)?.stats.bat?.[1] ?? 0]()}`}
				>
					<AreaChartDefault
						chartData={chartData}
						maxToggled={maxValues}
						dataPoints={[
							{
								label: t`Charge`,
								dataKey: ({ stats }) => stats?.bat?.[0],
								color: 1,
								opacity: 0.35,
							},
						]}
						domain={[0, 100]}
						tickFormatter={(val) => `${val}%`}
						contentFormatter={({ value }) => `${value}%`}
					/>
				</ChartCard>
			)}
		</div>
	)
}
