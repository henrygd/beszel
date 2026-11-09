import { expect, test } from "bun:test"
import { syncByNearestTime } from "./chart-sync"

const ticks = [100, 200, 300, 400].map((value) => ({ value }))

test("matches exact timestamps", () => {
	expect(syncByNearestTime(ticks, { activeLabel: 100 })).toBe(0)
	expect(syncByNearestTime(ticks, { activeLabel: 300 })).toBe(2)
	expect(syncByNearestTime(ticks, { activeLabel: 400 })).toBe(3)
})

test("matches the nearest timestamp", () => {
	expect(syncByNearestTime(ticks, { activeLabel: 140 })).toBe(0)
	expect(syncByNearestTime(ticks, { activeLabel: 160 })).toBe(1)
	expect(syncByNearestTime(ticks, { activeLabel: 399 })).toBe(3)
})

test("returns -1 outside the data range or without a label", () => {
	expect(syncByNearestTime(ticks, { activeLabel: 99 })).toBe(-1)
	expect(syncByNearestTime(ticks, { activeLabel: 401 })).toBe(-1)
	expect(syncByNearestTime(ticks, {})).toBe(-1)
	expect(syncByNearestTime([], { activeLabel: 100 })).toBe(-1)
})

test("skips gap markers", () => {
	const gapped = [{ value: 100 }, { value: null }, { value: 300 }, { value: null }]
	expect(syncByNearestTime(gapped, { activeLabel: 190 })).toBe(0)
	expect(syncByNearestTime(gapped, { activeLabel: 210 })).toBe(2)
	expect(syncByNearestTime(gapped, { activeLabel: 300 })).toBe(2)
	expect(syncByNearestTime([{ value: null }], { activeLabel: 100 })).toBe(-1)
})
