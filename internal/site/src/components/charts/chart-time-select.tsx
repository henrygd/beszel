import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ChevronLeftIcon, ChevronRightIcon, HistoryIcon, PencilIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectSeparator, SelectTrigger, SelectValue } from "@/components/ui/select"
import { type ChartRange, shiftRange } from "@/lib/chart-range"
import { $chartRange, $chartTime } from "@/lib/stores"
import { chartTimeData, cn, compareSemVer, parseSemVer } from "@/lib/utils"
import type { ChartTimes, SemVer } from "@/types"
import { memo, useState } from "react"
import { CustomRangeDialog, formatRange } from "./custom-range-dialog"

const customValue = "custom"

export default memo(function ChartTimeSelect({
	className,
	triggerClassName,
	agentVersion,
	chartTimeStore = $chartTime,
	allowRealtime = true,
}: {
	/** Applied to the wrapper that also holds the custom range buttons */
	className?: string
	triggerClassName?: string
	agentVersion: SemVer
	chartTimeStore?: typeof $chartTime
	allowRealtime?: boolean
}) {
	const { t } = useLingui()
	const chartTime = useStore(chartTimeStore)
	const storedRange = useStore($chartRange)
	const [dialogOpen, setDialogOpen] = useState(false)
	// custom ranges are only wired up for the system page charts, which use the global store
	const allowCustom = chartTimeStore === $chartTime
	const range = allowCustom ? storedRange : null

	// remove chart times that are not supported by the system agent version
	const availableChartTimes = Object.entries(chartTimeData).filter(([value, { minVersion }]) => {
		if (value === "1m" && !allowRealtime) {
			return false
		}
		if (!minVersion) {
			return true
		}
		return compareSemVer(agentVersion, parseSemVer(minVersion)) >= 0
	})

	function onValueChange(value: string) {
		if (value === customValue) {
			setDialogOpen(true)
			return
		}
		$chartRange.set(null)
		chartTimeStore.set(value as ChartTimes)
	}

	function getInitialRange(): ChartRange {
		if (range) {
			return range
		}
		const end = Date.now()
		const preset = chartTime === "1m" ? "1h" : chartTime
		return { start: chartTimeData[preset].getOffset(new Date(end)).getTime(), end }
	}

	const prevRange = range && shiftRange(range, -1)
	const nextRange = range && shiftRange(range, 1)

	return (
		<div className={cn("flex items-center gap-2", className)}>
			{range && (
				<Button
					variant="outline"
					size="icon"
					className="shrink-0"
					aria-label={t`Previous period`}
					disabled={!prevRange}
					onClick={() => prevRange && $chartRange.set(prevRange)}
				>
					<ChevronLeftIcon className="size-4 rtl:rotate-180" />
				</Button>
			)}
			<Select defaultValue="1h" value={range ? customValue : chartTime} onValueChange={onValueChange}>
				<SelectTrigger className={cn("relative ps-10 pe-5 flex-1 min-w-0", triggerClassName, range && "xl:w-auto")}>
					<HistoryIcon className="h-4 w-4 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					{availableChartTimes.map(([value, { label }]) => (
						<SelectItem key={value} value={value}>
							{label()}
						</SelectItem>
					))}
					{allowCustom && (
						<>
							<SelectSeparator />
							<SelectItem value={customValue}>{range ? formatRange(range) : <Trans>Custom range</Trans>}</SelectItem>
						</>
					)}
				</SelectContent>
			</Select>
			{range && (
				<>
					<Button
						variant="outline"
						size="icon"
						className="shrink-0"
						aria-label={t`Next period`}
						disabled={!nextRange}
						onClick={() => nextRange && $chartRange.set(nextRange)}
					>
						<ChevronRightIcon className="size-4 rtl:rotate-180" />
					</Button>
					<Button
						variant="outline"
						size="icon"
						className="shrink-0"
						aria-label={t`Edit custom range`}
						onClick={() => setDialogOpen(true)}
					>
						<PencilIcon className="size-4" />
					</Button>
				</>
			)}
			{allowCustom && (
				<CustomRangeDialog
					open={dialogOpen}
					onOpenChange={setDialogOpen}
					initialRange={getInitialRange()}
					onApply={(newRange) => {
						$chartRange.set(newRange)
						setDialogOpen(false)
					}}
				/>
			)}
		</div>
	)
})
