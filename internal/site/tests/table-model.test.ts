import { describe, expect, test } from "bun:test"
import {
	formatTableTime,
	numericValue,
	pageRange,
	snapshotKey,
	sourceSignature,
	type TablePoint,
	tableRows,
} from "../src/components/charts/table-model"
import type { SystemStatsRecord } from "../src/types"

type Row = { created: number | null; avg?: number | null; gpu?: number | null }

const points: TablePoint<Row>[] = [
	{ label: "CPU", dataKey: (r) => r.avg },
	{ label: "GPU", dataKey: (r) => r.gpu },
]

describe("numericValue", () => {
	test("zero is a measurement", () => {
		expect(numericValue(0)).toBe(0)
		expect(numericValue(-0.5)).toBe(-0.5)
	})

	test("missing, non-finite and string values are not measurements", () => {
		for (const value of [null, undefined, Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY, "0", {}]) {
			expect(numericValue(value)).toBeNull()
		}
	})
})

describe("tableRows", () => {
	test("uses the caller accessors and keeps the named payload for the shared formatter", () => {
		const data: Row[] = [{ created: 1, avg: 0, gpu: 2 }]
		const [row] = tableRows(data, points, false)
		expect(row.cells.map((c) => c.value)).toEqual([0, 2])
		expect(row.cells[0].name).toBe("CPU")
		expect(row.cells[0].payload).toBe(data[0])
	})

	test("missing samples stay missing instead of becoming zero", () => {
		const [row] = tableRows([{ created: 2, avg: null, gpu: undefined }], points, true)
		expect(row.cells.map((c) => c.value)).toEqual([null, null, null])
	})

	test("total sums only the available series and uses the shared total key", () => {
		const [row] = tableRows([{ created: 1, avg: 12.5, gpu: 75 }], points, true)
		expect(row.cells.at(-1)?.key).toBe("__total__")
		expect(row.cells.at(-1)?.value).toBe(87.5)
	})

	test("dimmed series are excluded from cells and from the total", () => {
		const withHidden: TablePoint<Row>[] = [...points, { label: "hidden", activeDot: false, dataKey: () => 100 }]
		const [row] = tableRows([{ created: 1, avg: 12.5, gpu: 75 }], withHidden, true)
		expect(row.cells).toHaveLength(3)
		expect(row.cells.at(-1)?.value).toBe(87.5)
	})

	test("avg or max comes from the accessor the caller passed, never recomputed", () => {
		const data = [{ created: 1, avg: 3, max: 42 }]
		expect(tableRows(data, [{ label: "CPU", dataKey: (r) => r.avg }], false)[0].cells[0].value).toBe(3)
		expect(tableRows(data, [{ label: "CPU", dataKey: (r) => r.max }], false)[0].cells[0].value).toBe(42)
	})

	test("gap sentinels are dropped but epoch zero is a real sample", () => {
		expect(tableRows([{ created: null }, { created: 0, avg: 0 }], points, false)).toHaveLength(1)
	})

	test("does not mutate the caller data", () => {
		const data: Row[] = [{ created: 1, avg: 1, gpu: 2 }]
		const snapshot = structuredClone(data)
		tableRows(data, points, true)
		expect(data).toEqual(snapshot)
	})

	test("accepts the real SystemStatsRecord shape without casts", () => {
		const stats: SystemStatsRecord[] = [{ created: 1_700_000_000_000, stats: { cpu: 12.5 } } as SystemStatsRecord]
		const cpu: TablePoint<SystemStatsRecord>[] = [{ label: "CPU", dataKey: ({ stats }) => stats?.cpu }]
		expect(tableRows(stats, cpu, false)[0].cells[0].value).toBe(12.5)
	})
})

describe("pageRange", () => {
	test("covers every row exactly once across the pages", () => {
		const seen: number[] = []
		for (let page = 0; page < 6; page++) {
			const range = pageRange(103, page, 20)
			seen.push(...Array.from({ length: range.end - range.start }, (_, i) => range.start + i))
		}
		expect(seen).toEqual(Array.from({ length: 103 }, (_, i) => i))
	})

	test("clamps an out-of-range page and reports one empty page for no rows", () => {
		expect(pageRange(103, 99, 20)).toEqual({ start: 100, end: 103, page: 5, pages: 6 })
		expect(pageRange(0, 8, 20)).toEqual({ start: 0, end: 0, page: 0, pages: 1 })
	})

	test("defaults to twenty rows per page", () => {
		expect(pageRange(100, 0).end).toBe(20)
	})
})

describe("formatTableTime", () => {
	test("renders the requested time zone, not the host zone", () => {
		const utc = formatTableTime(0, "pl", "UTC")
		expect(utc).toMatch(/1970/)
		expect(utc).toMatch(/UTC|GMT/)
		expect(formatTableTime(0, "pl", "Australia/Sydney")).not.toBe(utc)
	})

	test("missing and unparseable stamps have no rendered time", () => {
		for (const value of [null, undefined, Number.NaN, "not a date", {}]) {
			expect(formatTableTime(value, "pl", "UTC")).toBeNull()
		}
	})
})

describe("snapshotKey", () => {
	test("a deliberate scope, range, filter, max, units or locale change is a different key", () => {
		const selection = {
			scope: "sys1",
			time: "1h",
			filter: "",
			max: false,
			units: ["bytes", "bytes", "celsius"],
			locale: "pl",
			title: "CPU",
		}
		const changes = {
			scope: "sys2",
			time: "24h",
			filter: "db",
			max: true,
			units: ["bits", "bytes", "celsius"],
			locale: "en",
			title: "Memory",
		}
		for (const [field, value] of Object.entries(changes)) {
			expect(snapshotKey({ ...selection, [field]: value })).not.toBe(snapshotKey(selection))
		}
	})

	test("the same selection is the same key regardless of object identity", () => {
		expect(snapshotKey({ scope: "sys1", time: "1h" })).toBe(snapshotKey({ scope: "sys1", time: "1h" }))
	})
})

describe("sourceSignature", () => {
	test("new arrivals change the signature", () => {
		const rows = [{ created: 1 }, { created: 2 }]
		expect(sourceSignature(rows)).not.toBe(sourceSignature([...rows, { created: 3 }]))
	})

	test("a recreated array with the same records has the same signature", () => {
		const rows = [{ created: 1 }, { created: 2 }]
		expect(sourceSignature(rows)).toBe(sourceSignature(rows.map((r) => ({ ...r }))))
	})

	test("a rolling window that drops an old sample and adds a new one changes the signature", () => {
		expect(sourceSignature([{ created: 1 }, { created: 2 }])).not.toBe(
			sourceSignature([{ created: 2 }, { created: 3 }])
		)
	})

	test("an empty source has a stable signature", () => {
		expect(sourceSignature([])).toBe(sourceSignature([]))
		expect(sourceSignature([])).not.toBe(sourceSignature([{ created: 1 }]))
	})

	test("a corrected sample changes the signature even when timestamps and count stay unchanged", () => {
		const first = [
			{ created: 1, stats: { cpu: 0 } },
			{ created: 2, stats: { cpu: 20 } },
		]
		const corrected = [
			{ created: 1, stats: { cpu: 0 } },
			{ created: 2, stats: { cpu: 40 } },
		]
		expect(sourceSignature(first)).not.toBe(sourceSignature(corrected))
	})
})
