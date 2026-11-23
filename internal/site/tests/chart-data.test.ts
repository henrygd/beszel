import { describe, expect, mock, test } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	t: (value: string | TemplateStringsArray) => (typeof value === "string" ? value : value[0]),
	plural: (_count: number, forms: { other: string }) => forms.other,
}))

await import("../src/lib/utils")
const { appendData } = await import("../src/components/routes/system/chart-data")
const points = (start: number, count: number) =>
	Array.from({ length: count }, (_, index) => ({ created: (start + index) * 60_000 }))

describe("chart data point limit", () => {
	test("keeps the newest points when an update crosses the limit", () => {
		const previous = points(1, 99)
		const incoming = points(100, 2)
		const result = appendData(previous, incoming, 60_000, 100)
		expect(result).toHaveLength(100)
		expect(result[0].created).toBe(120_000)
		expect(result.at(-1)?.created).toBe(101 * 60_000)
		expect(previous).toHaveLength(99)
	})

	test("replaces a full cache when a full window arrives", () => {
		const incoming = points(101, 100)
		expect(appendData(points(1, 100), incoming, 60_000, 100)).toEqual(incoming)
	})

	test("caps an initial history fetch larger than the limit", () => {
		const incoming = points(1, 150)
		expect(appendData([], incoming, 60_000, 100)).toEqual(incoming.slice(-100))
	})

	test("caps oversized batches when a full cache already exists", () => {
		const incoming = points(101, 150)
		expect(appendData(points(1, 100), incoming, 60_000, 100)).toEqual(incoming.slice(-100))
	})

	test("includes inserted gap markers in the limit", () => {
		const result = appendData(points(1, 100), points(103, 1), 60_000, 100)
		expect(result).toHaveLength(100)
		expect(result[0].created).toBe(180_000)
		expect(result.slice(-2)).toEqual([{ created: null }, { created: 103 * 60_000 }])
	})

	test("preserves the current behavior when there is no limit", () => {
		expect(appendData(points(1, 100), points(101, 2), 60_000)).toHaveLength(102)
	})

	test("retains conversion of string timestamps and stats gap markers", () => {
		const previous = [{ created: 60_000, stats: {} }]
		const incoming = [{ created: "1970-01-01T00:04:00Z", stats: {} }]
		expect(
			appendData<{ created: string | number | null; stats: object | null }>(previous, incoming, 60_000, 2)
		).toEqual([
			{ created: null, stats: null },
			{ created: 240_000, stats: {} },
		])
	})
})
