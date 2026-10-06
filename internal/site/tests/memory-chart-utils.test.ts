import { describe, expect, test } from "bun:test"
import { getMemoryChartTotal } from "../src/components/routes/system/charts/memory-chart-utils"

describe("memory chart domain", () => {
	test("preserves a sub-megabyte physical memory total", () => {
		const totalMemoryGiB = 311.3 / 1024 / 1024

		expect(getMemoryChartTotal([{ stats: { m: totalMemoryGiB } }])).toBe(totalMemoryGiB)
	})

	test("defaults to zero while chart data is unavailable", () => {
		expect(getMemoryChartTotal()).toBe(0)
	})
})
