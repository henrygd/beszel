import { t } from "@lingui/core/macro"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useMemo, useRef, useState } from "react"
import AreaChartDefault, { DataPoint } from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import { compareSemVer, decimalString, parseSemVer, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard } from "../system"

const minAgentVersion = parseSemVer("0.15.3")

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

	const supportsBreakdown = useMemo(() => compareSemVer(chartData.agentVersion, minAgentVersion) >= 0, [chartData.agentVersion])

	if (!supportsBreakdown) {
		return null
	}

	if (cpuCoresOpen && !hasOpened.current) {
		hasOpened.current = true
	}

	// Latest stats snapshot
	const latest = chartData.systemStats.at(-1)?.stats
	const cpus = latest?.cpus ?? []
	const numCores = cpus.length
	const hasBreakdown = (latest?.cpub?.length ?? 0) > 0

	const breakdownDataPoints = [
		{
			label: t`Other`,
			dataKey: ({ stats }: SystemStatsRecord) => {
				const total = stats?.cpub?.reduce((acc, curr) => acc + curr, 0) ?? 0
				return total > 0 ? 100 - total : null
			},
			color: `hsl(80, 65%, 52%)`,
			opacity: 0.4,
			stackId: "a"
		},
		{
			label: "Steal",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.cpub?.[3],
			color: 5,
			opacity: 0.4,
			stackId: "a"
		},
		{
			label: "Idle",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.cpub?.[4],
			color: 2,
			opacity: 0.4,
			stackId: "a"
		},
		{
			label: "IOWait",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.cpub?.[2],
			color: 4,
			opacity: 0.4,
			stackId: "a"
		},
		{
			label: "User",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.cpub?.[0],
			color: 1,
			opacity: 0.4,
			stackId: "a"
		},
		{
			label: "System",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.cpub?.[1],
			color: 3,
			opacity: 0.4,
			stackId: "a"
		},
	] as DataPoint[]


	return (
		<Sheet open={cpuCoresOpen} onOpenChange={setCpuCoresOpen}>
			<DialogTitle className="sr-only">{t`CPU Usage`}</DialogTitle>
			<SheetTrigger asChild>
				<Button
					title={t`View more`}
					variant="outline"
					size="icon"
					className="shrink-0 max-sm:absolute max-sm:top-3 max-sm:end-3"
				>
					<MoreHorizontalIcon />
				</Button>
			</SheetTrigger>
			{hasOpened.current && (
				<SheetContent aria-describedby={undefined} className="overflow-auto w-200 !max-w-full p-4 sm:p-6">
					<ChartTimeSelect className="w-[calc(100%-2em)] bg-card" agentVersion={chartData.agentVersion} />
					{hasBreakdown && (
						<ChartCard
							key="cpu-breakdown"
							empty={dataEmpty}
							grid={grid}
							title={t`CPU Time Breakdown`}
							description={t`Percentage of time spent in each state`}
							legend={true}
							className="min-h-auto"
						>
							<AreaChartDefault
								reverseStackOrder={true}
								chartData={chartData}
								maxToggled={maxValues}
								legend={true}
								dataPoints={breakdownDataPoints}
								tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
								contentFormatter={({ value }) => `${decimalString(value)}%`}
								itemSorter={() => 1}
								domain={[0, 100]}
							/>
						</ChartCard>
					)}

					{numCores > 0 && (
						<ChartCard
							key="cpu-cores-all"
							empty={dataEmpty}
							grid={grid}
							title={t`CPU Cores`}
							legend={numCores < 10}
							description={t`Per-core average utilization`}
							className="min-h-auto"
						>
							<AreaChartDefault
								hideYAxis={true}
								chartData={chartData}
								maxToggled={maxValues}
								legend={numCores < 10}
								dataPoints={Array.from({ length: numCores }).map((_, i) => ({
									label: `CPU ${i}`,
									dataKey: ({ stats }: SystemStatsRecord) => stats?.cpus?.[i] ?? 1 / (stats?.cpus?.length ?? 1),
									color: `hsl(${226 + (((i * 360) / Math.max(1, numCores)) % 360)}, var(--chart-saturation), var(--chart-lightness))`,
									opacity: 0.35,
									stackId: "a"
								}))}
								tickFormatter={(val) => `${val}%`}
								contentFormatter={({ value }) => `${value}%`}
								reverseStackOrder={true}
								itemSorter={() => 1}
							/>
						</ChartCard>
					)}

					{Array.from({ length: numCores }).map((_, i) => (
						<ChartCard
							key={`cpu-core-${i}`}
							empty={dataEmpty}
							grid={grid}
							title={`CPU ${i}`}
							description={t`Per-core average utilization`}
							legend={false}
							className="min-h-auto"
						>
							<AreaChartDefault
								chartData={chartData}
								maxToggled={maxValues}
								legend={false}
								dataPoints={[
									{
										label: t`Usage`,
										dataKey: ({ stats }: SystemStatsRecord) => stats?.cpus?.[i],
										color: `hsl(${226 + (((i * 360) / Math.max(1, numCores)) % 360)}, 65%, 52%)`,
										opacity: 0.35,
									},
								]}
								tickFormatter={(val) => `${val}%`}
								contentFormatter={({ value }) => `${value}%`}
							/>
						</ChartCard>
					))}
				</SheetContent>
			)}
		</Sheet>
	)
})
