import { expect, test } from "bun:test"
import { hasZfsArcStats } from "./zfs"
import type { SystemStats, SystemStatsRecord } from "@/types"

const record = (mz?: number) => ({ stats: (mz === undefined ? {} : { mz }) as SystemStats }) as SystemStatsRecord

test("series is hidden when no record carries ARC stats", () => {
	expect(hasZfsArcStats([])).toBe(false)
	expect(hasZfsArcStats([record(), record()])).toBe(false)
})

test("series is shown as soon as one record in the window has ARC stats", () => {
	expect(hasZfsArcStats([record(), record(0.5)])).toBe(true)
	// A zero ARC reading is still reported data and must keep the series.
	expect(hasZfsArcStats([record(0)])).toBe(true)
})
