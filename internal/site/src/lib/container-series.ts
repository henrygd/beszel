/**
 * Series of the container charts: which containers are folded into one line, what that line is
 * called and which colour it gets.
 *
 * Pure functions only — no React, no stores, no lingui — so the whole rollup and the colour
 * assignment can be tested without a renderer. The stack and service labels are read through the
 * getters of {@link module:lib/container-grouping}, which the container table uses as well, so
 * both read the same fields (`stack`/`service` on records, `st`/`sv` on stats entries).
 *
 * Known limits:
 * - Averaged records written by a hub without container groups carry no `st`/`sv` at all, so on
 *   long ranges the group series start at the point where that hub was rolled out.
 * - A container without labels keeps its own series under its name. If that name happens to equal
 *   a stack name or a `stack/service` key, the two series merge. Compose (`project-service-1`)
 *   and swarm (`stack_service.N.id`) names don't collide with it in practice, and a sentinel in
 *   the key would show up in the legend, so this is accepted.
 */
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartDataContainer, ContainerStats, SeriesLevel } from "@/types"
import { type ContainerGroupSource, getServiceValue, getStackValue, hasStacks } from "./container-grouping"

export type { SeriesLevel }

/** Selectable series levels, in the order they are offered */
export const SERIES_LEVELS = ["container", "service", "stack"] as const

/** Level used when nothing is stored: the charts keep the series they have always had */
export const DEFAULT_SERIES_LEVEL: SeriesLevel = "container"

/** Narrows anything (a value read back from localStorage) to a series level */
export function isSeriesLevel(value: unknown): value is SeriesLevel {
	return typeof value === "string" && (SERIES_LEVELS as readonly string[]).includes(value)
}

/** Chart configurations for CPU, memory, and network usage charts */
export interface ContainerChartConfigs {
	cpu: ChartConfig
	memory: ChartConfig
	network: ChartConfig
}

/**
 * Key of the series a container belongs to. It is also the label in the legend and the tooltip
 * and the text the chart filter matches, so it has to stay readable: no sentinel values here,
 * unlike the grouping values of the table.
 *
 * The service key carries its stack (`mentors/web`), otherwise services that share a name in
 * different stacks would end up in one series. A service without a stack is possible
 * (`docker service create` sets the service label and no namespace label) and keeps its bare name.
 */
export function seriesKey(stat: ContainerGroupSource, level: SeriesLevel): string {
	const name = stat.n ?? stat.name ?? ""
	if (level === "container") {
		return name
	}
	const stack = getStackValue(stat)
	if (level === "stack") {
		return stack || name
	}
	const service = getServiceValue(stat)
	if (!service) {
		return name
	}
	return stack ? `${stack}/${service}` : service
}

/** Stats entries of one chart point, without the timestamp */
function pointStats(point: ChartDataContainer): ContainerStats[] {
	const stats: ContainerStats[] = []
	for (const key in point) {
		if (key === "created") {
			continue
		}
		const stat = (point as unknown as Record<string, ContainerStats | undefined>)[key]
		if (stat) {
			stats.push(stat)
		}
	}
	return stats
}

/** True when at least one container in the history reports a stack */
export function pointsHaveStacks(points: ChartDataContainer[]): boolean {
	for (const point of points) {
		if (point.created === null) {
			continue
		}
		if (hasStacks(pointStats(point))) {
			return true
		}
	}
	return false
}

/**
 * The level the charts can actually draw. Without a single stack in the data there is nothing to
 * group by, so the charts stay on containers — same series, same colours, no selector.
 */
export function resolveSeriesLevel(stacksPresent: boolean, requested: SeriesLevel): SeriesLevel {
	return stacksPresent ? requested : "container"
}

/** {@link resolveSeriesLevel} for a history that has not been inspected yet */
export function effectiveSeriesLevel(points: ChartDataContainer[], requested: SeriesLevel): SeriesLevel {
	return resolveSeriesLevel(pointsHaveStacks(points), requested)
}

/** Bandwidth of a stats entry in bytes, falling back to the deprecated megabyte fields */
function networkBytes(stat: ContainerStats): [number, number] {
	return [stat.b?.[0] ?? (stat.ns ?? 0) * 1024 * 1024, stat.b?.[1] ?? (stat.nr ?? 0) * 1024 * 1024]
}

/**
 * Fold every container of a group into one stats entry per timestamp.
 *
 * Values are added up, never divided: a record of an averaged range already holds the average of
 * its window, so the sum over the containers of a group is the group's value in that window.
 * A group with no container at a timestamp gets no entry — a zero would draw a line through a
 * gap that the data does not have. Gap markers (`created: null`) are passed through untouched.
 *
 * At the container level the input array is returned as is, which keeps the chart from
 * redrawing (`AreaChartDefault` compares the array identity).
 */
