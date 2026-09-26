import type { MonitorCertInfo, MonitorStats, NetworkMonitorRecord, RawMonitorStatsRecord } from "@/types"
import { toFixedFloat } from "./utils"

/** Derive chart metrics from the counts and response sum stored at every retention tier. */
export function getMonitorStats(record: RawMonitorStatsRecord): MonitorStats {
	return {
		res_avg: record.success_count > 0 ? toFixedFloat(record.res_sum / record.success_count, 2) : 0,
		res_min: record.res_min,
		res_max: record.res_max,
		loss:
			record.total_count > 0
				? toFixedFloat(((record.total_count - record.success_count) / record.total_count) * 100, 2)
				: 0,
	}
}

export function getMonitorTarget(monitor: Pick<NetworkMonitorRecord, "target" | "protocol" | "port">) {
	if (monitor.protocol !== "tcp") return monitor.target
	const host = monitor.target.includes(":") && !monitor.target.startsWith("[") ? `[${monitor.target}]` : monitor.target
	return `${host}:${monitor.port}`
}

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
