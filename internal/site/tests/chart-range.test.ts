import { describe, expect, mock, test } from "bun:test"

// The real formatters depend on the browser locale, and lib/utils pulls in UI and lingui macros.
// These stubs read UTC, which matches the ISO strings formatRange passes them.
mock.module("../src/lib/utils", () => ({
	formatDay: (timestamp: string) => timestamp.slice(0, 10),
	hourWithMinutes: (timestamp: string) => timestamp.slice(11, 16),
}))

const {
	formatRange,
	getRangeChartTime,
	normalizeRange,
	oldestRangeStart,
	rangeAxisTicks,
	shiftRange,
	toDateTimeLocal,
} = await import("../src/lib/chart-range")

const minute = 60 * 1000
const hour = 60 * minute
const day = 24 * hour
const now = Date.UTC(2026, 9, 30, 12, 0, 0)

describe("getRangeChartTime", () => {
	test("uses the finest tier that still covers the start of the range", () => {
		expect(getRangeChartTime({ start: now - 30 * minute, end: now }, now)).toBe("1h")
		expect(getRangeChartTime({ start: now - 3 * hour, end: now - 2 * hour }, now)).toBe("12h")
		expect(getRangeChartTime({ start: now - 20 * hour, end: now - 16 * hour }, now)).toBe("24h")
		expect(getRangeChartTime({ start: now - 3 * day, end: now - 2 * day }, now)).toBe("1w")
		expect(getRangeChartTime({ start: now - 20 * day, end: now - 19 * day }, now)).toBe("30d")
	})

	test("allows a minute of slack for minute precision inputs", () => {
		expect(getRangeChartTime({ start: now - hour - 30 * 1000, end: now }, now)).toBe("1h")
		expect(getRangeChartTime({ start: now - hour - 2 * minute, end: now }, now)).toBe("12h")
	})

	test("falls back to the coarsest tier for ranges older than the stored history", () => {
		expect(getRangeChartTime({ start: now - 40 * day, end: now - 35 * day }, now)).toBe("30d")
	})
})

describe("normalizeRange", () => {
	test("clamps the range to the stored history", () => {
		expect(normalizeRange({ start: now - 40 * day, end: now + hour }, now)).toEqual({
			start: oldestRangeStart(now),
			end: now,
			chartTime: "30d",
		})
	})

	test("stores the tier picked at normalize time", () => {
		const range = normalizeRange({ start: now - 24 * hour, end: now }, now)
		expect(range?.chartTime).toBe("24h")
		// the dialog hint and the fetch both read this, so a later now can't move the range to 1w
		expect(getRangeChartTime({ start: now - 24 * hour, end: now }, now + 2 * minute)).toBe("1w")
	})

	test("rejects empty, reversed or invalid ranges", () => {
		expect(normalizeRange({ start: now - hour, end: now - hour }, now)).toBeNull()
		expect(normalizeRange({ start: now - hour, end: now - 2 * hour }, now)).toBeNull()
		expect(normalizeRange({ start: Number.NaN, end: now }, now)).toBeNull()
	})
})

describe("shiftRange", () => {
	const range = { start: now - 2 * day, end: now - 2 * day + 6 * hour, chartTime: "1w" as const }

	test("steps back and forward by the range length", () => {
		expect(shiftRange(range, -1, now)).toEqual({
			start: range.start - 6 * hour,
			end: range.start,
			chartTime: "1w",
		})
		expect(shiftRange(range, 1, now)).toEqual({ start: range.end, end: range.end + 6 * hour, chartTime: "1w" })
	})

	test("stops at the edges of the stored history", () => {
		expect(shiftRange({ start: now - 3 * hour, end: now - hour, chartTime: "12h" }, 1, now)).toEqual({
			start: now - 2 * hour,
			end: now,
			chartTime: "12h",
		})
		expect(shiftRange({ start: now - hour, end: now, chartTime: "1h" }, 1, now)).toBeNull()
		const oldest = oldestRangeStart(now)
		expect(shiftRange({ start: oldest, end: oldest + hour, chartTime: "30d" }, -1, now)).toBeNull()
	})
})

describe("toDateTimeLocal", () => {
	test("formats local time with zero padded fields", () => {
		expect(toDateTimeLocal(new Date(2026, 0, 5, 9, 7, 30).getTime())).toBe("2026-01-05T09:07")
		expect(toDateTimeLocal(new Date(2026, 11, 31, 23, 59).getTime())).toBe("2026-12-31T23:59")
	})

	test("round trips through a datetime-local value at minute precision", () => {
		const timestamp = now + 42 * 1000
		expect(new Date(toDateTimeLocal(timestamp)).getTime()).toBe(now)
	})
})

describe("formatRange", () => {
	test("shows the day once when the range stays within it", () => {
		expect(formatRange({ start: now - 6 * hour, end: now })).toBe("2026-10-30 06:00 – 12:00")
	})

	test("shows both days when the range crosses midnight", () => {
		expect(formatRange({ start: now - day, end: now - 6 * hour })).toBe("2026-10-29 12:00 – 2026-10-30 06:00")
	})
})

describe("rangeAxisTicks", () => {
	test("labels times of day for ranges up to a day", () => {
		expect(rangeAxisTicks({ start: now - 6 * hour, end: now })).toEqual({ tickCount: 12, daily: false })
		expect(rangeAxisTicks({ start: now - day, end: now })).toEqual({ tickCount: 12, daily: false })
	})

	test("puts one tick per day on longer ranges, up to 30", () => {
		expect(rangeAxisTicks({ start: now - 3 * day - 2 * hour, end: now })).toEqual({ tickCount: 3, daily: true })
		expect(rangeAxisTicks({ start: now - 40 * day, end: now })).toEqual({ tickCount: 30, daily: true })
	})
})
