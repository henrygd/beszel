import { describe, expect, mock, test } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	t: (strings: string | TemplateStringsArray) => (typeof strings === "string" ? strings : (strings?.[0] ?? "")),
	plural: (_count: number, forms: { other?: string }) => forms.other ?? "",
}))

const { getMonitorStats, mergeMonitorStats, mergeSameTimestamps, withMonitorGaps } = await import(
	"../src/lib/network-monitor-utils"
)

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
