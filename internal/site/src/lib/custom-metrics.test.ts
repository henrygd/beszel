import { expect, mock, test } from "bun:test"
import {
	customMetricCharts,
	customMetricColor,
	customMetricKeys,
	customMetricLabel,
	customMetricLegend,
	customMetricLines,
	customStatsPoint,
	formatCustomMetric,
	formatCustomMetricValue,
	hasCustomMetrics,
	systemHasCustomMetrics,
	unitSymbol,
	type UnitConverters,
} from "./custom-metrics"
import { Unit } from "./enums"
import type { SystemInfo, SystemStats } from "@/types"

const system = (cmm?: SystemInfo["cmm"], cmr?: SystemInfo["cmr"]) => ({ info: { cmm, cmr } as SystemInfo })
/** A custom_stats record: its stats are the values by key. */
const record = (values: Record<string, number> = {}) => ({ stats: values })
const gap = { stats: null }
/** One record with a value for every key, so each key has data in range. */
const valuesFor = (...keys: string[]) => record(Object.fromEntries(keys.map((key) => [key, 1])))
/** The charts for this metadata, with a value in range for every key in it. */
const chartsFor = (cmm: NonNullable<SystemInfo["cmm"]>) =>
	customMetricCharts(system(cmm), [valuesFor(...Object.keys(cmm))])

test("keys are the series with a value in the visible records", () => {
	const keys = customMetricKeys([record({ b_watts: 1, a_watts: 2 }), gap, record({ old_only: 3 }), record()])
	expect(keys).toEqual(["a_watts", "b_watts", "old_only"])
})

test("a series the agent reports, but with no data in range, gets no chart", () => {
	const s = system({ live: { c: "Live" }, stopped: { c: "Stopped" } })
	expect(customMetricCharts(s, [valuesFor("live")]).map((chart) => chart.title)).toEqual(["Live"])
	expect(customMetricCharts(s, [])).toEqual([])
})

test("gap records and missing data do not throw", () => {
	const nullInfo = system()
	// Exercise malformed stored data without widening the wire type.
	Reflect.set(nullInfo.info, "cmm", null)
	Reflect.set(nullInfo.info, "cmr", null)
	expect(customMetricKeys([gap, gap])).toEqual([])
	expect(customMetricCharts(nullInfo, [gap, valuesFor("k")])).toEqual([
		{ title: "k", keys: ["k"], unit: "", description: undefined },
	])
	expect(customMetricLabel(nullInfo, "k")).toBe("k")
	expect(customMetricLabel({ info: undefined as unknown as SystemInfo }, "k")).toBe("k")
	expect(customMetricKeys([])).toEqual([])
})

test("hasCustomMetrics is true only with data in range", () => {
	expect(hasCustomMetrics([])).toBe(false)
	expect(hasCustomMetrics([gap, record(), record({})])).toBe(false)
	expect(hasCustomMetrics([gap, record({ a: 1 })])).toBe(true)
})

test("names come from what the agent reports, else from what the hub kept", () => {
	const s = system(
		{ live: { u: "watts", l: "Live power", c: "Power" }, both: { l: "Current name", c: "Power" } },
		{
			gone: { u: "volts", l: "Old voltage", c: "Power", h: "A removed series.", t: 1 },
			both: { l: "Kept name", c: "Old chart", t: 1 },
			lone: { u: "bytes", c: "Disk", cd: "Kept description", t: 1 },
		}
	)
	const charts = customMetricCharts(s, [valuesFor("live", "both", "gone", "lone")])
	expect(charts).toEqual([
		{ title: "Disk", keys: ["lone"], unit: "bytes", description: "Kept description" },
		{ title: "Power", keys: ["both", "gone", "live"], unit: undefined, description: undefined },
	])
	expect(customMetricLabel(s, "gone")).toBe("Old voltage")
	expect(customMetricLabel(s, "both")).toBe("Current name")
	expect(customMetricLines(s, ["gone"])[0].unit).toBe("volts")
})

