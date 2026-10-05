import { describe, expect, mock, test } from "bun:test"
import type { SystemStatsRecord } from "../src/types"

// Web Storage so the real modules can be imported outside a browser. No application API is replaced.
if (typeof globalThis.localStorage === "undefined") {
	const values = new Map<string, string>()
	Object.defineProperty(globalThis, "localStorage", {
		configurable: true,
		value: {
			get length() {
				return values.size
			},
			getItem: (key: string) => values.get(String(key)) ?? null,
			setItem: (key: string, value: string) => {
				values.set(String(key), String(value))
			},
			removeItem: (key: string) => {
				values.delete(String(key))
			},
			clear: () => values.clear(),
			key: (index: number) => [...values.keys()][index] ?? null,
		},
	})
}

// Only the translation macros are stubbed; the accessors under test are the real exported ones.
mock.module("@lingui/core/macro", () => ({
	t: (strings: string | TemplateStringsArray | { message?: string }) => {
		if (typeof strings === "string") return strings
		if (Array.isArray(strings)) return strings[0] ?? ""
		return (strings as { message?: string })?.message ?? ""
	},
	plural: (_count: number, forms: { other?: string }) => forms.other ?? "",
}))
mock.module("@lingui/react/macro", () => ({
	Trans: () => null,
	useLingui: () => ({ t: (s: TemplateStringsArray) => s?.[0] ?? "", i18n: { locale: "en" } }),
}))

// utils.ts and stores.ts are mutually dependent; load them first so the order does not depend on
// which chart module happens to be imported first.
await import("../src/lib/utils")
const { tableRows, tableSample, withChartFallback } = await import("../src/components/charts/table-model")
const { diskDataFns } = await import("../src/components/routes/system/charts/disk-charts")
const { networkDataFns } = await import("../src/components/routes/system/charts/network-charts")
const { zfsPoolDataFns } = await import("../src/components/routes/system/charts/storage-pool-charts")
const { cpuCoreDataFns } = await import("../src/components/routes/system/cpu-sheet")

// Partial and malformed records deliberately exercise compatibility with older agents.
const row = (stats: Record<string, unknown> | null) => ({ created: 1, stats }) as unknown as SystemStatsRecord
const graphic = (fn: (r: SystemStatsRecord) => unknown, r: SystemStatsRecord) => fn(r)
const table = (fn: (r: SystemStatsRecord) => unknown, r: SystemStatsRecord) => tableSample(fn)(r)

describe("withChartFallback", () => {
	test("the graphic gets the substituted value, the table the raw gap", () => {
		const accessor = withChartFallback((r: { v?: number | null }) => r.v)
		expect(accessor({})).toBe(0)
		expect(accessor({ v: null })).toBe(0)
		expect(accessor({ v: Number.NaN })).toBeNaN()
		expect(tableSample(accessor)({})).toBeUndefined()
		expect(tableSample(accessor)({ v: null })).toBeNull()
	})

	test("a present sample, including a real zero, is never substituted", () => {
		const accessor = withChartFallback((r: { v?: number | null }) => r.v)
		expect(accessor({ v: 0 })).toBe(0)
		expect(accessor({ v: -3.5 })).toBe(-3.5)
		expect(tableSample(accessor)({ v: 0 })).toBe(0)
	})

	test("a computed fallback receives the same row the graphic is drawing", () => {
		const accessor = withChartFallback(
			(r: { cpus?: number[] }) => r.cpus?.[3],
			(r: { cpus?: number[] }) => 1 / (r.cpus?.length ?? 1)
		)
		expect(accessor({ cpus: [1, 2] })).toBe(0.5)
		expect(tableSample(accessor)({ cpus: [1, 2] })).toBeUndefined()
	})

	test("an accessor without a substitution is returned unchanged for the table", () => {
		const plain = (r: { v?: number | null }) => r.v
		expect(tableSample(plain)).toBe(plain)
		expect(tableSample(plain)({ v: null })).toBeNull()
	})
})

