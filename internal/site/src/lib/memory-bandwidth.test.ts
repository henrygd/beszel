import { expect, test } from "bun:test"
import type { SystemStatsRecord } from "@/types"
import { memoryBandwidthTotal } from "./memory-bandwidth"

const stats = (values: Partial<SystemStatsRecord["stats"]>) => values as SystemStatsRecord["stats"]

test("total keeps writes when history mixes split and total-only sources", () => {
	expect(memoryBandwidthTotal(stats({ mbw: [10, 4] }), false)).toBe(14)
	expect(memoryBandwidthTotal(stats({ mbw: [40, 0], mbwt: true }), false)).toBe(40)
	// Hub averaging ORs mbwt and averages each direction independently.
	expect(memoryBandwidthTotal(stats({ mbw: [25, 2], mbwt: true }), false)).toBe(27)
})

test("max total requires a total-only peak", () => {
	expect(memoryBandwidthTotal(stats({ mbw: [40, 0], mbwm: [60, 0], mbwt: true }), true)).toBe(60)
	expect(memoryBandwidthTotal(stats({ mbw: [40, 0], mbwt: true }), true)).toBe(40)
	expect(memoryBandwidthTotal(stats({ mbw: [10, 4], mbwm: [30, 8] }), true)).toBeUndefined()
	expect(memoryBandwidthTotal(stats({ mbw: [25, 2], mbwm: [40, 8], mbwt: true }), true)).toBeUndefined()
})

test("missing system bandwidth stays absent, including GPU-only records", () => {
	for (const showMax of [false, true]) {
		expect(memoryBandwidthTotal(stats({}), showMax)).toBeUndefined()
		expect(memoryBandwidthTotal(stats({ g: { "0": { n: "GPU", u: 0, bw: 100, bwp: 200 } } }), showMax)).toBeUndefined()
	}
})
