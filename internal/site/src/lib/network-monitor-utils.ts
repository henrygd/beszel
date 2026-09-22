import type { MonitorStats, NetworkMonitorRecord, RawMonitorStatsRecord } from "@/types"

/** Derive chart metrics from the counts and response sum stored at every retention tier. */
export function getMonitorStats(record: RawMonitorStatsRecord): MonitorStats {
	return {
		res_avg: record.success_count > 0 ? record.res_sum / record.success_count : 0,
		res_min: record.res_min,
		res_max: record.res_max,
		loss: record.total_count > 0 ? ((record.total_count - record.success_count) / record.total_count) * 100 : 0,
	}
}

export function getMonitorTarget(monitor: Pick<NetworkMonitorRecord, "target" | "protocol" | "port" | "server">) {
	if (monitor.protocol === "tcp") {
		const host =
			monitor.target.includes(":") && !monitor.target.startsWith("[") ? `[${monitor.target}]` : monitor.target
		return `${host}:${monitor.port}`
	}
	if (monitor.protocol === "dns" && monitor.server) {
		return `${monitor.target} via ${monitor.server}`
	}
	return monitor.target
}