describe("disk accessors keep the upstream graphic fallback", () => {
	test("usage and cumulative totals draw zero but report a missing sample", () => {
		for (const fn of [diskDataFns.usage, diskDataFns.totalRead, diskDataFns.totalWrite]) {
			expect(graphic(fn, row({}))).toBe(0)
			expect(table(fn, row({}))).toBeUndefined()
		}
		expect(graphic(diskDataFns.usage, row({ du: 0 }))).toBe(0)
		expect(table(diskDataFns.usage, row({ du: 0 }))).toBe(0)
	})

	test("throughput prefers the byte field and falls back to the older MiB field", () => {
		expect(graphic(diskDataFns.read, row({ dio: [7, 9] }))).toBe(7)
		expect(graphic(diskDataFns.write, row({ dio: [7, 9] }))).toBe(9)
		// older agents only sent dr/dw in MiB/s
		expect(graphic(diskDataFns.read, row({ dr: 2 }))).toBe(2 * 1024 * 1024)
		expect(table(diskDataFns.read, row({ dr: 2 }))).toBe(2 * 1024 * 1024)
		// neither field present: graphic draws zero, table reports the gap
		expect(graphic(diskDataFns.read, row({}))).toBe(0)
		expect(table(diskDataFns.read, row({}))).toBeUndefined()
		expect(graphic(diskDataFns.readMax, row({}))).toBe(0)
		expect(table(diskDataFns.writeMax, row({}))).toBeUndefined()
	})

	test("extra filesystem throughput behaves like the root filesystem", () => {
		const read = diskDataFns.extraRead("data")
		expect(graphic(read, row({ efs: { data: { rb: 5 } } }))).toBe(5)
		expect(graphic(read, row({ efs: { data: { r: 3 } } }))).toBe(3 * 1024 * 1024)
		expect(graphic(read, row({ efs: {} }))).toBe(0)
		expect(table(read, row({ efs: {} }))).toBeUndefined()
		expect(table(diskDataFns.extraUsage("data"), row({ efs: {} }))).toBeUndefined()
	})

	test("indexed dios metrics draw zero and report the gap", () => {
		expect(graphic(diskDataFns.util, row({ dios: [1, 2, 55] }))).toBe(55)
		expect(graphic(diskDataFns.util, row({}))).toBe(0)
		expect(table(diskDataFns.util, row({}))).toBeUndefined()
		expect(table(diskDataFns.rAwait, row({ dios: [1] }))).toBeUndefined()
		expect(graphic(diskDataFns.extraUtil("data"), row({}))).toBe(0)
	})

	test("queue depth scales a present sample and does not scale a gap into zero", () => {
		expect(graphic(diskDataFns.weightedIO, row({ dios: [0, 0, 0, 0, 0, 250] }))).toBe(2.5)
		expect(table(diskDataFns.weightedIO, row({ dios: [0, 0, 0, 0, 0, 250] }))).toBe(2.5)
		expect(graphic(diskDataFns.weightedIO, row({}))).toBe(0)
		expect(table(diskDataFns.weightedIO, row({}))).toBeUndefined()
		expect(graphic(diskDataFns.extraWeightedIOMax("data"), row({}))).toBe(0)
		expect(table(diskDataFns.extraWeightedIOMax("data"), row({}))).toBeUndefined()
	})
})

describe("bandwidth accessors keep the upstream graphic fallback", () => {
	const sent = networkDataFns.sent(false)
	const recv = networkDataFns.received(false)
	const sentMax = networkDataFns.sent(true)

	test("the newer byte field wins, the older MiB field is converted", () => {
		expect(graphic(sent, row({ b: [11, 22] }))).toBe(11)
		expect(graphic(recv, row({ b: [11, 22] }))).toBe(22)
		expect(graphic(sent, row({ ns: 4 }))).toBe(4 * 1024 * 1024)
		expect(table(recv, row({ nr: 4 }))).toBe(4 * 1024 * 1024)
		expect(graphic(sentMax, row({ bm: [1, 2] }))).toBe(1)
		expect(graphic(sentMax, row({ nsm: 1 }))).toBe(1024 * 1024)
	})

	test("neither field present draws zero and reports a missing sample", () => {
		for (const fn of [sent, recv, sentMax, networkDataFns.received(true)]) {
			expect(graphic(fn, row({}))).toBe(0)
			expect(table(fn, row({}))).toBeUndefined()
		}
		expect(graphic(sent, row(null))).toBe(0)
		expect(table(sent, row(null))).toBeUndefined()
	})

	test("a real zero stays a measurement in both modes", () => {
		expect(graphic(sent, row({ b: [0, 0] }))).toBe(0)
		expect(table(sent, row({ b: [0, 0] }))).toBe(0)
	})
})

