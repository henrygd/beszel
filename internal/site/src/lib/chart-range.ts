import type { ChartTimes } from "@/types"

/** Fixed window of time for historical charts, in ms timestamps */
export interface ChartRange {
	start: number
	end: number
}

const HOUR = 3_600_000
const DAY = 24 * HOUR

/** Duration of each chart time. Each non-realtime duration also matches the retention of its stats tier. */
export const chartTimeDurations: Record<ChartTimes, number> = {
	"1m": 60_000,
	"1h": HOUR,
	"12h": 12 * HOUR,
	"24h": DAY,
	"1w": 7 * DAY,
	"30d": 30 * DAY,
}

/** Oldest data kept by the hub (retention of the 480m tier) */
export const maxRangeAge = chartTimeDurations["30d"]

const tieredChartTimes: ChartTimes[] = ["1h", "12h", "24h", "1w", "30d"]

/** Smallest non-realtime chart time that is at least `ms` long */
export function coveringChartTime(ms: number): ChartTimes {
	return tieredChartTimes.find((chartTime) => chartTimeDurations[chartTime] >= ms) ?? "30d"
}

/** Chart time whose stats tier still holds records for the start of the range */
export function rangeDataChartTime(range: ChartRange, now = Date.now()): ChartTimes {
	return coveringChartTime(now - range.start)
}

function windowOf(range: ChartRange | null, chartTime: ChartTimes, now: number): ChartRange {
	return range ?? { start: now - chartTimeDurations[chartTime], end: now }
}

/** Move the displayed window by its own length. Returns null when it lands back on the live preset. */
export function shiftRange(
	range: ChartRange | null,
	chartTime: ChartTimes,
	direction: -1 | 1,
	now = Date.now()
): ChartRange | null {
	const { start, end } = windowOf(range, chartTime, now)
	const span = end - start
	const next = { start: start + direction * span, end: end + direction * span }
	// The range was anchored when it was set, so "now" has drifted forward since. Snap to the present
	// once the next window is closer to it than to the previous one.
	if (next.end > now - span / 2) {
		return span === chartTimeDurations[chartTime] ? null : { start: now - span, end: now }
	}
	const oldest = now - maxRangeAge
	if (next.start < oldest) {
		return { start: oldest, end: oldest + span }
	}
	return next
}

export function canShiftBack(range: ChartRange | null, chartTime: ChartTimes, now = Date.now()): boolean {
	return windowOf(range, chartTime, now).start > now - maxRangeAge
}

/** Local `YYYY-MM-DDTHH:mm` value for a datetime-local input */
export function formatDateTimeLocal(date: Date | number): string {
	const d = new Date(date)
	const pad = (n: number) => String(n).padStart(2, "0")
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** Clamp a user-entered range to the retained data, or null if nothing valid is left */
export function normalizeCustomRange(start: number, end: number, now = Date.now()): ChartRange | null {
	const range = { start: Math.max(start, now - maxRangeAge), end: Math.min(end, now) }
	if (Number.isNaN(range.start) || Number.isNaN(range.end) || range.start >= range.end) {
		return null
	}
	return range
}
