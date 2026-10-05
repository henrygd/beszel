/** Pure projection of chart inputs. Missing samples never become zero. */
export type TablePoint<T> = { label: string; dataKey: (row: T) => unknown; activeDot?: boolean }

/** Matches the key the shared chart tooltip uses for its total row. */
export const TOTAL_KEY = "__total__"

/** One projected cell, shaped like the tooltip item the shared contentFormatter already receives. */
export type TableCell<T> = {
	key: string
	name: string
	value: number | null
	payload: T
	dataKey: TablePoint<T>["dataKey"]
}

export type TableRow<T> = { created: unknown; cells: TableCell<T>[] }

export function numericValue(value: unknown): number | null {
	return typeof value === "number" && Number.isFinite(value) ? value : null
}

/** Accessor whose graphic value may be substituted, while the raw sample stays available. */
export type ChartAccessor<A extends unknown[], R = number> = ((...args: A) => R) & {
	/** The unsubstituted sample, so the table never renders a substituted value as a measurement. */
	tableDataKey?: (...args: A) => number | null | undefined
}

/**
 * Keeps the long-standing graphic behaviour (a missing sample is drawn as `fallback`) without
 * letting that substituted value reach the table, which reports the gap instead.
 */
export function withChartFallback<A extends unknown[]>(
	sample: (...args: A) => number | null | undefined,
	fallback: number | ((...args: A) => number) = 0
): ChartAccessor<A> {
	const forGraphic: ChartAccessor<A> = (...args: A) => {
		const value = sample(...args)
		if (value == null) {
			return typeof fallback === "function" ? fallback(...args) : fallback
		}
		return value
	}
	forGraphic.tableDataKey = sample
	return forGraphic
}

/** The accessor the table must read: the raw sample when the graphic substitutes one, else the accessor itself. */
export function tableSample<A extends unknown[]>(dataKey: (...args: A) => unknown): (...args: A) => unknown {
	return (dataKey as ChartAccessor<A, unknown>).tableDataKey ?? dataKey
}

export function tableRows<T extends { created: unknown }>(
	data: readonly T[],
	points: readonly TablePoint<T>[],
	total: boolean
): TableRow<T>[] {
	const visible = points.filter((p) => p.activeDot !== false)
	return data
		.filter((row) => row.created != null)
		.map((row) => {
			const cells: TableCell<T>[] = visible.map((point) => ({
				key: point.label,
				name: point.label,
				value: numericValue(tableSample(point.dataKey)(row)),
				payload: row,
				dataKey: point.dataKey,
			}))
			if (total) {
				const values = cells.flatMap((c) => (c.value === null ? [] : [c.value]))
				cells.push({
					key: TOTAL_KEY,
					name: "Total",
					value: values.length ? values.reduce((a, b) => a + b, 0) : null,
					payload: row,
					dataKey: () => null,
				})
			}
			return { created: row.created, cells }
		})
}

export function pageRange(count: number, requestedPage: number, size = 20) {
	const pages = Math.max(1, Math.ceil(count / size))
	const page = Math.max(0, Math.min(requestedPage, pages - 1))
	return { start: page * size, end: Math.min((page + 1) * size, count), page, pages }
}

export function formatTableTime(value: unknown, locale: string, timeZone?: string) {
	if (value == null || (typeof value !== "number" && typeof value !== "string")) return null
	const date = new Date(value)
	if (!Number.isFinite(date.getTime())) return null
	return new Intl.DateTimeFormat(locale, {
		year: "numeric",
		month: "2-digit",
		day: "2-digit",
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		timeZoneName: "short",
		timeZone,
	}).format(date)
}

/** Identifies a deliberate selection: system/scope, range, filter, avg/max, units and locale. */
export function snapshotKey(selection: Record<string, unknown>) {
	return JSON.stringify(selection)
}

/** Chart inputs are immutable after ingestion. Share serialization across cards using the same history. */
const sourceSignatures = new WeakMap<readonly { created: unknown }[], string>()

export function sourceSignature(source: readonly { created: unknown }[]) {
	let signature = sourceSignatures.get(source)
	if (signature === undefined) {
		// Window bounds alone miss corrected values at unchanged timestamps.
		signature = JSON.stringify(source)
		sourceSignatures.set(source, signature)
	}
	return signature
}
