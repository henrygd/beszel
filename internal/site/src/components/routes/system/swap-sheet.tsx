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

export default memo(function SwapSheet({ systemData }: { systemData: SystemData }) {
	const { chartData, grid, dataEmpty } = systemData
	const [open, setOpen] = useState(false)
	const hasOpened = useRef(false)

	if (open && !hasOpened.current) {
		hasOpened.current = true
	}

	const bytesFormatter = (value: number) => {
		const { value: v, unit } = formatBytes(value, true, Unit.Bytes, false)
		return `${decimalString(v, v >= 100 ? 1 : 2)} ${unit}`
	}
	const bytesTickFormatter = (value: number) => {
		const { value: v, unit } = formatBytes(value, true, Unit.Bytes, false)
		return `${toFixedFloat(v, value >= 10 ? 0 : 1)} ${unit}`
	}

	return (
		<Sheet open={open} onOpenChange={setOpen}>
			<DialogTitle className="sr-only">{t`Swap I/O`}</DialogTitle>
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
						title={t`Swap I/O`}
						description={t`Rate of pages being swapped in and out of physical memory`}
						className="min-h-auto"
						legend={true}
					>
						<AreaChartDefault
							chartData={chartData}
							domain={[0, (dataMax: number) => Math.max(dataMax, 1)]}
							tickFormatter={bytesTickFormatter}
							contentFormatter={({ value }) => bytesFormatter(value)}
							showTotal={true}
							legend={true}
							dataPoints={[
								{
									label: t`Swap Out`,
									dataKey: ({ stats }) => stats?.so ?? 0,
									color: 3,
									opacity: 0.4,
									stackId: 0,
								},
								{
									label: t`Swap In`,
									dataKey: ({ stats }) => stats?.si ?? 0,
									color: 2,
									opacity: 0.4,
									stackId: 0,
								},
							]}
						/>
					</ChartCard>
				</SheetContent>
			)}
		</Sheet>
	)
})
