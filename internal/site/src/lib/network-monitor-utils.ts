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

export { getMonitorTarget } from "./monitor-compare"

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