export function rollupContainerPoints(points: ChartDataContainer[], level: SeriesLevel): ChartDataContainer[] {
	if (level === "container") {
		return points
	}
	const rolled: ChartDataContainer[] = []
	for (const point of points) {
		if (point.created === null) {
			rolled.push(point)
			continue
		}
		const groups = new Map<string, ContainerStats>()
		for (const stat of pointStats(point)) {
			const key = seriesKey(stat, level)
			const [sent, recv] = networkBytes(stat)
			const group = groups.get(key)
			if (group) {
				group.c += stat.c ?? 0
				group.m += stat.m ?? 0
				// biome-ignore lint/style/noNonNullAssertion: every group entry is created with b
				group.b![0] += sent
				// biome-ignore lint/style/noNonNullAssertion: every group entry is created with b
				group.b![1] += recv
				continue
			}
			const stack = getStackValue(stat)
			const service = getServiceValue(stat)
			groups.set(key, {
				n: key,
				c: stat.c ?? 0,
				m: stat.m ?? 0,
				b: [sent, recv],
				// the labels of the group, so a rolled up entry can be grouped again
				...(stack ? { st: stack } : {}),
				...(level === "service" && service ? { sv: service } : {}),
			})
		}
		const next = { created: point.created } as ChartDataContainer
		for (const [key, group] of groups) {
			;(next as unknown as Record<string, ContainerStats>)[key] = group
		}
		rolled.push(next)
	}
	return rolled
}

/** Hue of a series name: FNV-1a, so the same group keeps its colour as groups come and go */
export function seriesHue(name: string): number {
	let hash = 0x811c9dc5
	for (let i = 0; i < name.length; i++) {
		hash ^= name.charCodeAt(i)
		hash = Math.imul(hash, 0x01000193)
	}
	return (hash >>> 0) % 360
}

/** Colour of a group series, derived from its name instead of its rank */
export function seriesColor(name: string): string {
	return `hsl(${seriesHue(name)}, var(--chart-saturation), var(--chart-lightness))`
}

/** Colour of a series by rank, the formula the container charts have always used */
function rankColor(index: number, count: number): string {
	return `hsl(${((index * 360) / count) % 360}, var(--chart-saturation), var(--chart-lightness))`
}

/**
 * Chart configuration (label, colour) per series for the three container charts.
 *
 * Series are ranked by total usage on every level: the rank is the stacking order of the areas.
 * The colour follows the level — by rank on containers, which is what users see today, and by
 * name on services and stacks, so adding a container to a stack does not repaint the other
 * stacks. Containers without labels keep their own series and, having no group name worth
 * hashing, keep the rank colour too.
 */
export function containerChartConfigs(
	containerData: ChartDataContainer[],
	level: SeriesLevel = "container"
): ContainerChartConfigs {
	const configs: ContainerChartConfigs = {
		cpu: {} as ChartConfig,
		memory: {} as ChartConfig,
		network: {} as ChartConfig,
	}

	// Aggregate usage metrics for each series
	const totalUsage = {
		cpu: new Map<string, number>(),
		memory: new Map<string, number>(),
		network: new Map<string, number>(),
	}
	// Series that carry no group labels, so they are not coloured by name
	const ungrouped = new Set<string>()

	for (const point of containerData) {
		if (point.created === null) {
			continue
		}
		for (const key in point) {
			if (key === "created") {
				continue
			}
			const stats = (point as unknown as Record<string, ContainerStats | undefined>)[key]
			if (!stats) {
				continue
			}
			if (level !== "container" && !getStackValue(stats) && !getServiceValue(stats)) {
				ungrouped.add(key)
			}
			const [sentBytes, recvBytes] = networkBytes(stats)
			totalUsage.cpu.set(key, (totalUsage.cpu.get(key) ?? 0) + (stats.c ?? 0))
			totalUsage.memory.set(key, (totalUsage.memory.get(key) ?? 0) + (stats.m ?? 0))
			totalUsage.network.set(key, (totalUsage.network.get(key) ?? 0) + sentBytes + recvBytes)
		}
	}

	for (const [chartType, usageMap] of Object.entries(totalUsage)) {
		const sorted = Array.from(usageMap.entries()).sort(([, a], [, b]) => b - a)
		const chartConfig = {} as Record<string, { label: string; color: string }>
		const count = sorted.length
		for (let i = 0; i < count; i++) {
			const [key] = sorted[i]
			chartConfig[key] = {
				label: key,
				color: level === "container" || ungrouped.has(key) ? rankColor(i, count) : seriesColor(key),
			}
		}
		configs[chartType as keyof ContainerChartConfigs] = chartConfig
	}

	return configs
}
