import { t } from "@lingui/core/macro"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { useConnectionStatsDetailed, useConnectionStatsIPv6 } from "@/components/charts/hooks"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import type { ChartData } from "@/types"
import { ChartCard } from "../system"

export default memo(function ConnectionSheet({
	chartData,
	dataEmpty,
	grid,
}: {
	chartData: ChartData
	dataEmpty: boolean
	grid: boolean
}) {
	const [sheetOpen, setSheetOpen] = useState(false)
	const hasOpened = useRef(false)
	const connectionStats = useConnectionStatsDetailed()
	const connectionStatsIPv6 = useConnectionStatsIPv6()
	const latestStats = chartData.systemStats.at(-1)?.stats?.nc

	// Check if we have IPv6 data (any non-zero IPv6 connection count)
	const hasIPv6Data = latestStats?.["_total"]?.tt6 ?? 0 > 0

	if (sheetOpen && !hasOpened.current) {
		hasOpened.current = true
	}

	// Don't show button if no connection data is available
	if (!latestStats) {
		return null
	}

	return (
		<Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
			<DialogTitle className="sr-only">{t`TCP connection states over time`}</DialogTitle>
			<SheetTrigger asChild>
				<Button
					title={t`View TCP states`}
					variant="outline"
					size="icon"
					className="shrink-0"
				>
					<MoreHorizontalIcon />
				</Button>
			</SheetTrigger>
			{hasOpened.current && (
				<SheetContent aria-describedby={undefined} className="overflow-auto w-200 !max-w-full p-4 sm:p-6">
					<ChartTimeSelect className="w-[calc(100%-2em)] bg-card" agentVersion={chartData.agentVersion} />
					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`TCP Connection States (IPv4)`}
						description={t`All TCP IPv4 connection states over time`}
						legend={true}
						className="min-h-auto"
					>
						<AreaChartDefault
							chartData={chartData}
							dataPoints={[
								connectionStats.established,
								connectionStats.listening,
								connectionStats.timeWait,
								connectionStats.closeWait,
								connectionStats.finWait1,
								connectionStats.finWait2,
								connectionStats.synSent,
								connectionStats.synRecv,
								connectionStats.closing,
								connectionStats.lastAck,
							]}
							legend={true}
							tickFormatter={(val) => val.toLocaleString()}
							contentFormatter={({ value }) => value.toLocaleString()}
						/>
					</ChartCard>

					{hasIPv6Data && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={t`TCP Connection States (IPv6)`}
							description={t`All TCP IPv6 connection states over time`}
							legend={true}
							className="min-h-auto"
						>
							<AreaChartDefault
								chartData={chartData}
								dataPoints={[
									connectionStatsIPv6.established,
									connectionStatsIPv6.listening,
									connectionStatsIPv6.timeWait,
									connectionStatsIPv6.closeWait,
									connectionStatsIPv6.finWait1,
									connectionStatsIPv6.finWait2,
									connectionStatsIPv6.synSent,
									connectionStatsIPv6.synRecv,
									connectionStatsIPv6.closing,
									connectionStatsIPv6.lastAck,
								]}
								legend={true}
								tickFormatter={(val) => val.toLocaleString()}
								contentFormatter={({ value }) => value.toLocaleString()}
							/>
						</ChartCard>
					)}
				</SheetContent>
			)}
		</Sheet>
	)
})
