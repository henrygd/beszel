import type { CustomMetricMeta, CustomStatsRecord, SystemRecord, SystemStats, UserSettings } from "@/types"
import { Unit } from "./enums"
import type { formatBytes, formatTemperature } from "./utils"

type MetricsSystem = Pick<SystemRecord, "info">
// custom_stats records, whose stats are the values by key. Time gaps arrive as records with `stats: null`.
type MetricsRecord = { stats: CustomStatsRecord["stats"] | null }

/**
 * Keys with a value in the visible records, sorted. A chart is drawn only while it has
 * data in the selected range, as Beszel's own charts are: a series whose producer
 * stopped keeps its chart, with a gap, until its data is out of range.
 */
export function customMetricKeys(records: MetricsRecord[]): string[] {
	const keys = new Set<string>()
	for (const record of records) {
		for (const key in record.stats ?? {}) {
			keys.add(key)
		}
	}
	return [...keys].sort((a, b) => a.localeCompare(b))
}

/**
 * Whether some visible record has a custom metric value, which decides whether the charts
 * and the Custom tab show. systemHasCustomMetrics decides instead whether to fetch history.
 */
export function hasCustomMetrics(records: MetricsRecord[]): boolean {
	return records.some((record) => Object.keys(record.stats ?? {}).length > 0)
}

/**
 * Whether the system has custom metrics to fetch history for: names the agent reports
 * now, or names the hub kept for series that stopped. Systems without them skip the
 * custom_stats request.
 */
export function systemHasCustomMetrics(system: MetricsSystem): boolean {
	return Object.keys(system.info?.cmm ?? {}).length > 0 || Object.keys(system.info?.cmr ?? {}).length > 0
}

/** A live-view point from the values in a realtime message, or null when it has none. */
export function customStatsPoint(created: number, stats: SystemStats): CustomStatsRecord | null {
	return stats?.cm && Object.keys(stats.cm).length > 0 ? ({ created, stats: stats.cm } as CustomStatsRecord) : null
}

/**
 * A series' metadata: what the agent reports now, else what the hub kept after the agent
 * stopped reporting it, so its history keeps its title and unit.
 */
function customMetricMeta(system: MetricsSystem, key: string): CustomMetricMeta | undefined {
	return system.info?.cmm?.[key] ?? system.info?.cmr?.[key]
}

export interface CustomMetricChart {
	title: string
	keys: string[]
	/** the unit every series in the chart shares, or undefined when they differ */
	unit?: string
	/** the chart's configured description, else the # HELP text of its only series, less a final period */
	description?: string
}

/** # HELP text as a subtitle, without a final period, as Beszel's other subtitles have none. */
const helpSubtitle = (help?: string) => help?.replace(/(?<!\.)\.$/, "")

/**
 * Keys grouped into charts by the title the agent resolved: series with the same title
 * share a chart, whatever their units. A key without metadata gets a chart of its own,
 * titled with the key. Charts are sorted by title, and keys within a chart.
 */
export function customMetricCharts(system: MetricsSystem, records: MetricsRecord[]): CustomMetricChart[] {
	const charts = new Map<string, string[]>()
	for (const key of customMetricKeys(records)) {
		const title = customMetricMeta(system, key)?.c || key
		const keys = charts.get(title)
		if (keys) {
			keys.push(key)
		} else {
			charts.set(title, [key])
		}
	}
	return [...charts]
		.sort(([a], [b]) => a.localeCompare(b))
		.map(([title, keys]) => {
			const units = new Set(keys.map((key) => customMetricUnit(system, key)))
			const description =
				keys.map((key) => customMetricMeta(system, key)?.cd).find(Boolean) ||
				(keys.length === 1 ? helpSubtitle(customMetricMeta(system, keys[0])?.h) : undefined) ||
				undefined
			return { title, keys, unit: units.size === 1 ? [...units][0] : undefined, description }
		})
}

/** One line of a custom metric chart. */
export interface CustomMetricLine {
	key: string
	label: string
	color: string
	/** the line's unit, which picks how the tooltip formats its values */
	unit: string
	/** the line's value in a record; undefined leaves a gap, so a stale producer never reads as zero */
	value: (record: MetricsRecord) => number | undefined
}

