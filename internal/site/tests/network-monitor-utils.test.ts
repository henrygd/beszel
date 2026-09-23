import { describe, expect, test } from "bun:test"
import { getMonitorStats } from "../src/lib/network-monitor-utils"

describe("monitor stats derived from stored counts", () => {
	test("retains probe weights and response precision", () => {
		const stats = getMonitorStats({
			monitor: "monitor1",
			created: 1000,
			res_min: 5,
			res_max: 20,
			total_count: 7,
			success_count: 6,
			res_sum: 61,
		})
		expect(stats.res_avg).toBeCloseTo(10.1666667)
		expect(stats.loss).toBeCloseTo(14.2857143)
		expect(stats.res_min).toBe(5)
		expect(stats.res_max).toBe(20)
	})

	test.each([
		{ total_count: 3, success_count: 0, loss: 100 },
		{ total_count: 0, success_count: 0, loss: 0 },
		{ total_count: 1, success_count: 1, loss: 0 },
	])("handles zero sums with $total_count attempts and $success_count successes", ({ loss, ...counts }) => {
		const stats = getMonitorStats({
			monitor: "monitor1",
			created: 1000,
			res_min: 0,
			res_max: 0,
			res_sum: 0,
			...counts,
		})
		expect(stats).toEqual({ res_avg: 0, res_min: 0, res_max: 0, loss })
	})
})
