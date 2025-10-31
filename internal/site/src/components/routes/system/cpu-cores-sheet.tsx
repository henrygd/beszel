import { t } from "@lingui/core/macro"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import { decimalString, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard } from "../system"

export default memo(function CpuCoresSheet({
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
	const [cpuCoresOpen, setCpuCoresOpen] = useState(false)
	const hasOpened = useRef(false)

	if (cpuCoresOpen && !hasOpened.current) {
		hasOpened.current = true
	}

	// Get list of CPU cores from the latest stats
	const cpuCoresData = chartData.systemStats.at(-1)?.stats?.cpuc ?? {}
	const coreNames = Object.keys(cpuCoresData).sort((a, b) => {
		const numA = Number.parseInt(a.replace("cpu", ""))
		const numB = Number.parseInt(b.replace("cpu", ""))
		return numA - numB
	})

	if (coreNames.length === 0) {
		return null
	}

	return (
		<Sheet open={cpuCoresOpen} onOpenChange={setCpuCoresOpen}>
			<DialogTitle className="sr-only">{t`Per-core CPU usage`}</DialogTitle>
			<SheetTrigger asChild>
				<Button
					title={t`View per-core CPU`}
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
					{coreNames.map((coreName) => (
						<ChartCard
							key={coreName}
							empty={dataEmpty}
							grid={grid}
							title={coreName.toUpperCase()}
							description={t`CPU usage breakdown for ${coreName}`}
							legend={true}
							className="min-h-auto"
						>
							<AreaChartDefault
								chartData={chartData}
								maxToggled={maxValues}
								legend={true}
								dataPoints={[
									{
										label: t`Total`,
										dataKey: ({ stats }: SystemStatsRecord) => {
											const core = stats?.cpuc?.[coreName]
											if (!core) return undefined
											// Sum all metrics: user + system + iowait + steal
											return core[0] + core[1] + core[2] + core[3]
										},
										color: 1,
										opacity: 0.4,
									},
									{
										label: t`User`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[coreName]?.[0],
										color: 2,
										opacity: 0.3,
									},
									{
										label: t`System`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[coreName]?.[1],
										color: 3,
										opacity: 0.3,
									},
									{
										label: t`IOWait`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[coreName]?.[2],
										color: 4,
										opacity: 0.3,
									},
									{
										label: t`Steal`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpuc?.[coreName]?.[3],
										color: 5,
										opacity: 0.3,
									},
								]}
								tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
								contentFormatter={({ value }) => `${decimalString(value)}%`}
							/>
						</ChartCard>
					))}
				</SheetContent>
			)}
		</Sheet>
	)
})
