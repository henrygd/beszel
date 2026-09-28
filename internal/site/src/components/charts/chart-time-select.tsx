import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ChevronLeftIcon, ChevronRightIcon, HistoryIcon } from "lucide-react"
import { memo, useState } from "react"
import { CustomRangeDialog } from "@/components/charts/custom-range-dialog"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { type ChartRange, canShiftBack, chartTimeDurations, coveringChartTime, shiftRange } from "@/lib/chart-range"
import { $chartRange, $chartTime } from "@/lib/stores"
import { chartTimeData, cn, compareSemVer, formatShortDate, parseSemVer } from "@/lib/utils"
import type { ChartTimes, SemVer } from "@/types"

const customRangeValue = "custom"

export default memo(function ChartTimeSelect({
	className,
	controlClassName,
	agentVersion,
	chartTimeStore = $chartTime,
	allowRealtime = true,
	allowRange = true,
}: {
	className?: string
	controlClassName?: string
	agentVersion: SemVer
	chartTimeStore?: typeof $chartTime
	allowRealtime?: boolean
	allowRange?: boolean
}) {
	const chartTime = useStore(chartTimeStore)
	const storedRange = useStore($chartRange)
	const range = allowRange ? storedRange : null
	const [customOpen, setCustomOpen] = useState(false)

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
		if (value === customRangeValue) {
			setCustomOpen(true)
			return
		}
		if (allowRange) {
			$chartRange.set(null)
		}
		chartTimeStore.set(value as ChartTimes)
	}

	function applyRange(newRange: ChartRange) {
		chartTimeStore.set(coveringChartTime(newRange.end - newRange.start))
		$chartRange.set(newRange)
		setCustomOpen(false)
	}

	const now = Date.now()
	const rangeLabel = range ? `${formatShortDate(range.start)} – ${formatShortDate(range.end)}` : undefined
	const showNav = allowRange && chartTime !== "1m"

	return (
		<div className={cn("flex items-center gap-2", className)}>
			{showNav && (
				<Button
					variant="outline"
					size="icon"
					className={cn("shrink-0", controlClassName)}
					aria-label={t`Previous period`}
					title={t`Previous period`}
					disabled={!canShiftBack(range, chartTime, now)}
					onClick={() => $chartRange.set(shiftRange(range, chartTime, -1))}
				>
					<ChevronLeftIcon className="size-4 rtl:rotate-180" />
				</Button>
			)}
			{/* An empty value shows the range label as the placeholder and keeps every item selectable */}
			<Select value={range ? "" : chartTime} onValueChange={onValueChange}>
				<SelectTrigger className={cn("relative ps-10 pe-5 min-w-0 flex-1", controlClassName)} title={rangeLabel}>
					<HistoryIcon className="h-4 w-4 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
					<SelectValue placeholder={rangeLabel} />
				</SelectTrigger>
				<SelectContent>
					{availableChartTimes.map(([value, { label }]) => (
						<SelectItem key={value} value={value}>
							{label()}
						</SelectItem>
					))}
					{allowRange && (
						<SelectItem value={customRangeValue}>
							<Trans>Custom range</Trans>…
						</SelectItem>
					)}
				</SelectContent>
			</Select>
			{showNav && (
				<Button
					variant="outline"
					size="icon"
					className={cn("shrink-0", controlClassName)}
					aria-label={t`Next period`}
					title={t`Next period`}
					disabled={!range}
					onClick={() => $chartRange.set(shiftRange(range, chartTime, 1))}
				>
					<ChevronRightIcon className="size-4 rtl:rotate-180" />
				</Button>
			)}
			{allowRange && (
				<CustomRangeDialog
					open={customOpen}
					onOpenChange={setCustomOpen}
					initialRange={range ?? { start: now - chartTimeDurations[chartTime], end: now }}
					onApply={applyRange}
				/>
			)}
		</div>
	)
})
