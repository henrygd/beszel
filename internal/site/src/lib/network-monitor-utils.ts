import type {
	MonitorCertInfo,
	MonitorStats,
	NetworkMonitorRecord,
	NetworkMonitorStatsRecord,
	RawMonitorStatsRecord,
} from "@/types"
import { toFixedFloat } from "./utils"

/** Derive chart metrics from the counts and response sum stored at every retention tier. */
export function getMonitorStats(record: RawMonitorStatsRecord): MonitorStats {
	const success = record.success_count > 0
	return {
		res_avg: success ? toFixedFloat(record.res_sum / record.success_count, 2) : null,
		res_min: success ? record.res_min : null,
		res_max: success ? record.res_max : null,
		loss:
			record.total_count > 0
				? toFixedFloat(((record.total_count - record.success_count) / record.total_count) * 100, 2)
				: 0,
	}
}

/**
 * Realtime stats come from the agent without counts and report 0 response times when every
 * probe failed; clear them to match stored stats.
 */
export function clearFailedResponse(stats: MonitorStats): MonitorStats {
	if (stats.loss < 100) return stats
	return { ...stats, res_avg: null, res_min: null, res_max: null }
}

/**
 * Gap marker in the same form appendData uses. Without a timestamp it can't become the active
 * tooltip point, which would otherwise have no values and make the tooltip jump to the corner.
 */
export const monitorGapRecord = { created: null, stats: null } as unknown as NetworkMonitorStatsRecord

/**
 * Return the records that have stats for one monitor, with a gap marker inserted wherever
 * consecutive records are further apart than expected (e.g. while the agent was disconnected),
 * so charts break the line there instead of drawing across the missing time.
 */
export function withMonitorGaps(
	records: NetworkMonitorStatsRecord[],
	monitor: Pick<NetworkMonitorRecord, "id" | "interval">,
	expectedInterval: number
): NetworkMonitorStatsRecord[] {
	// long-interval monitors only get a record when a new probe completes
	const maxGap = Math.max(expectedInterval, monitor.interval * 1000) * 1.5
	const result: NetworkMonitorStatsRecord[] = []
	let prevTime = 0
	for (const record of records) {
		// skip appendData's gap markers (created: null) and records without this monitor
		if (record.created == null || !record.stats?.[monitor.id]) continue
		if (prevTime && record.created - prevTime > maxGap) {
			result.push(monitorGapRecord)
		}
		prevTime = record.created
		result.push(record)
	}
	return result
}

/**
 * Records for charting several monitors together without connectNulls, so lines break at real gaps.
 *
 * Keeps the records with stats for any of the monitors. Where a monitor has no sample in a record
 * only because the record comes from another system, or falls between its long-interval probes, or
 * 1m jitter moved its sample into the next bucket, the sample is interpolated from its neighbors so
 * its line continues. Records where every probe failed keep no response time, so the response line
 * breaks there, and records further apart than every monitor's gap limit get a gap marker.
 */
export function withMonitorsGaps(
	records: NetworkMonitorStatsRecord[],
	monitors: Pick<NetworkMonitorRecord, "id" | "interval">[],
	expectedInterval: number
): NetworkMonitorStatsRecord[] {
	// long-interval monitors only get a record when a new probe completes
	const maxGaps = new Map(monitors.map((m) => [m.id, Math.max(expectedInterval, m.interval * 1000) * 1.5]))
	const rows = records.filter((record) => record.created != null && monitors.some((m) => record.stats?.[m.id] != null))
	// Copy-on-write, since records are shared with the stats cache.
	const filled = rows.slice()

	for (const m of monitors) {
		// Allow one missing bucket, which is what 1m jitter looks like.
		const fillGap = (maxGaps.get(m.id) ?? 0) + expectedInterval
		let prev = -1
		for (let i = 0; i < rows.length; i++) {
			const stats = rows[i].stats?.[m.id]
			if (!stats) continue
			const prevStats = prev >= 0 ? rows[prev].stats[m.id] : null
			if (prevStats && i - prev > 1 && rows[i].created - rows[prev].created <= fillGap) {
				const start = rows[prev].created
				const span = rows[i].created - start
				for (let k = prev + 1; k < i; k++) {
					const ratio = (rows[k].created - start) / span
					const lerp = (a: number | null, b: number | null) =>
						a == null || b == null ? null : toFixedFloat(a + (b - a) * ratio, 2)
					const sample: MonitorStats = {
						res_avg: lerp(prevStats.res_avg, stats.res_avg),
						res_min: lerp(prevStats.res_min, stats.res_min),
						res_max: lerp(prevStats.res_max, stats.res_max),
						loss: lerp(prevStats.loss, stats.loss) ?? 0,
					}
					filled[k] = { ...filled[k], stats: { ...filled[k].stats, [m.id]: sample } }
				}
			}
			prev = i
		}
	}

	const result: NetworkMonitorStatsRecord[] = []
	for (let i = 0; i < filled.length; i++) {
		if (i > 0) {
			// Break when the time between records is too long for every monitor in both of them.
			const gap = filled[i].created - filled[i - 1].created
			const shared = monitors.filter((m) => filled[i].stats[m.id] && filled[i - 1].stats[m.id])
			if (shared.length && shared.every((m) => gap > (maxGaps.get(m.id) ?? 0))) {
				result.push(monitorGapRecord)
			}
		}
		result.push(filled[i])
	}
	return result
}

