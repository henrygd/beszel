import { describe, expect, test } from "bun:test"

import {
	containerChartConfigs,
	effectiveSeriesLevel,
	pointsHaveStacks,
	rollupContainerPoints,
	type SeriesLevel,
	seriesColor,
	seriesHue,
	seriesKey,
} from "../src/lib/container-series"
import type { ChartDataContainer, ContainerStats } from "../src/types"

type Stat = {
	n: string
	st?: string
	sv?: string
	c?: number
	m?: number
	ns?: number
	nr?: number
	b?: [number, number]
}

/** Build a chart point the same way makeContainerPoint does: stats keyed by container name */
function point(created: number | null, stats: Stat[] = []): ChartDataContainer {
	const result: Record<string, unknown> = { created }
	for (const stat of stats) {
		result[stat.n] = stat
	}
	return result as ChartDataContainer
}

/** Stats of a rolled up point, by series key */
function series(p: ChartDataContainer, key: string): ContainerStats {
	return (p as unknown as Record<string, ContainerStats>)[key]
}

/** Series keys of a point, without the timestamp */
function keys(p: ChartDataContainer): string[] {
	return Object.keys(p).filter((key) => key !== "created")
}

const MB = 1024 * 1024

describe("seriesKey", () => {
	test("uses the container name at the container level", () => {
		expect(seriesKey({ n: "mentors_web.1.abc", st: "mentors", sv: "web" }, "container")).toBe("mentors_web.1.abc")
	})

	test("service keys are prefixed with the stack", () => {
		expect(seriesKey({ n: "web_api.1.a", st: "web", sv: "api" }, "service")).toBe("web/api")
		expect(seriesKey({ n: "admin_api.1.a", st: "admin", sv: "api" }, "service")).toBe("admin/api")
		// docker service create without a stack: a service name, no namespace label
		expect(seriesKey({ n: "api.1.a", sv: "api" }, "service")).toBe("api")
	})

	test("falls back to the container name without labels", () => {
		expect(seriesKey({ n: "redis" }, "service")).toBe("redis")
		expect(seriesKey({ n: "redis" }, "stack")).toBe("redis")
		expect(seriesKey({ n: "redis", sv: "cache" }, "stack")).toBe("redis")
	})

	test("uses the stack at the stack level", () => {
		expect(seriesKey({ n: "mentors_web.1.abc", st: "mentors", sv: "web" }, "stack")).toBe("mentors")
	})
})