test("charts group keys by the agent's chart title, sorted by title", () => {
	const charts = customMetricCharts(
		system({
			wall_watts: { u: "watts", c: "Power consumption" },
			board_watts: { u: "watts", c: "Power consumption" },
			core_volts: { u: "volts", c: "Power consumption" },
			temp: { u: "celsius", c: "Disk temp" },
			queue: { c: "Queue depth" },
			jobs: { u: "/s", c: "Queue depth" },
			no_chart: { u: "watts" },
			stopped: { c: "Stopped" },
		}),
		[
			valuesFor("wall_watts", "board_watts", "core_volts", "temp", "queue", "jobs", "no_chart"),
			record({ history_only: 5 }),
		]
	)
	expect(charts).toEqual([
		{ title: "Disk temp", keys: ["temp"], unit: "celsius", description: undefined },
		{ title: "history_only", keys: ["history_only"], unit: "", description: undefined },
		{ title: "no_chart", keys: ["no_chart"], unit: "watts", description: undefined },
		{
			title: "Power consumption",
			keys: ["board_watts", "core_volts", "wall_watts"],
			unit: undefined,
			description: undefined,
		},
		{ title: "Queue depth", keys: ["jobs", "queue"], unit: undefined, description: undefined },
	])
})

test("a chart's unit is set only when all its series share it", () => {
	const [chart] = chartsFor({ a: { u: "watts", c: "P" }, b: { u: "watts", c: "P" } })
	expect(chart.unit).toBe("watts")
	const [mixed] = chartsFor({ a: { u: "watts", c: "P" }, b: { c: "P" } })
	expect(mixed.unit).toBeUndefined()
	const [unitless] = chartsFor({ a: { c: "P" }, b: { c: "P" } })
	expect(unitless.unit).toBe("")
})

test("labels come from metadata, falling back to the key", () => {
	const s = system({ pi_power_board_watts: { u: "watts", l: "Board power" }, bare: { u: "watts" }, empty: { l: "" } })
	expect(customMetricLabel(s, "pi_power_board_watts")).toBe("Board power")
	expect(customMetricLabel(s, "bare")).toBe("bare")
	expect(customMetricLabel(s, "empty")).toBe("empty")
	expect(customMetricLabel(s, "unknown")).toBe("unknown")
})

test("unit symbols", () => {
	expect(unitSymbol("watts")).toBe("W")
	expect(unitSymbol("volts")).toBe("V")
	expect(unitSymbol("amperes")).toBe("A")
	expect(unitSymbol("joules")).toBe("J")
	expect(unitSymbol("bytes")).toBe("B")
	expect(unitSymbol("seconds")).toBe("s")
	expect(unitSymbol("celsius")).toBe("°C")
	expect(unitSymbol("hertz")).toBe("Hz")
	expect(unitSymbol("percent")).toBe("%")
	expect(unitSymbol("ratio")).toBe("")
	expect(unitSymbol("bytes/s")).toBe("B/s")
	expect(unitSymbol("joules/s")).toBe("J/s")
	expect(unitSymbol("/s")).toBe("/s")
	expect(unitSymbol("lpm")).toBe("lpm")
	expect(unitSymbol("W")).toBe("W")
	expect(unitSymbol("")).toBe("")
})

test("values keep small magnitudes visible", () => {
	expect(formatCustomMetricValue(1.8437)).toBe("1.84")
	expect(formatCustomMetricValue(450)).toBe("450")
	expect(formatCustomMetricValue(12345678)).toBe("12345678")
	expect(formatCustomMetricValue(-2.555)).toBe("-2.56")
	expect(formatCustomMetricValue(0)).toBe("0")
	expect(formatCustomMetricValue(0.5)).toBe("0.5")
	expect(formatCustomMetricValue(0.0012)).toBe("0.0012")
	expect(formatCustomMetricValue(0.123456)).toBe("0.123")
	expect(formatCustomMetricValue(-0.0456789)).toBe("-0.0457")
})

