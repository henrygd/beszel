import { expect, test } from "bun:test"
import { selectChartMax } from "../src/components/routes/system/chart-max"
import type { SystemStats } from "../src/types"

test("old records and gaps retain averages; zero and negative peaks survive", () => {
	const old = { cpu: 20, t: { cold: -30 } } as SystemStats
	expect(selectChartMax(old).t).toEqual(old.t)
	expect(selectChartMax(old).cpum).toBe(20)
	const record = { ...old, mx: { t: { cold: -10 }, swap: [100, 0] } } as SystemStats
	expect(selectChartMax(record).t?.cold).toBe(-10)
	expect(selectChartMax(record).swap?.[1]).toBe(0)
	expect(record.t?.cold).toBe(-30)
})

test("nested peaks retain capacity and display metadata", () => {
	const record = { g: { gpu: { n: "Demo", mt: 1000, u: 20 } }, mx: { g: { gpu: { u: 80 } } } } as unknown as SystemStats
	expect(selectChartMax(record).g?.gpu).toMatchObject({ n: "Demo", mt: 1000, u: 80 })
})

test("peak projection preserves existing IO peaks and cumulative counters", () => {
	const record = {
		efs: { disk: { d: 100, du: 20, rbm: 900 } },
		ni: { eth: [10, 20, 1000, 2000] },
		mx: { efs: { disk: { d: 0, du: 80, rbm: 0 } }, ni: { eth: [90, 80, 0, 0] } },
	} as unknown as SystemStats
	expect(selectChartMax(record).efs?.disk.rbm).toBe(900)
	expect(selectChartMax(record).efs?.disk.d).toBe(100)
	expect(selectChartMax(record).ni?.eth).toEqual([90, 80, 1000, 2000])
})