describe("rollupContainerPoints", () => {
	test("sums containers of one service on a short range", () => {
		const points = [60_000, 120_000, 180_000].map((created, i) =>
			point(created, [
				{ n: `mentors_web.1.a`, st: "mentors", sv: "web", c: 1 + i, m: 100, b: [10, 20] },
				{ n: `mentors_web.2.b`, st: "mentors", sv: "web", c: 2 + i, m: 200, b: [1, 2] },
			])
		)
		const rolled = rollupContainerPoints(points, "service")
		expect(rolled.length).toBe(3)
		for (let i = 0; i < rolled.length; i++) {
			expect(keys(rolled[i])).toEqual(["mentors/web"])
			const stats = series(rolled[i], "mentors/web")
			expect(stats.c).toBe(3 + 2 * i)
			expect(stats.m).toBe(300)
			expect(stats.b).toEqual([11, 22])
			expect(stats.st).toBe("mentors")
			expect(stats.sv).toBe("web")
			expect(rolled[i].created).toBe(points[i].created)
		}
	})

	test("sums averaged records without re-averaging", () => {
		// 480 minute records: the hub already averaged each window, the client only adds them up
		const step = 480 * 60_000
		const points = [step, 2 * step].map((created) =>
			point(created, [
				{ n: "mentors_web.1.a", st: "mentors", sv: "web", c: 2.5, m: 150.5, b: [5, 5] },
				{ n: "mentors_db.1.a", st: "mentors", sv: "db", c: 0.5, m: 49.5, b: [1, 1] },
			])
		)
		const rolled = rollupContainerPoints(points, "stack")
		for (const p of rolled) {
			expect(keys(p)).toEqual(["mentors"])
			const stats = series(p, "mentors")
			expect(stats.c).toBeCloseTo(3, 10)
			expect(stats.m).toBeCloseTo(200, 10)
			expect(stats.b).toEqual([6, 6])
		}
	})

	test("records without stack fields stay at the container level", () => {
		const points = [
			point(60_000, [
				{ n: "mentors_web.1.a", st: "mentors", sv: "web", c: 1, m: 10, b: [1, 1] },
				{ n: "plain-redis", c: 2, m: 20, b: [2, 2] },
				{ n: "plain-pg", c: 3, m: 30, b: [3, 3] },
			]),
		]
		for (const level of ["service", "stack"] as SeriesLevel[]) {
			const rolled = rollupContainerPoints(points, level)
			const rolledKeys = keys(rolled[0]).sort()
			expect(rolledKeys).toEqual(["plain-pg", "plain-redis", level === "stack" ? "mentors" : "mentors/web"].sort())
			expect(series(rolled[0], "plain-redis").c).toBe(2)
			expect(series(rolled[0], "plain-pg").m).toBe(30)
			// a fallback series carries no group labels
			expect(series(rolled[0], "plain-redis").st).toBeUndefined()
			expect(series(rolled[0], "plain-redis").sv).toBeUndefined()
		}
	})

	test("keeps gap markers and omits a group with no members at a timestamp", () => {
		const points = [
			point(60_000, [{ n: "mentors_web.1.a", st: "mentors", sv: "web", c: 1, m: 10, b: [1, 1] }]),
			point(null),
			point(180_000, [{ n: "other_web.1.a", st: "other", sv: "web", c: 1, m: 10, b: [1, 1] }]),
		]
		const rolled = rollupContainerPoints(points, "stack")
		expect(rolled.length).toBe(3)
		expect(rolled[1]).toBe(points[1])
		expect(keys(rolled[1])).toEqual([])
		// the stack that has no container at this timestamp gets no key at all, not a zero
		expect(keys(rolled[2])).toEqual(["other"])
		expect(series(rolled[2], "mentors")).toBeUndefined()
	})

	test("container level returns the same array reference", () => {
		const points = [point(60_000, [{ n: "mentors_web.1.a", st: "mentors", sv: "web", c: 1, m: 10 }])]
		expect(rollupContainerPoints(points, "container")).toBe(points)
	})

	test("network bytes fall back to the deprecated megabyte fields", () => {
		const points = [
			point(60_000, [
				{ n: "mentors_web.1.a", st: "mentors", sv: "web", c: 1, m: 10, ns: 2, nr: 3 },
				{ n: "mentors_web.2.b", st: "mentors", sv: "web", c: 1, m: 10, ns: 1, nr: 1 },
			]),
		]
		const stats = series(rollupContainerPoints(points, "service")[0], "mentors/web")
		expect(stats.b).toEqual([3 * MB, 4 * MB])
		expect(stats.ns).toBeUndefined()
		expect(stats.nr).toBeUndefined()
	})

	test("mixes byte and megabyte records in one group", () => {
		const points = [
			point(60_000, [
				{ n: "mentors_web.1.a", st: "mentors", sv: "web", c: 1, m: 10, b: [MB, MB] },
				{ n: "mentors_web.2.b", st: "mentors", sv: "web", c: 1, m: 10, ns: 1, nr: 1 },
			]),
		]
		expect(series(rollupContainerPoints(points, "service")[0], "mentors/web").b).toEqual([2 * MB, 2 * MB])
	})
})

describe("effectiveSeriesLevel", () => {
	const noStacks = [point(60_000, [{ n: "redis", c: 1, m: 10 }]), point(120_000, [{ n: "redis", c: 1, m: 10 }])]
	const withStacks = [point(60_000, [{ n: "mentors_web.1.a", st: "mentors", sv: "web", c: 1, m: 10 }])]

	test("falls back to the container level when the data has no stacks", () => {
		expect(effectiveSeriesLevel(noStacks, "stack")).toBe("container")
		expect(effectiveSeriesLevel(noStacks, "service")).toBe("container")
		expect(rollupContainerPoints(noStacks, effectiveSeriesLevel(noStacks, "stack"))).toBe(noStacks)
	})

	// The point of the fallback is that someone without stacks sees exactly what they saw before,
	// so the expectation is written out here rather than taken from another call into the same
	// function: a change in the key order, the label or the hue formula has to fail this test.
	test("the fallback config is the one the charts built before", () => {
		const points = [
			point(60_000, [
				{ n: "redis", c: 1, m: 10, b: [1, 1] },
				{ n: "api", c: 4, m: 40, b: [4, 4] },
			]),
		]
		const colour = (index: number) =>
			`hsl(${((index * 360) / 2) % 360}, var(--chart-saturation), var(--chart-lightness))`
		const expected = {
			api: { label: "api", color: colour(0) },
			redis: { label: "redis", color: colour(1) },
		}
		for (const requested of ["stack", "service"] as SeriesLevel[]) {
			const configs = containerChartConfigs(points, effectiveSeriesLevel(points, requested))
			expect(Object.keys(configs.cpu)).toEqual(["api", "redis"])
			expect(configs.cpu).toEqual(expected)
			expect(configs.memory).toEqual(expected)
			expect(configs.network).toEqual(expected)
		}
	})

	test("keeps the requested level when the data has stacks", () => {
		expect(effectiveSeriesLevel(withStacks, "stack")).toBe("stack")
		expect(effectiveSeriesLevel(withStacks, "container")).toBe("container")
	})

	test("ignores gap markers while looking for stacks", () => {
		expect(pointsHaveStacks([point(null), ...noStacks])).toBe(false)
		expect(pointsHaveStacks([point(null), ...withStacks])).toBe(true)
		expect(pointsHaveStacks([])).toBe(false)
	})
})