test("colors are well apart within a chart, however alike the keys", () => {
	const hue = (index: number) => Number(/^hsl\((\d+), 65%, 52%\)$/.exec(customMetricColor(index))?.[1])
	const gap = (a: number, b: number) => Math.min(Math.abs(a - b), 360 - Math.abs(a - b))
	expect(hue(0)).toBe(226)
	// Colors depend on position, not on keys such as disk_temp_celsius_sda and _sdb.
	for (let i = 0; i < 30; i++) {
		expect(gap(hue(i), hue(i + 1))).toBeGreaterThanOrEqual(130)
	}
	// Any two of the first 12 series, as many as a legend shows, are at least 20° apart.
	for (let i = 0; i < 12; i++) {
		for (let j = i + 1; j < 12; j++) {
			expect(gap(hue(i), hue(j))).toBeGreaterThanOrEqual(20)
		}
	}
})

test("a chart's description is its configured one, else the HELP of its only series", () => {
	const charts = chartsFor({
		board: { c: "Power", h: "Board power.", cd: "Measured and estimated" },
		wall: { c: "Power", h: "Wall power." },
		temp: { c: "Disk temp", h: "Drive temperature." },
		queue: { c: "Queue", h: "Jobs waiting." },
		jobs: { c: "Queue", h: "Jobs running." },
		bare: { c: "Bare" },
	})
	const description = Object.fromEntries(charts.map((chart) => [chart.title, chart.description]))
	expect(description).toEqual({
		Power: "Measured and estimated",
		"Disk temp": "Drive temperature",
		Queue: undefined,
		Bare: undefined,
	})
})

test("a HELP subtitle loses one final period, as other subtitles have none", () => {
	const description = (h: string) => chartsFor({ a: { c: "A", h } })[0].description
	expect(description("Jobs waiting.")).toBe("Jobs waiting")
	expect(description("Jobs waiting")).toBe("Jobs waiting")
	expect(description("Still counting...")).toBe("Still counting...")
	// The operator's own description is shown as written.
	expect(chartsFor({ a: { c: "A", cd: "Configured." } })[0].description).toBe("Configured.")
})

test("a legend shows for 2 to 11 series", () => {
	const chart = (count: number) =>
		chartsFor(Object.fromEntries(Array.from({ length: count }, (_, i) => [`k${i}`, { c: "C" }])))[0]
	expect([1, 2, 11, 12].map((count) => customMetricLegend(chart(count)))).toEqual([false, true, true, false])
})

test("a line's value leaves a gap for missing data and keeps a real zero", () => {
	const [line] = customMetricLines(system({ a: { u: "watts" } }), ["a"])
	expect(line.value(gap)).toBeUndefined()
	expect(line.value(record())).toBeUndefined()
	expect(line.value(record({ b: 1 }))).toBeUndefined()
	expect(line.value(record({ a: 0 }))).toBe(0)
	expect(line.value(record({ a: 1.84 }))).toBe(1.84)
})

test("each line has its own label, color and unit", () => {
	const lines = customMetricLines(system({ a: { u: "watts", l: "Board power" }, b: { u: "bytes/s" } }), [
		"a",
		"b",
		"bare",
	])
	expect(lines.map(({ key, label, color, unit }) => ({ key, label, color, unit }))).toEqual([
		{ key: "a", label: "Board power", color: customMetricColor(0), unit: "watts" },
		{ key: "b", label: "b", color: customMetricColor(1), unit: "bytes/s" },
		{ key: "bare", label: "bare", color: customMetricColor(2), unit: "" },
	])
})

test("the axis shows a unit only when every series shares it", () => {
	const [shared, mixed] = chartsFor({
		a: { c: "A", u: "watts" },
		b: { c: "A", u: "watts" },
		v: { c: "B", u: "volts" },
		w: { c: "B", u: "watts" },
	})
	expect(shared.unit).toBe("watts")
	expect(mixed.unit).toBeUndefined()
})

/** Unit settings never saved, as for most users: the store has no default disk unit. */
const unset = {}

/**
 * Stand-ins for lib/utils' converters, which this test cannot load: they report how
 * they were called, so the tests pin which converter and which setting each unit uses.
 */
