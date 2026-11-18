import { describe, expect, mock, test } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	t: (strings: string | TemplateStringsArray) => (typeof strings === "string" ? strings : (strings?.[0] ?? "")),
	plural: (_count: number, forms: { other?: string }) => forms.other ?? "",
}))

const {
	formatMonitorFields,
	getMonitorIncidents,
	getMonitorStats,
	mergeMonitorStats,
	mergeSameTimestamps,
	parseMonitorFields,
	withMonitorGaps,
} = await import("../src/lib/network-monitor-utils")

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
		{ total_count: 3, success_count: 0, loss: 100, res: null },
		{ total_count: 0, success_count: 0, loss: 0, res: null },
		{ total_count: 1, success_count: 1, loss: 0, res: 0 },
	])("handles zero sums with $total_count attempts and $success_count successes", ({ loss, res, ...counts }) => {
		const stats = getMonitorStats({
			monitor: "monitor1",
			created: 1000,
			res_min: 0,
			res_max: 0,
			res_sum: 0,
			...counts,
		})
		expect(stats).toEqual({ res_avg: res, res_min: res, res_max: res, loss })
	})
})

describe("monitor gaps", () => {
	const monitor = { id: "m1", interval: 30 }
	const stats = { res_avg: 1, res_min: 1, res_max: 1, loss: 0 }
	const record = (created: number | null, id = monitor.id) => ({ created, stats: { [id]: stats } })

	test("does not insert markers at the expected cadence", () => {
		const records = [record(0), record(60_000), record(120_000)]
		expect(withMonitorGaps(records, monitor, 60_000)).toEqual(records)
	})

	test("inserts a marker between records further apart than expected", () => {
		const records = [record(60_000), record(300_000)]
		expect(withMonitorGaps(records, monitor, 60_000)).toEqual([records[0], { created: null, stats: null }, records[1]])
	})

	test("uses the monitor interval when it is longer than the tier interval", () => {
		const slowMonitor = { id: monitor.id, interval: 300 }
		const records = [record(300_000), record(600_000), record(900_000)]
		expect(withMonitorGaps(records, slowMonitor, 60_000)).toEqual(records)
		expect(withMonitorGaps([records[0], record(1_200_000)], slowMonitor, 60_000)).toHaveLength(3)
	})

	test("skips records for other monitors and existing gap markers", () => {
		const records = [
			record(60_000),
			record(90_000, "m2"),
			{ created: null, stats: null },
			record(120_000),
		] as Parameters<typeof withMonitorGaps>[0]
		expect(withMonitorGaps(records, monitor, 60_000)).toEqual([records[0], records[3]])
	})
})

const raw = (monitor: string, created: number, res_sum = 10) => ({
	monitor,
	created,
	res_min: 1,
	res_max: 20,
	total_count: 1,
	success_count: 1,
	res_sum,
})

describe("merging stats across monitors", () => {
	test("keeps exact timestamps without bucketing", () => {
		const merged = mergeMonitorStats([raw("a", 60_100), raw("b", 60_400)])
		expect(merged.map((r) => r.created)).toEqual([60_100, 60_400])
	})

	test("aligns unaligned system timestamps into one row per bucket", () => {
		const merged = mergeMonitorStats([raw("b", 120_900, 30), raw("a", 60_100), raw("b", 60_400, 20)], 60_000)
		expect(merged).toHaveLength(2)
		expect(merged[0].created).toBe(60_000)
		expect(Object.keys(merged[0].stats).sort()).toEqual(["a", "b"])
		expect(merged[0].stats.b.res_avg).toBe(20)
		expect(merged[1].created).toBe(120_000)
		expect(merged[1].stats.b.res_avg).toBe(30)
	})

	test("folds a late system's record into an existing cached bucket", () => {
		const existing = mergeMonitorStats([raw("a", 60_100)], 60_000)
		const late = mergeMonitorStats([raw("b", 60_700), raw("b", 120_200)], 60_000)
		const remaining = mergeSameTimestamps(existing, late)
		expect(Object.keys(existing[0].stats).sort()).toEqual(["a", "b"])
		expect(remaining.map((r) => r.created)).toEqual([120_000])
	})
})

describe("bulk monitor rows", () => {
	test.each(
		[
			["1.1.1.1", "icmp"],
			["example.com", "tcp", "8080", "60"],
			["example.com", "dns", "", "", "1.1.1.1"],
			["https://example.com/status?ids=1,2", "http", "", "60"],
			['https://example.com/status?q="a,b"', "http"],
			["", "http", ""],
		].map((fields) => ({ fields }))
	)("round-trips $fields", ({ fields }) => {
		expect(parseMonitorFields(formatMonitorFields(fields))).toEqual(fields)
	})

	test("keeps the existing unquoted format for ordinary entries", () => {
		expect(formatMonitorFields(["example.com", "tcp", "", "60"])).toBe("example.com,tcp,,60")
		expect(parseMonitorFields("example.com,tcp,,60")).toEqual(["example.com", "tcp", "", "60"])
	})

	test("does not treat a quoted target's comma as its protocol separator", () => {
		expect(parseMonitorFields('"https://example.com/status?ids=1,2",http')).toEqual([
			"https://example.com/status?ids=1,2",
			"http",
		])
	})

	test("allows whitespace before a quoted field", () => {
		expect(parseMonitorFields('  "https://example.com/status?ids=1,2",http')).toEqual([
			"https://example.com/status?ids=1,2",
			"http",
		])
		expect(parseMonitorFields('example.com, "a,b", 53')).toEqual(["example.com", "a,b", " 53"])
	})

	test("rejects malformed quoted rows instead of shifting fields", () => {
		expect(() => parseMonitorFields('"https://example.com,status,http')).toThrow("Unclosed quoted field")
		expect(() => parseMonitorFields('"example.com"extra,tcp')).toThrow("Expected comma after quoted field")
	})
})

describe("monitor incidents", () => {
	const record = (created: number | null, loss: number | null, id = "m1") =>
		({ created, stats: loss === null ? {} : { [id]: { res_avg: 1, res_min: 1, res_max: 1, loss } } }) as never

	test("groups consecutive buckets with loss and reports the worst loss", () => {
		const records = [
			record(60_000, 0),
			record(120_000, 20),
			record(180_000, 100),
			record(240_000, 0),
			record(300_000, 50),
			record(360_000, 0),
		]
		expect(getMonitorIncidents(records, "m1", 60_000)).toEqual([
			{ start: 240_000, end: 300_000, loss: 50, ongoing: false },
			{ start: 60_000, end: 180_000, loss: 100, ongoing: false },
		])
	})

	test("marks an incident as ongoing while the latest record is affected", () => {
		const records = [record(60_000, 0), record(120_000, 100)]
		expect(getMonitorIncidents(records, "m1", 60_000)).toEqual([
			{ start: 60_000, end: 120_000, loss: 100, ongoing: true },
		])
	})

	test("missing data ends an incident instead of extending it", () => {
		const records = [record(60_000, 100), record(null, null), record(180_000, 100), record(240_000, null)]
		const incidents = getMonitorIncidents(records, "m1", 60_000)
		expect(incidents).toHaveLength(2)
		expect(incidents.every((incident) => !incident.ongoing)).toBe(true)
	})

	test("ignores other monitors and returns nothing without failures", () => {
		expect(getMonitorIncidents([record(60_000, 100, "other")], "m1", 60_000)).toEqual([])
		expect(getMonitorIncidents([record(60_000, 0), record(120_000, 0)], "m1", 60_000)).toEqual([])
		expect(getMonitorIncidents([], "m1", 60_000)).toEqual([])
	})
})
