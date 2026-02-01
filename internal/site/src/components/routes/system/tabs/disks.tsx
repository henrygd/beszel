import { t } from "@lingui/core/macro"
import { lazy } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import DiskChart from "@/components/charts/disk-chart"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn, compareSemVer, decimalString, formatBytes, parseSemVer, toFixedFloat } from "@/lib/utils"
import type { SystemStatsRecord } from "@/types"
import { ChartCard } from "./shared"
import type { DisksTabProps } from "./types"

const SmartTable = lazy(() => import("../smart-table"))

function LazySmartTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SmartTable systemId={systemId} />}
		</div>
	)
}

export function DisksTab({
	chartData,
	grid,
	dataEmpty,
	maxValSelect,
	showMax,
	systemStats,
	systemId,
	userSettings,
}: DisksTabProps) {
	return (
		<>
			<div className="grid xl:grid-cols-2 gap-4">
				{/* Root disk charts */}
				<ChartCard empty={dataEmpty} grid={grid} title={t`Disk Usage`} description={t`Usage of root partition`}>
					<DiskChart chartData={chartData} dataKey="stats.du" diskSize={systemStats.at(-1)?.stats.d ?? NaN} />
				</ChartCard>

				<ChartCard
					empty={dataEmpty}
					grid={grid}
					title={t`Disk I/O`}
					description={t`Throughput of root filesystem`}
					cornerEl={maxValSelect}
				>
					<AreaChartDefault
						chartData={chartData}
						maxToggled={showMax}
						dataPoints={[
							{
								label: t({ message: "Write", comment: "Disk write" }),
								dataKey: ({ stats }: SystemStatsRecord) => {
									if (showMax) {
										return stats?.dio?.[1] ?? (stats?.dwm ?? 0) * 1024 * 1024
									}
									return stats?.dio?.[1] ?? (stats?.dw ?? 0) * 1024 * 1024
								},
								color: 3,
								opacity: 0.3,
							},
							{
								label: t({ message: "Read", comment: "Disk read" }),
								dataKey: ({ stats }: SystemStatsRecord) => {
									if (showMax) {
										return stats?.diom?.[0] ?? (stats?.drm ?? 0) * 1024 * 1024
									}
									return stats?.dio?.[0] ?? (stats?.dr ?? 0) * 1024 * 1024
								},
								color: 1,
								opacity: 0.3,
							},
						]}
						tickFormatter={(val) => {
							const { value, unit } = formatBytes(val, true, userSettings.unitDisk, false)
							return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
						}}
						contentFormatter={({ value }) => {
							const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, false)
							return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
						}}
						showTotal={true}
					/>
				</ChartCard>

				{/* Extra filesystem charts */}
				{Object.keys(systemStats.at(-1)?.stats.efs ?? {}).map((extraFsName) => {
					return (
						<div key={extraFsName} className="contents">
							<ChartCard
								empty={dataEmpty}
								grid={grid}
								title={`${extraFsName} ${t`Usage`}`}
								description={t`Disk usage of ${extraFsName}`}
							>
								<DiskChart
									chartData={chartData}
									dataKey={({ stats }: SystemStatsRecord) => stats?.efs?.[extraFsName]?.du}
									diskSize={systemStats.at(-1)?.stats.efs?.[extraFsName].d ?? NaN}
								/>
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
													return (
														stats?.efs?.[extraFsName]?.rbm ?? (stats?.efs?.[extraFsName]?.rm ?? 0) * 1024 * 1024
													)
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
						</div>
					)
				})}
			</div>

			{/* S.M.A.R.T. Table */}
			{compareSemVer(chartData.agentVersion, parseSemVer("0.15.0")) >= 0 && (
				<div className="mt-4">
					<LazySmartTable systemId={systemId} />
				</div>
			)}
		</>
	)
}