const formatBytes = mock((size: number, perSecond = false, unit = Unit.Bytes) => ({
	value: size / 1000,
	unit: `${unit === Unit.Bits ? "kb" : "kB"}${perSecond ? "/s" : ""}`,
}))
const formatTemperature = mock((celsius: number, unit?: Unit) => ({
	value: unit === Unit.Fahrenheit ? celsius * 1.8 + 32 : celsius,
	unit: unit === Unit.Fahrenheit ? "°F" : "°C",
}))
const convert = { formatBytes, formatTemperature } as unknown as UnitConverters

test("a value is shown with its unit's symbol, and with no trailing space without one", () => {
	expect(formatCustomMetric(1.8437, "watts", unset, convert)).toBe("1.84 W")
	expect(formatCustomMetric(1.2, "joules/s", unset, convert)).toBe("1.2 J/s")
	expect(formatCustomMetric(1.2, "/s", unset, convert)).toBe("1.2/s")
	expect(formatCustomMetric(0.0012, "ratio", unset, convert)).toBe("0.0012")
	expect(formatCustomMetric(3, "lpm", unset, convert)).toBe("3 lpm")
	expect(formatCustomMetric(0.0012, "", unset, convert)).toBe("0.0012")
})

test("bytes are sizes, scaled in bytes whatever the disk unit, as disk usage is", () => {
	formatBytes.mockClear()
	expect(formatCustomMetric(123456, "bytes", { unitDisk: Unit.Bits }, convert)).toBe("123.46 kB")
	expect(formatBytes).toHaveBeenLastCalledWith(123456, false, Unit.Bytes)
})

test("byte rates follow the disk unit, as disk I/O does", () => {
	expect(formatCustomMetric(12345, "bytes/s", unset, convert)).toBe("12.35 kB/s")
	expect(formatBytes).toHaveBeenLastCalledWith(12345, true, undefined)
	expect(formatCustomMetric(12345, "bytes/s", { unitDisk: Unit.Bits }, convert)).toBe("12.35 kb/s")
	expect(formatBytes).toHaveBeenLastCalledWith(12345, true, Unit.Bits)
})

test("temperatures follow the temperature unit, Celsius when unset", () => {
	expect(formatCustomMetric(45.25, "celsius", unset, convert)).toBe("45.25 °C")
	expect(formatTemperature).toHaveBeenLastCalledWith(45.25, Unit.Celsius)
	expect(formatCustomMetric(45.25, "celsius", { unitTemp: Unit.Fahrenheit }, convert)).toBe("113.45 °F")
	expect(formatTemperature).toHaveBeenLastCalledWith(45.25, Unit.Fahrenheit)
})

test("other units never reach the converters", () => {
	formatBytes.mockClear()
	formatTemperature.mockClear()
	for (const unit of ["watts", "joules/s", "bytes_total", "celsius/s", "B", ""])
		formatCustomMetric(1, unit, unset, convert)
	expect(formatBytes).not.toHaveBeenCalled()
	expect(formatTemperature).not.toHaveBeenCalled()
})

test("only systems with custom metrics fetch their history", () => {
	expect(systemHasCustomMetrics(system())).toBe(false)
	expect(systemHasCustomMetrics(system({}, {}))).toBe(false)
	expect(systemHasCustomMetrics({ info: undefined as unknown as SystemInfo })).toBe(false)
	expect(systemHasCustomMetrics(system({ a: { c: "A" } }))).toBe(true)
	expect(systemHasCustomMetrics(system(undefined, { stopped: { c: "Stopped", t: 1 } }))).toBe(true)
})

test("a live-view point carries the realtime message's values, or is null without them", () => {
	const point = customStatsPoint(5, { cm: { a: 1.5 } } as unknown as SystemStats)
	expect(point?.created).toBe(5)
	expect(point?.stats).toEqual({ a: 1.5 })
	expect(customStatsPoint(5, {} as SystemStats)).toBeNull()
	expect(customStatsPoint(5, { cm: {} } as unknown as SystemStats)).toBeNull()
})