/** The lines of a chart, one per key, colored by position. */
export function customMetricLines(system: MetricsSystem, keys: string[]): CustomMetricLine[] {
	return keys.map((key, i) => ({
		key,
		label: customMetricLabel(system, key),
		color: customMetricColor(i),
		unit: customMetricUnit(system, key),
		value: ({ stats }) => stats?.[key],
	}))
}

/**
 * A legend only for 2 to 11 series: one would repeat the chart's title, and 12 or more
 * would fill the card, as the GPU power and temperature charts decide.
 */
export function customMetricLegend(chart: CustomMetricChart): boolean {
	return chart.keys.length > 1 && chart.keys.length < 12
}

/** Unit from the series' metadata, or "" when unknown. */
export function customMetricUnit(system: MetricsSystem, key: string): string {
	return customMetricMeta(system, key)?.u ?? ""
}

/** Display label from the series' metadata, falling back to the key. */
export function customMetricLabel(system: MetricsSystem, key: string): string {
	return customMetricMeta(system, key)?.l || key
}

const unitSymbols: Record<string, string> = {
	watts: "W",
	volts: "V",
	amperes: "A",
	joules: "J",
	bytes: "B",
	seconds: "s",
	celsius: "°C",
	hertz: "Hz",
	percent: "%",
	ratio: "",
}

/** Short symbol for a unit, keeping a rate's "/s" suffix; unknown units are unchanged. */
export function unitSymbol(unit: string): string {
	const rate = unit.endsWith("/s")
	const base = rate ? unit.slice(0, -2) : unit
	return (unitSymbols[base] ?? base) + (rate ? "/s" : "")
}

/**
 * Two decimals from 1 upward, as other charts use, and three significant digits below,
 * so small ratios stay visible, which toFixedFloat would round to zero.
 */
export function formatCustomMetricValue(value: number): string {
	const rounded = value === 0 || Math.abs(value) >= 1 ? value.toFixed(2) : value.toPrecision(3)
	return String(Number.parseFloat(rounded))
}

/** The unit settings custom charts follow, as Beszel's own charts do. */
export type CustomMetricUnitSettings = Pick<UserSettings, "unitTemp" | "unitDisk">

/**
 * Beszel's own converters from lib/utils, passed in rather than imported: lib/utils loads
 * the app's stores, and through them the translations a build compiles, so importing it
 * would keep bun test from loading this file in a fresh checkout.
 */
export interface UnitConverters {
	formatBytes: typeof formatBytes
	formatTemperature: typeof formatTemperature
}

/**
 * A value as axis ticks and tooltips show it, given its unit ("" when unknown). Units
 * Beszel already formats are shown as its own charts show them: bytes scale to KB, MB
 * and up, as disk usage does; byte rates follow the disk unit setting, as disk I/O does;
 * temperatures follow the temperature setting. Any other unit is shown as written, by
 * its symbol. A bare rate attaches: 1.2/s.
 */
export function formatCustomMetric(
	value: number,
	unit: string,
	settings: CustomMetricUnitSettings,
	convert: UnitConverters
): string {
	let scaled: { value: number; unit: string } | undefined
	if (unit === "bytes") scaled = convert.formatBytes(value, false, Unit.Bytes)
	else if (unit === "bytes/s") scaled = convert.formatBytes(value, true, settings.unitDisk)
	else if (unit === "celsius") scaled = convert.formatTemperature(value, settings.unitTemp ?? Unit.Celsius)
	const formatted = formatCustomMetricValue(scaled?.value ?? value)
	const symbol = scaled?.unit ?? unitSymbol(unit)
	if (!symbol) return formatted
	return symbol.startsWith("/") ? formatted + symbol : `${formatted} ${symbol}`
}

/**
 * Color for the series at an index within its chart: from the blue the GPU charts start
 * with, each next series turns the hue by the golden angle. Neighbouring series are about
 * 138° apart however alike their keys, and a series added after the others leaves their
 * colors unchanged. (Hashing the key, as wifiColor does, put keys that differ only in
 * their last character, such as cpu_core_0 and cpu_core_1, 1° apart.)
 */
export function customMetricColor(index: number): string {
	return `hsl(${Math.round((226 + index * 137.508) % 360)}, 65%, 52%)`
}
