import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { type ChartRange, normalizeRange, oldestRangeStart, type TimeWindow, toDateTimeLocal } from "@/lib/chart-range"
import { chartTimeData } from "@/lib/utils"

export function CustomRangeDialog({
	open,
	onOpenChange,
	initialRange,
	onApply,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	initialRange: TimeWindow
	onApply: (range: ChartRange) => void
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-md">
				{/* mounted only while open so the inputs start from the current range each time */}
				{open && <CustomRangeForm initialRange={initialRange} onApply={onApply} onCancel={() => onOpenChange(false)} />}
			</DialogContent>
		</Dialog>
	)
}

function CustomRangeForm({
	initialRange,
	onApply,
	onCancel,
}: {
	initialRange: TimeWindow
	onApply: (range: ChartRange) => void
	onCancel: () => void
}) {
	const { t } = useLingui()
	const [now] = useState(Date.now)
	const [start, setStart] = useState(() => toDateTimeLocal(initialRange.start))
	const [end, setEnd] = useState(() => toDateTimeLocal(initialRange.end))

	const range = normalizeRange({ start: new Date(start).getTime(), end: new Date(end).getTime() }, now)
	const minutes = range ? chartTimeData[range.chartTime].expectedInterval / 60_000 : 0
	const hours = minutes / 60

	return (
		<form
			className="grid gap-4"
			onSubmit={(e) => {
				e.preventDefault()
				if (range) {
					onApply(range)
				}
			}}
		>
			<DialogHeader>
				<DialogTitle>
					<Trans>Custom range</Trans>
				</DialogTitle>
				<DialogDescription>
					<Trans>Stats are kept for 30 days, and older data is stored at a lower resolution.</Trans>
				</DialogDescription>
			</DialogHeader>
			<div className="grid gap-2">
				<Label htmlFor="range-start">
					<Trans>Start Time</Trans>
				</Label>
				<Input
					id="range-start"
					type="datetime-local"
					value={start}
					onChange={(e) => setStart(e.target.value)}
					min={toDateTimeLocal(oldestRangeStart(now))}
					max={end}
					required
					className="tabular-nums tracking-tighter"
				/>
			</div>
			<div className="grid gap-2">
				<Label htmlFor="range-end">
					<Trans>End Time</Trans>
				</Label>
				<Input
					id="range-end"
					type="datetime-local"
					value={end}
					onChange={(e) => setEnd(e.target.value)}
					min={start}
					max={toDateTimeLocal(now)}
					required
					className="tabular-nums tracking-tighter"
				/>
			</div>
			<p className="text-sm text-muted-foreground" aria-live="polite">
				{!range
					? t`End time must be after start time.`
					: minutes < 60
						? plural(minutes, {
								one: "Charts will show one point every minute.",
								other: "Charts will show one point every # minutes.",
							})
						: plural(hours, {
								one: "Charts will show one point every hour.",
								other: "Charts will show one point every # hours.",
							})}
			</p>
			<DialogFooter>
				<Button type="button" variant="outline" onClick={onCancel}>
					<Trans>Cancel</Trans>
				</Button>
				<Button type="submit" disabled={!range}>
					<Trans>Apply</Trans>
				</Button>
			</DialogFooter>
		</form>
	)
}
