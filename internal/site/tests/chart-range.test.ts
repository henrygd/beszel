import { describe, expect, test } from "bun:test"
import {
	canShiftBack,
	coveringChartTime,
	normalizeCustomRange,
	rangeDataChartTime,
	shiftRange,
	formatDateTimeLocal,
} from "../src/lib/chart-range"

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR
const now = Date.UTC(2026, 8, 26, 12)

describe("coveringChartTime", () => {
	test("returns the smallest preset at least as long as the duration", () => {
		expect(coveringChartTime(30 * MIN)).toBe("1h")
		expect(coveringChartTime(HOUR)).toBe("1h")
		expect(coveringChartTime(HOUR + 1)).toBe("12h")
		expect(coveringChartTime(20 * HOUR)).toBe("24h")
		expect(coveringChartTime(3 * DAY)).toBe("1w")
		expect(coveringChartTime(20 * DAY)).toBe("30d")
	})

	test("caps at 30 days", () => {
		expect(coveringChartTime(40 * DAY)).toBe("30d")
	})
})

describe("rangeDataChartTime", () => {
	test("uses a tier whose retention still covers the range start", () => {
		// 1m records are only kept for an hour, so an hour shifted back needs the 10m tier
		expect(rangeDataChartTime({ start: now - 2 * HOUR, end: now - HOUR }, now)).toBe("12h")
		expect(rangeDataChartTime({ start: now - 36 * HOUR, end: now - 24 * HOUR }, now)).toBe("1w")
		expect(rangeDataChartTime({ start: now - 30 * MIN, end: now - 10 * MIN }, now)).toBe("1h")
	})
})

describe("shiftRange", () => {
	test("shifting a live preset back moves one full period earlier", () => {
		expect(shiftRange(null, "12h", -1, now)).toEqual({ start: now - 24 * HOUR, end: now - 12 * HOUR })
	})

	test("shifting a range back by its own span", () => {
		const range = { start: now - 8 * HOUR, end: now - 5 * HOUR }
		expect(shiftRange(range, "12h", -1, now)).toEqual({ start: now - 11 * HOUR, end: now - 8 * HOUR })
	})

	test("shifting a preset window forward to now returns to live", () => {
		expect(shiftRange({ start: now - 24 * HOUR, end: now - 12 * HOUR }, "12h", 1, now)).toBeNull()
	})

	test("shifting forward returns to live even after time has passed since the range was set", () => {
		const setAt = now - 18_000
		expect(shiftRange({ start: setAt - 24 * HOUR, end: setAt - 12 * HOUR }, "12h", 1, now)).toBeNull()
	})

	test("shifting forward stays historical when the next window is mostly in the past", () => {
		const setAt = now - 7 * HOUR
		expect(shiftRange({ start: setAt - 24 * HOUR, end: setAt - 12 * HOUR }, "12h", 1, now)).toEqual({
			start: setAt - 12 * HOUR,
			end: setAt,
		})
	})

	test("shifting a custom range forward past now clamps it to end at now", () => {
		const range = { start: now - 4 * HOUR, end: now - HOUR }
		expect(shiftRange(range, "12h", 1, now)).toEqual({ start: now - 3 * HOUR, end: now })
	})

	test("shifting back past the 30 day retention clamps to the oldest window", () => {
		const range = { start: now - 28 * DAY, end: now - 21 * DAY }
		expect(shiftRange(range, "1w", -1, now)).toEqual({ start: now - 30 * DAY, end: now - 23 * DAY })
	})
})

describe("canShiftBack", () => {
	test("allowed while the window starts inside the retention period", () => {
		expect(canShiftBack(null, "1w", now)).toBe(true)
		expect(canShiftBack({ start: now - 29 * DAY, end: now - 22 * DAY }, "1w", now)).toBe(true)
	})

	test("not allowed once the window reaches the oldest retained data", () => {
		expect(canShiftBack(null, "30d", now)).toBe(false)
		expect(canShiftBack({ start: now - 30 * DAY, end: now - 23 * DAY }, "1w", now)).toBe(false)
	})
})

describe("normalizeCustomRange", () => {
	test("keeps a valid range unchanged", () => {
		expect(normalizeCustomRange(now - 5 * HOUR, now - 2 * HOUR, now)).toEqual({
			start: now - 5 * HOUR,
			end: now - 2 * HOUR,
		})
	})

	test("rejects a range whose start is not before its end", () => {
		expect(normalizeCustomRange(now - HOUR, now - HOUR, now)).toBeNull()
		expect(normalizeCustomRange(now - HOUR, now - 2 * HOUR, now)).toBeNull()
	})

	test("clamps the end to now", () => {
		expect(normalizeCustomRange(now - HOUR, now + HOUR, now)).toEqual({ start: now - HOUR, end: now })
	})

	test("clamps the start to the 30 day retention", () => {
		expect(normalizeCustomRange(now - 40 * DAY, now - 20 * DAY, now)).toEqual({
			start: now - 30 * DAY,
			end: now - 20 * DAY,
		})
	})

	test("rejects a range entirely older than the retention", () => {
		expect(normalizeCustomRange(now - 40 * DAY, now - 35 * DAY, now)).toBeNull()
	})

	test("rejects invalid dates", () => {
		expect(normalizeCustomRange(Number.NaN, now, now)).toBeNull()
	})
})

describe("formatDateTimeLocal", () => {
	test("formats a timestamp as a local datetime-local input value", () => {
		expect(formatDateTimeLocal(new Date(2026, 0, 5, 9, 7, 45).getTime())).toBe("2026-01-05T09:07")
	})

	test("accepts a Date", () => {
		expect(formatDateTimeLocal(new Date(2026, 11, 31, 18, 30))).toBe("2026-12-31T18:30")
	})

	test("round-trips through the Date parser", () => {
		const value = formatDateTimeLocal(new Date(2026, 8, 26, 23, 59).getTime())
		expect(new Date(value).getTime()).toBe(new Date(2026, 8, 26, 23, 59).getTime())
	})
})