/**
 * Fold new records into recent cached records with the same timestamp. Bucketed multi-monitor
 * stats arrive per system, so one bucket can be filled by several batches.
 * Updates matching cached records in place; returns the records that still need appending.
 */
export function mergeSameTimestamps(existing: NetworkMonitorStatsRecord[], newStats: NetworkMonitorStatsRecord[]) {
	if (!existing.length) return newStats
	const recent = existing.slice(-10)
	return newStats.filter((record) => {
		const match = recent.find((r) => r.created !== null && r.created === record.created)
		if (!match) return true
		match.stats = { ...match.stats, ...record.stats }
		return false
	})
}

/**
 * Merge an array of per-monitor raw records into the map-keyed format expected by chart components.
 * `bucketMs` floors timestamps so records from different systems (which are not aligned) share a row.
 */
export function mergeMonitorStats(rawRecords: RawMonitorStatsRecord[], bucketMs = 0): NetworkMonitorStatsRecord[] {
	const byTimestamp = new Map<number, Record<string, MonitorStats>>()
	for (const rec of rawRecords) {
		const created = bucketMs > 0 ? Math.floor(rec.created / bucketMs) * bucketMs : rec.created
		let statsMap = byTimestamp.get(created)
		if (!statsMap) {
			statsMap = {}
			byTimestamp.set(created, statsMap)
		}
		statsMap[rec.monitor] = getMonitorStats(rec)
	}
	return Array.from(byTimestamp.entries())
		.sort(([a], [b]) => a - b)
		.map(([created, stats]) => ({ created, stats }))
}

export {
	getMonitorLabels,
	getMonitorProtocolLabel,
	getMonitorStatus,
	getMonitorTarget,
	isHttpsTarget,
	type MonitorProtocol,
	type MonitorStatus,
	PROTOCOL_LABELS,
	PROTOCOLS,
} from "./monitor-compare"

/** Whole days until the certificate expires; negative once expired. */
export function getCertDaysLeft(cert: Pick<MonitorCertInfo, "expires">, now = Date.now()) {
	return Math.floor((cert.expires - now) / 86_400_000)
}

/** Expiry severity used for certificate colors. */
export function getCertExpiryLevel(daysLeft: number): "ok" | "warning" | "critical" {
	if (daysLeft < 7) return "critical"
	if (daysLeft < 14) return "warning"
	return "ok"
}

/** Read one bulk-monitor row, including CSV-quoted targets containing commas. */
export function parseMonitorFields(line: string): string[] {
	const fields: string[] = []
	let position = 0
	while (position <= line.length) {
		let value = ""
		// allow whitespace before an opening quote, e.g. an indented line or `a, "b,c"`
		let start = position
		while (start < line.length && /\s/.test(line[start])) start++
		if (line[start] === '"') {
			position = start + 1
			let closed = false
			while (position < line.length) {
				const character = line[position++]
				if (character !== '"') {
					value += character
				} else if (line[position] === '"') {
					value += '"'
					position++
				} else {
					closed = true
					break
				}
			}
			if (!closed) throw new Error("Unclosed quoted field")
			while (position < line.length && /\s/.test(line[position])) position++
			if (position < line.length && line[position] !== ",") throw new Error("Expected comma after quoted field")
		} else {
			const end = line.indexOf(",", position)
			const next = end === -1 ? line.length : end
			value = line.slice(position, next)
			position = next
		}
		fields.push(value)
		if (position === line.length) break
		position++
	}
	return fields
}

export function formatMonitorFields(fields: string[]): string {
	return fields.map((field) => (/[",]/.test(field) ? `"${field.replaceAll('"', '""')}"` : field)).join(",")
}
