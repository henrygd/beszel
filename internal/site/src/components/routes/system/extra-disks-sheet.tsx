import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState, useMemo } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import DiskChart from "@/components/charts/disk-chart"
import InodeChart from "@/components/charts/inode-chart"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard } from "../system"

type SheetType = "usage" | "io"

export default memo(function ExtraDisksSheet({
	chartData,
	dataEmpty,
	grid,
	maxValues,
	type,
}: {
	chartData: ChartData
	dataEmpty: boolean
	grid: boolean
	maxValues: boolean
	type: SheetType
}) {
	const [extraDisksOpen, setExtraDisksOpen] = useState(false)
	const userSettings = useStore($userSettings)
	const hasOpened = useRef(false)

	// Get list of extra filesystems from the latest data point
	const extraFsNames = useMemo(() => {
		const latestStats = chartData.systemStats.at(-1)?.stats
		if (!latestStats?.efs) {
			return []
		}
		return Object.keys(latestStats.efs)
	}, [chartData.systemStats])

	if (extraDisksOpen && !hasOpened.current) {
		hasOpened.current = true
	}

	if (!extraFsNames.length) {
		return null
	}

	const title = type === "usage" ? t`Extra Disk Usage` : t`Extra Disk I/O`
	const buttonTitle = type === "usage" ? t`View extra disk usage` : t`View extra disk I/O`

	return (
		<Sheet open={extraDisksOpen} onOpenChange={setExtraDisksOpen}>
			<DialogTitle className="sr-only">{title}</DialogTitle>
			<SheetTrigger asChild>
				<Button
					title={buttonTitle}
					variant="outline"
					size="icon"
					className="shrink-0 max-sm:absolute max-sm:top-3 max-sm:end-3"
				>
					<MoreHorizontalIcon />
				</Button>
			</SheetTrigger>
			{hasOpened.current && (
				<SheetContent aria-describedby={undefined} className="overflow-auto w-200 !max-w-full p-4 sm:p-6">
					<ChartTimeSelect className="w-[calc(100%-2em)]" agentVersion={chartData.agentVersion} />

					{/* Create charts for each extra filesystem */}
					{extraFsNames.map((extraFsName) => {
						const fsStats = chartData.systemStats.at(-1)?.stats.efs?.[extraFsName]
						const displayName = fsStats?.n || extraFsName

						// Create chart data for this specific filesystem
						const extraFsChartData = {
							...chartData,
							systemStats: chartData.systemStats.map((point) => {
								const efs = point.stats && point.stats.efs ? point.stats.efs[extraFsName] : undefined
								return {
									...point,
									stats: efs ? {
										...point.stats,
										du: efs?.du ?? 0,
										d: efs?.d ?? 0,
									} : point.stats,
								}
							}),
						}

						const hasData = extraFsChartData.systemStats.some(
							(point) => point.stats && Object.keys(point.stats).length > 0
						)

						if (!hasData) return null

						return (
							<div key={extraFsName} className="space-y-4 mb-6">
								{type === "usage" ? (
									<>
										{/* Disk Usage Chart */}
										<ChartCard
											empty={dataEmpty}
											grid={grid}
											title={`${displayName} - ${t`Disk Usage`}`}
											description={t`Disk space utilization`}
											className="min-h-auto"
										>
											<DiskChart
												chartData={extraFsChartData}
												dataKey="stats.du"
												diskSize={fsStats?.d ?? NaN}
												showLegend={userSettings.showChartLegend !== false}
											/>
										</ChartCard>

										{/* Inode Chart if available */}
										{fsStats?.it && (
											<ChartCard
												empty={dataEmpty}
												grid={grid}
												title={`${displayName} - ${t`Inodes`}`}
												description={t`Filesystem inode usage`}
												className="min-h-auto"
											>
												<InodeChart
													chartData={{
														...chartData,
														systemStats: chartData.systemStats.map((point) => {
															const efs = point.stats?.efs?.[extraFsName]
															return {
																...point,
																stats: {
																	...point.stats,
																	iu: efs?.iu ?? 0,
																	it: efs?.it ?? 0,
																	ip: efs?.ip ?? 0,
																},
															}
														}),
													}}
													showLegend={userSettings.showChartLegend !== false}
												/>
											</ChartCard>
										)}
									</>
								) : (
									<>
										{/* Disk I/O Chart */}
										<ChartCard
											empty={dataEmpty}
											grid={grid}
											title={`${displayName} - ${t`Disk I/O`}`}
											description={t`Read and write operations`}
											className="min-h-auto"
										>
											<AreaChartDefault
												chartData={chartData}
												maxToggled={maxValues}
												dataPoints={[
													{
														label: t`Write`,
														dataKey: ({ stats }: SystemStatsRecord) => {
															const efs = stats?.efs?.[extraFsName]
															return efs?.wb ?? (efs?.w ?? 0) * 1024 * 1024
														},
														color: 3,
														opacity: 0.3,
													},
													{
														label: t`Read`,
														dataKey: ({ stats }: SystemStatsRecord) => {
															const efs = stats?.efs?.[extraFsName]
															return efs?.rb ?? (efs?.r ?? 0) * 1024 * 1024
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
													const { value: convertedValue, unit } = formatBytes(
														value,
														true,
														userSettings.unitDisk,
														false
													)
													return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
												}}
												legend={userSettings.showChartLegend !== false}
											/>
										</ChartCard>
									</>
								)}
							</div>
						)
					})}
				</SheetContent>
			)}
		</Sheet>
	)
})
