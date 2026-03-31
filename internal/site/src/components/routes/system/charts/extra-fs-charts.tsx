import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, SelectAvgMax } from "../chart-card"
import { Unit } from "@/lib/enums"
import { pinnedAxisDomain } from "@/components/ui/chart"

export function ExtraFsCharts({
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
	const extraFs = systemStats.at(-1)?.stats.efs
	if (!extraFs || Object.keys(extraFs).length === 0) {
		return null
	}

	return (
		<div className="grid xl:grid-cols-2 gap-4">
			{Object.keys(extraFs).map((extraFsName) => {
				let diskSize = systemStats.at(-1)?.stats.efs?.[extraFsName].d ?? NaN
				// round to nearest GB
				if (diskSize >= 100) {
					diskSize = Math.round(diskSize)
				}
				return (
					<div key={extraFsName} className="contents">
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={`${extraFsName} ${t`Usage`}`}
							description={t`Disk usage of ${extraFsName}`}
						>
							<AreaChartDefault
								chartData={chartData}
								domain={[0, diskSize]}
								tickFormatter={(val) => {
									const { value, unit } = formatBytes(val * 1024, false, Unit.Bytes, true)
									return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
								}}
								contentFormatter={({ value }) => {
									const { value: convertedValue, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
									return `${decimalString(convertedValue)} ${unit}`
								}}
								dataPoints={[
									{
										label: t`Disk Usage`,
										color: 4,
										opacity: 0.4,
										dataKey: ({ stats }) => stats?.efs?.[extraFsName]?.du,
									},
								]}
							></AreaChartDefault>
						</ChartCard>
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={`${extraFsName} I/O`}
							description={t`Throughput of ${extraFsName}`}
							cornerEl={maxValSelect}
						>
							<AreaChartDefault
								chartData={chartData}
								showTotal={true}
								dataPoints={[
									{
										label: t`Write`,
										dataKey: ({ stats }) => {
											if (showMax) {
												return stats?.efs?.[extraFsName]?.wbm || (stats?.efs?.[extraFsName]?.wm ?? 0) * 1024 * 1024
											}
											return stats?.efs?.[extraFsName]?.wb || (stats?.efs?.[extraFsName]?.w ?? 0) * 1024 * 1024
										},
										color: 3,
										opacity: 0.3,
									},
									{
										label: t`Read`,
										dataKey: ({ stats }) => {
											if (showMax) {
												return stats?.efs?.[extraFsName]?.rbm ?? (stats?.efs?.[extraFsName]?.rm ?? 0) * 1024 * 1024
											}
											return stats?.efs?.[extraFsName]?.rb ?? (stats?.efs?.[extraFsName]?.r ?? 0) * 1024 * 1024
										},
										color: 1,
										opacity: 0.3,
									},
								]}
								maxToggled={showMax}
								tickFormatter={(val) => {
									const { value, unit } = formatBytes(val, true, userSettings.unitDisk, false)
									return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
								}}
								contentFormatter={({ value }) => {
									const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, false)
									return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
								}}
							/>
						</ChartCard>
						{systemStats.some((r) => r.stats?.efs?.[extraFsName]?.diur !== undefined) && (
							<ChartCard
								empty={dataEmpty}
								grid={grid}
								title={`${extraFsName} ${t`I/O Utilization`}`}
								description={t`Percentage of time ${extraFsName} disk was busy with reads/writes`}
							>
								<AreaChartDefault
									chartData={chartData}
									domain={pinnedAxisDomain()}
									tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
									contentFormatter={({ value }) => `${decimalString(value)}%`}
									showTotal={true}
									dataPoints={[
										{
											label: t`Write`,
											dataKey: ({ stats }: SystemStatsRecord) => stats?.efs?.[extraFsName]?.diuw,
											color: 3,
											opacity: 0.3,
										},
										{
											label: t`Read`,
											dataKey: ({ stats }: SystemStatsRecord) => stats?.efs?.[extraFsName]?.diur,
											color: 1,
											opacity: 0.3,
										},
									]}
								/>
							</ChartCard>
						)}
					</div>
				)
			})}
		</div>
	)
}
