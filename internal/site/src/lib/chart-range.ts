import type { ChartTimes } from "@/types"

/** Custom chart window in ms timestamps */
export interface ChartRange {
	start: number
	end: number
}

const hour = 60 * 60 * 1000
const rangeSlack = 60 * 1000

/**
 * Stats tiers that can serve a custom range, finest first, with how long the hub keeps each one
 * (see records_deletion.go). "1m" is left out because realtime data is never stored.
 */
const rangeTiers: { chartTime: Exclude<ChartTimes, "1m">; retention: number }[] = [
	{ chartTime: "1h", retention: hour },
	{ chartTime: "12h", retention: 12 * hour },
	{ chartTime: "24h", retention: 24 * hour },
	{ chartTime: "1w", retention: 7 * 24 * hour },
	{ chartTime: "30d", retention: 30 * 24 * hour },
]

/** Oldest timestamp that still has stored stats */
export function oldestRangeStart(now = Date.now()) {
	return now - rangeTiers[rangeTiers.length - 1].retention
}

/**
 * Finest stats tier that still holds data for the whole range. Finer tiers are deleted after a
 * while, so a window from yesterday can only be drawn from 20 minute averages or coarser.
 */
export function getRangeChartTime(range: ChartRange, now = Date.now()): ChartTimes {
	for (const tier of rangeTiers) {
		// Range inputs only have minute precision, so "the last hour" starts up to a minute too early.
		if (range.start >= now - tier.retention - rangeSlack) {
			return tier.chartTime
		}
	}
	return "30d"
}

/** Clamp a range to the stored history and make sure it ends after it starts */
export function normalizeRange(range: ChartRange, now = Date.now()): ChartRange | null {
	const start = Math.max(range.start, oldestRangeStart(now))
	const end = Math.min(range.end, now)
	if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
		return null
	}
	return { start, end }
}

/**
 * Move a range back (-1) or forward (1) by its own length, staying within the stored history.
 * Returns null when the range can't move any further in that direction.
 */
export function shiftRange(range: ChartRange, direction: -1 | 1, now = Date.now()): ChartRange | null {
	const length = range.end - range.start
	if (direction === -1) {
		const start = Math.max(range.start - length, oldestRangeStart(now))
		if (start >= range.start) {
			return null
		}
		return { start, end: start + length }
	}
	const end = Math.min(range.end + length, now)
	if (end <= range.end) {
		return null
	}
	return { start: end - length, end }
}

/** Format a timestamp as a datetime-local input value (YYYY-MM-DDTHH:mm) in local time */
export function toDateTimeLocal(timestamp: number) {
	const d = new Date(timestamp)
	const pad = (n: number) => String(n).padStart(2, "0")
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