describe("container network graphic parity", () => {
	test("an absent container remains null instead of drawing a zero", () => {
		expect(networkDataFns.container("web", {})).toBeNull()
	})
	test("an incomplete legacy sample retains its measured direction in the graphic", () => {
		const data = { web: { ns: 2 } } as unknown as Parameters<typeof networkDataFns.container>[1]
		expect(networkDataFns.container("web", data)).toBe(2 * 1024 * 1024)
		expect(tableSample(networkDataFns.container)("web", data)).toBeNull()
	})
	test("an empty record is zero only in the graphic", () => {
		const data = { web: {} } as unknown as Parameters<typeof networkDataFns.container>[1]
		expect(networkDataFns.container("web", data)).toBe(0)
		expect(tableSample(networkDataFns.container)("web", data)).toBeNull()
	})
})

describe("zfs pool accessors keep the upstream graphic fallback", () => {
	test("read and write draw zero but report the gap", () => {
		const read = zfsPoolDataFns.read("tank")
		const write = zfsPoolDataFns.write("tank")
		expect(graphic(read, row({ z: { tank: { rb: 8 } } }))).toBe(8)
		expect(graphic(write, row({ z: { tank: { wb: 0 } } }))).toBe(0)
		expect(table(write, row({ z: { tank: { wb: 0 } } }))).toBe(0)
		expect(graphic(read, row({ z: {} }))).toBe(0)
		expect(table(read, row({ z: {} }))).toBeUndefined()
	})

	test("pool usage has no substitution upstream and keeps its explicit null", () => {
		const usage = zfsPoolDataFns.usage("tank", false)
		expect(graphic(usage, row({ z: {} }))).toBeNull()
		expect(table(usage, row({ z: {} }))).toBeNull()
		expect(graphic(usage, row({ z: { tank: { du: 12, raw: true } } }))).toBeNull()
		expect(graphic(usage, row({ z: { tank: { du: 12 } } }))).toBe(12)
	})
})

describe("cpu core accessors keep the upstream per-core share fallback", () => {
	test("a missing core draws the upstream share and reports a missing sample", () => {
		const core = cpuCoreDataFns.core(3)
		expect(graphic(core, row({ cpus: [10, 20, 30, 40] }))).toBe(40)
		expect(table(core, row({ cpus: [10, 20, 30, 40] }))).toBe(40)
		// upstream substituted 1 / cpus.length for a core with no sample
		expect(graphic(core, row({ cpus: [10, 20] }))).toBe(0.5)
		expect(table(core, row({ cpus: [10, 20] }))).toBeUndefined()
		expect(graphic(core, row({}))).toBe(1)
		expect(table(core, row({}))).toBeUndefined()
	})

	test("a real zero utilization is not mistaken for a gap", () => {
		const core = cpuCoreDataFns.core(0)
		expect(graphic(core, row({ cpus: [0, 50] }))).toBe(0)
		expect(table(core, row({ cpus: [0, 50] }))).toBe(0)
	})
})

describe("tableRows reads the raw sample of substituted accessors", () => {
	test("a disk row with no samples is all missing, not a row of zeros", () => {
		const points = [
			{ label: "Write", dataKey: diskDataFns.write },
			{ label: "Read", dataKey: diskDataFns.read },
		]
		const [projected] = tableRows([row({})], points, true)
		expect(projected.cells.map((c) => c.value)).toEqual([null, null, null])
	})

	test("a partially present row sums only the measured series", () => {
		const points = [
			{ label: "Write", dataKey: diskDataFns.write },
			{ label: "Read", dataKey: diskDataFns.read },
		]
		const [projected] = tableRows([row({ dio: [undefined, 40] })], points, true)
		expect(projected.cells.map((c) => c.value)).toEqual([40, null, 40])
	})

	test("plain accessors with no substitution keep their own null and undefined", () => {
		const points = [
			{ label: "null", dataKey: () => null },
			{ label: "undefined", dataKey: () => undefined },
			{ label: "zero", dataKey: () => 0 },
		]
		const [projected] = tableRows([{ created: 1 }], points, false)
		expect(projected.cells.map((c) => c.value)).toEqual([null, null, 0])
	})
})
