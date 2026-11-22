type TooltipTick = { value: number | null }

/**
 * Finds the tooltip index with the timestamp closest to the hovered one. Charts don't share
 * one data array (container / monitor data, frozen off-screen data), so matching by index or
 * exact value would show different moments in time. Gap markers (null timestamps) are skipped.
 */
export function syncByNearestTime(ticks: TooltipTick[], data: { activeLabel?: number | null }) {
	const target = data.activeLabel
	if (target == null) {
		return -1
	}
	let first: number | undefined
	let last: number | undefined
	let index = -1
	let minDiff = Infinity
	for (let i = 0; i < ticks.length; i++) {
		const value = ticks[i].value
		if (value == null) {
			continue
		}
		first ??= value
		last = value
		const diff = Math.abs(value - target)
		if (diff < minDiff) {
			minDiff = diff
			index = i
		}
	}
	// don't show a tooltip when hovering outside this chart's data range
	if (first === undefined || last === undefined || target < first || target > last) {
		return -1
	}
	return index
}
