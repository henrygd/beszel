import { describe, expect, mock, test } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	t: (strings: string | TemplateStringsArray) => (typeof strings === "string" ? strings : (strings?.[0] ?? "")),
	plural: (_count: number, forms: { other?: string }) => forms.other ?? "",
}))

const { getMonitorStats } = await import("../src/lib/network-monitor-utils")

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
		expect(stats.res_avg).toBe(10.17)
		expect(stats.loss).toBe(14.29)
		expect(stats.res_min).toBe(5)
		expect(stats.res_max).toBe(20)
	})

	test("limits average response and loss to two decimals", () => {
		const stats = getMonitorStats({
			monitor: "monitor1",
			created: 1000,
			res_min: 3,
			res_max: 4,
			total_count: 9,
			success_count: 3,
			res_sum: 10,
		})
		expect(stats.res_avg).toBe(3.33)
		expect(stats.loss).toBe(66.67)
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