describe("colors", () => {
	test("series color is stable for a group name", () => {
		const web = seriesColor("web")
		expect(seriesColor("web")).toBe(web)
		expect(seriesColor("db")).not.toBe(web)
		expect(seriesHue("web")).toBeGreaterThanOrEqual(0)
		expect(seriesHue("web")).toBeLessThan(360)
		expect(web).toBe(`hsl(${seriesHue("web")}, var(--chart-saturation), var(--chart-lightness))`)
	})

	test("container level keeps the rank based hue", () => {
		const points = [
			point(60_000, [
				{ n: "low", c: 1, m: 1, b: [1, 1] },
				{ n: "high", c: 10, m: 10, b: [10, 10] },
				{ n: "mid", c: 5, m: 5, b: [5, 5] },
			]),
		]
		const configs = containerChartConfigs(points, "container")
		const ranked = ["high", "mid", "low"]
		expect(Object.keys(configs.cpu)).toEqual(ranked)
		for (let i = 0; i < ranked.length; i++) {
			const hue = ((i * 360) / ranked.length) % 360
			expect(configs.cpu[ranked[i]].color).toBe(`hsl(${hue}, var(--chart-saturation), var(--chart-lightness))`)
			expect(configs.cpu[ranked[i]].label).toBe(ranked[i])
		}
	})

	test("group levels colour by name, not by rank", () => {
		const twoStacks = rollupContainerPoints(
			[
				point(60_000, [
					{ n: "a_web.1.a", st: "a", sv: "web", c: 1, m: 1, b: [1, 1] },
					{ n: "b_web.1.a", st: "b", sv: "web", c: 9, m: 9, b: [9, 9] },
				]),
			],
			"stack"
		)
		const threeStacks = rollupContainerPoints(
			[
				point(60_000, [
					{ n: "a_web.1.a", st: "a", sv: "web", c: 1, m: 1, b: [1, 1] },
					{ n: "b_web.1.a", st: "b", sv: "web", c: 9, m: 9, b: [9, 9] },
					{ n: "c_web.1.a", st: "c", sv: "web", c: 5, m: 5, b: [5, 5] },
				]),
			],
			"stack"
		)
		const two = containerChartConfigs(twoStacks, "stack")
		const three = containerChartConfigs(threeStacks, "stack")
		for (const key of ["a", "b"]) {
			expect(two.cpu[key].color).toBe(seriesColor(key))
			expect(three.cpu[key].color).toBe(two.cpu[key].color)
		}
		expect(three.cpu.c.color).toBe(seriesColor("c"))
	})

	test("fallback series are coloured by rank on group levels", () => {
		const points = rollupContainerPoints(
			[
				point(60_000, [
					{ n: "a_web.1.a", st: "a", sv: "web", c: 1, m: 1, b: [1, 1] },
					{ n: "plain", c: 9, m: 9, b: [9, 9] },
				]),
			],
			"stack"
		)
		const configs = containerChartConfigs(points, "stack")
		// "plain" has no labels, so it keeps the rank hue instead of the group hash
		expect(configs.cpu.plain.color).toBe(`hsl(0, var(--chart-saturation), var(--chart-lightness))`)
		expect(configs.cpu.a.color).toBe(seriesColor("a"))
	})

	test("ranks groups by total usage for the stacking order", () => {
		const points = rollupContainerPoints(
			[
				point(60_000, [
					{ n: "a_web.1.a", st: "a", sv: "web", c: 1, m: 100, b: [0, 0] },
					{ n: "b_web.1.a", st: "b", sv: "web", c: 5, m: 1, b: [0, 0] },
				]),
				point(120_000, [
					{ n: "a_web.1.a", st: "a", sv: "web", c: 1, m: 100, b: [0, 0] },
					{ n: "b_web.1.a", st: "b", sv: "web", c: 5, m: 1, b: [9, 9] },
				]),
			],
			"stack"
		)
		const configs = containerChartConfigs(points, "stack")
		expect(Object.keys(configs.cpu)).toEqual(["b", "a"])
		expect(Object.keys(configs.memory)).toEqual(["a", "b"])
		expect(Object.keys(configs.network)).toEqual(["b", "a"])
	})
})
