import { t } from "@lingui/core/macro"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import { Unit } from "@/lib/enums"
import { ChartCard } from "./chart-card"
import type { SystemData } from "./use-system-data"

// OOM kills are integers — round max up to next multiple of 4 so Recharts picks integer ticks
const oomDomain: [number, (max: number) => number] = [0, (dataMax: number) => Math.max(Math.ceil(dataMax / 4) * 4, 4)]

export default memo(function MemorySheet({ systemData }: { systemData: SystemData }) {
	const { chartData, grid, dataEmpty } = systemData
	const [open, setOpen] = useState(false)
	const hasOpened = useRef(false)

	if (open && !hasOpened.current) {
		hasOpened.current = true
	}

	const hasSlabData = chartData.systemStats?.some((r) => (r.stats?.msr ?? 0) + (r.stats?.msu ?? 0) > 0)
	if (!hasSlabData) {
		return null
	}

	const memFormatter = (value: number) => {
		const { value: v, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
		return `${decimalString(v, v >= 100 ? 1 : 2)} ${unit}`
	}
	const memTickFormatter = (value: number) => {
		const { value: v, unit } = formatBytes(value * 1024, false, Unit.Bytes, true)
		return `${toFixedFloat(v, value >= 10 ? 0 : 1)} ${unit}`
	}

	return (
		<Sheet open={open} onOpenChange={setOpen}>
			<DialogTitle className="sr-only">{t`Memory Details`}</DialogTitle>
			<SheetTrigger asChild>
				<Button
					title={t`View more`}
					variant="outline"
					size="icon"
					className="shrink-0 max-sm:absolute max-sm:top-0 max-sm:end-0"
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
						title={t`Kernel Slab`}
						description={t`Memory used by the kernel slab allocator`}
						className="min-h-auto"
						legend={true}
					>
						<AreaChartDefault
							chartData={chartData}
							tickFormatter={memTickFormatter}
							contentFormatter={({ value }) => memFormatter(value)}
							showTotal={true}
							legend={true}
							dataPoints={[
								{
									label: t`Reclaimable`,
									dataKey: ({ stats }) => stats?.msr ?? null,
									color: 1,
									opacity: 0.4,
									stackId: 0,
								},
								{
									label: t`Unreclaimable`,
									dataKey: ({ stats }) => stats?.msu ?? null,
									color: 5,
									opacity: 0.4,
									stackId: 0,
								},
							]}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Memory Pressure`}
						description={t`% of time tasks were stalled waiting for memory`}
						className="min-h-auto"
						legend={true}
					>
						<AreaChartDefault
							chartData={chartData}
							tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
							contentFormatter={({ value }) => `${decimalString(value, 2)}%`}
							legend={true}
							dataPoints={[
								{
									label: t`Some`,
									dataKey: ({ stats }) => stats?.mpr?.[0] ?? null,
									color: 3,
									opacity: 0.3,
								},
								{
									label: t`Full`,
									dataKey: ({ stats }) => stats?.mpr?.[1] ?? null,
									color: 5,
									opacity: 0.4,
								},
							]}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Major Page Faults`}
						description={t`Page faults that required reading from disk`}
						className="min-h-auto"
					>
						<AreaChartDefault
							chartData={chartData}
							tickFormatter={(val) => `${toFixedFloat(val, 2)}/s`}
							contentFormatter={({ value }) => `${decimalString(value, value >= 100 ? 0 : 2)}/s`}
							dataPoints={[
								{
									label: t`Major Faults`,
									dataKey: ({ stats }) => stats?.mpf ?? 0,
									color: 2,
									opacity: 0.4,
								},
							]}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`OOM Kill Events`}
						description={t`Number of processes killed by the OOM killer per interval`}
						className="min-h-auto"
					>
						<AreaChartDefault
							chartData={chartData}
							domain={oomDomain}
							tickFormatter={(val) => `${toFixedFloat(val, 0)}`}
							contentFormatter={({ value }) => String(Math.round(value))}
							dataPoints={[
								{
									label: t`OOM Kills`,
									dataKey: ({ stats }) => stats?.moom ?? 0,
									color: 5,
									opacity: 0.5,
								},
							]}
						/>
					</ChartCard>
				</SheetContent>
			)}
		</Sheet>
	)
})
