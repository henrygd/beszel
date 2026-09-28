import { Trans } from "@lingui/react/macro"
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
import { type ChartRange, formatDateTimeLocal, maxRangeAge, normalizeCustomRange } from "@/lib/chart-range"

export function CustomRangeDialog({
	open,
	onOpenChange,
	initialRange,
	onApply,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	initialRange: ChartRange
	onApply: (range: ChartRange) => void
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<CustomRangeForm initialRange={initialRange} onApply={onApply} onCancel={() => onOpenChange(false)} />
			</DialogContent>
		</Dialog>
	)
}

function CustomRangeForm({
	initialRange,
	onApply,
	onCancel,
}: {
	initialRange: ChartRange
	onApply: (range: ChartRange) => void
	onCancel: () => void
}) {
	const [start, setStart] = useState(() => formatDateTimeLocal(initialRange.start))
	const [end, setEnd] = useState(() => formatDateTimeLocal(initialRange.end))
	const now = Date.now()
	const range = normalizeCustomRange(new Date(start).getTime(), new Date(end).getTime(), now)
	const max = formatDateTimeLocal(now)

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
					<Trans>Data is kept for 30 days. Older periods are shown at lower resolution.</Trans>
				</DialogDescription>
			</DialogHeader>
			<div className="grid gap-4 sm:grid-cols-2">
				<div className="grid gap-2 min-w-0">
					<Label htmlFor="chart-range-start">
						<Trans>Start Time</Trans>
					</Label>
					<Input
						id="chart-range-start"
						type="datetime-local"
						value={start}
						min={formatDateTimeLocal(now - maxRangeAge)}
						max={end || max}
						onChange={(e) => setStart(e.target.value)}
						required
						className="tabular-nums tracking-tighter"
					/>
				</div>
				<div className="grid gap-2 min-w-0">
					<Label htmlFor="chart-range-end">
						<Trans>End Time</Trans>
					</Label>
					<Input
						id="chart-range-end"
						type="datetime-local"
						value={end}
						min={start}
						max={max}
						onChange={(e) => setEnd(e.target.value)}
						required
						className="tabular-nums tracking-tighter"
					/>
				</div>
			</div>
			<DialogFooter className="gap-2">
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
