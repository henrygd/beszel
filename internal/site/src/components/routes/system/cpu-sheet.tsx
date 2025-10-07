import { t } from "@lingui/core/macro"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState, useMemo } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import { decimalString } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard } from "../system"

export default memo(function CpuSheet({
	chartData,
	dataEmpty,
	grid,
	maxValues,
}: {
	chartData: ChartData
	dataEmpty: boolean
	grid: boolean
	maxValues: boolean
}) {
	const [cpuDetailsOpen, setCpuDetailsOpen] = useState(false)
	const hasOpened = useRef(false)

	// Get the list of CPU cores from the latest data point
	const cpuCores = useMemo(() => {
		const latestStats = chartData.systemStats.at(-1)?.stats
		if (!latestStats?.cpuc) {
			return []
		}
		return Object.keys(latestStats.cpuc).sort()
	}, [chartData.systemStats])

	if (cpuDetailsOpen && !hasOpened.current) {
		hasOpened.current = true
	}

	if (!cpuCores.length) {
		return null
	}

	return (
		<Sheet open={cpuDetailsOpen} onOpenChange={setCpuDetailsOpen}>
			<DialogTitle className="sr-only">{t`Per-core CPU metrics`}</DialogTitle>
			<SheetTrigger asChild>
				<Button
					title={t`View per-core details`}
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

					{/* Create a chart for each CPU core showing breakdown of user/system/iowait/steal */}
					{cpuCores.map((core) => {
						// Extract the core number from the core name (e.g., "cpu0" -> "0")
						const coreNumber = core.replace(/[^0-9]/g, '')
						return (
							<ChartCard
								key={core}
								empty={dataEmpty}
								grid={grid}
								title={t`CPU Core ${coreNumber}`}
								description={t`CPU time breakdown for this core`}
								legend={true}
								className="min-h-auto"
							>
							<AreaChartDefault
								chartData={chartData}
								maxToggled={maxValues}
								itemSorter={(a, b) => b.value - a.value}
								dataPoints={[
									{
										label: t`User`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[core]?.u ?? 0,
										color: 1,
										opacity: 0.4,
									},
									{
										label: t`System`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[core]?.s ?? 0,
										color: 2,
										opacity: 0.4,
									},
									{
										label: t`IO Wait`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[core]?.i ?? 0,
										color: 3,
										opacity: 0.4,
									},
									{
										label: t`Steal`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[core]?.st ?? 0,
										color: 4,
										opacity: 0.4,
									},
								]}
								legend={true}
								domain={[0, 100]}
								tickFormatter={(val) => `${val}%`}
								contentFormatter={({ value }) => `${decimalString(value, 1)}%`}
							/>
						</ChartCard>
						)
					})}
				</SheetContent>
			)}
		</Sheet>
	)
})
