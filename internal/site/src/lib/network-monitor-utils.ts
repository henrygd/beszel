import type { NetworkMonitorRecord } from "@/types"

export function getMonitorTarget(monitor: Pick<NetworkMonitorRecord, "target" | "protocol" | "port">) {
	if (monitor.protocol !== "tcp") return monitor.target
	const host = monitor.target.includes(":") && !monitor.target.startsWith("[") ? `[${monitor.target}]` : monitor.target
	return `${host}:${monitor.port}`
}
