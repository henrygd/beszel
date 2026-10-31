import type { NetworkMonitorRecord } from "@/types"

// Kept free of UI and store imports so it can be unit tested with bun.

/** Most lines a comparison charts, to keep it readable and the stats request's filter short. */
export const MAX_COMPARE_MONITORS = 24

export type MonitorProtocol = NetworkMonitorRecord["protocol"]

/** Protocols in display order. */
export const PROTOCOLS: MonitorProtocol[] = ["icmp", "tcp", "http", "dns"]
export const PROTOCOL_LABELS: Record<MonitorProtocol, string> = { icmp: "ICMP", tcp: "TCP", http: "HTTP(S)", dns: "DNS" }

type MonitorTarget =Pick<NetworkMonitorRecord, "target" | "protocol" | "port">

export function getMonitorTarget(monitor: MonitorTarget) {
	// The protocol column already says http, so the URL scheme is redundant.
	if (monitor.protocol === "http") return monitor.target.replace(/^https?:\/\//i, "")
	if (monitor.protocol !== "tcp") return monitor.target
	const host = monitor.target.includes(":") && !monitor.target.startsWith("[") ? `[${monitor.target}]` : monitor.target
	return `${host}:${monitor.port}`
}

export function isHttpsTarget(target: string) {
	return /^https:\/\//i.test(target)
}

/** The protocol to show next to a target, since getMonitorTarget hides whether an HTTP monitor uses https. */
export function getMonitorProtocolLabel(monitor: Pick<NetworkMonitorRecord, "target" | "protocol">) {
	return monitor.protocol === "http" && isHttpsTarget(monitor.target) ? "https" : monitor.protocol
}

/** Identifies what a monitor probes, regardless of which system probes it. */
export function getMonitorIdentityKey({
	protocol,
	target,
	port,
	server,
}: Pick<NetworkMonitorRecord, "protocol" | "target" | "port" | "server">) {
	return JSON.stringify([protocol, target, port, server])
}

type LabeledMonitor = Pick<NetworkMonitorRecord, "id" | "target" | "protocol" | "port" | "server">

/**
 * Chart series labels by monitor ID. `getSeriesLabel` gets the display target (see getMonitorTarget)
 * and builds the label; monitors that would share one are told apart by DNS server or URL scheme.
 */
export function getMonitorLabels<T extends LabeledMonitor>(
	monitors: T[],
	getSeriesLabel: (monitor: T, target: string) => string
) {
	const labels = new Map<string, string>()
	const counts = new Map<string, number>()
	for (const m of monitors) {
		const label = getSeriesLabel(m, getMonitorTarget(m))
		labels.set(m.id, label)
		counts.set(label, (counts.get(label) ?? 0) + 1)
	}
	// DNS lookups of the same name against different servers, and http and https URLs that are otherwise
	// the same, would share a label.
	for (const m of monitors) {
		const label = labels.get(m.id) as string
		if ((counts.get(label) ?? 0) < 2) continue
		if (m.server) labels.set(m.id, `${label} (${m.server})`)
		else if (m.protocol === "http") labels.set(m.id, getSeriesLabel(m, m.target))
	}
	return labels
}

export type MonitorStatus = "up" | "down" | "pending" | "paused"

/**
 * Status shown by the monitors table's status dot: paused when the monitor is disabled or its system
 * is paused, pending while the system is down or pending, and down when the last probe got no response.
 */
export function getMonitorStatus(
	monitor: Pick<NetworkMonitorRecord, "enabled" | "updated" | "res">,
	systemStatus?: string
): MonitorStatus {
	if (!monitor.enabled || systemStatus === "paused") return "paused"
	if (systemStatus === "down" || systemStatus === "pending") return "pending"
	if (monitor.updated && !monitor.res) return "down"
	return "up"
}

interface MonitorCompareInput {
	/** The monitor whose sheet is open; always charted. */
	monitor: NetworkMonitorRecord
	/** Monitors that may include other targets on the opened monitor's system. */
	localMonitors: NetworkMonitorRecord[]
	/** Monitors that may include other systems' monitors. */
	otherMonitors: NetworkMonitorRecord[]
	selectedSystemIds: Set<string>
	selectedTargetIds: Set<string>
	getSystemName: (systemId: string) => string
}

/**
 * Works out what the monitor sheet can compare and what it charts. Comparisons are limited to the opened
 * monitor's protocol, since response time and loss mean different things per protocol.
 */
export function getMonitorCompareState({
	monitor,
	localMonitors,
	otherMonitors,
	selectedSystemIds,
	selectedTargetIds,
	getSystemName,
}: MonitorCompareInput) {
	const sameProtocol = (m: NetworkMonitorRecord) => m.protocol === monitor.protocol
	const systemTargets = localMonitors.filter(
		(m) => m.system === monitor.system && m.id !== monitor.id && sameProtocol(m)
	)
	const systemMonitors = otherMonitors.filter((m) => m.system !== monitor.system && sameProtocol(m))

	const keysBySystem = new Map<string, Set<string>>()
	for (const m of systemMonitors) {
		const keys = keysBySystem.get(m.system) ?? new Set<string>()
		keys.add(getMonitorIdentityKey(m))
		keysBySystem.set(m.system, keys)
	}

	// Each picker only offers what fits the other's selection, so every pick charts a line per system:
	// systems must probe the opened target and every selected target, and targets must be probed by
	// every selected system. Selections outside the options (e.g. a monitor deleted while the sheet is
	// open) are ignored, and ones past MAX_COMPARE_MONITORS lines are dropped, keeping targets over
	// systems and earlier picks over later ones.
	const systemTargetsById = new Map(systemTargets.map((m) => [m.id, m]))
	const pickedTargets = [...selectedTargetIds]
		.flatMap((id) => systemTargetsById.get(id) ?? [])
		.slice(0, MAX_COMPARE_MONITORS - 1)
	const targetMonitors = [monitor, ...pickedTargets]
	const requiredKeys = targetMonitors.map(getMonitorIdentityKey)
	const systemOptions = [...keysBySystem]
		.filter(([, keys]) => requiredKeys.every((key) => keys.has(key)))
		.map(([id]) => id)
	const systemOptionSet = new Set(systemOptions)
	const systemIds = [...selectedSystemIds]
		.filter((id) => systemOptionSet.has(id))
		.slice(0, Math.floor(MAX_COMPARE_MONITORS / targetMonitors.length) - 1)

	const targetOptions = systemTargets.filter((m) => {
		const key = getMonitorIdentityKey(m)
		return systemIds.every((id) => keysBySystem.get(id)?.has(key))
	})
	const targetIds = new Set(pickedTargets.map((m) => m.id))

	// Every charted target is also charted for each selected system.
	const targetKeys = new Set(targetMonitors.map(getMonitorIdentityKey))
	const selectedSystems = new Set(systemIds)
	const compareMonitors = [
		...targetMonitors,
		...systemMonitors.filter((m) => selectedSystems.has(m.system) && targetKeys.has(getMonitorIdentityKey(m))),
	]

	// Label series by whatever differs between them: system, target, or both.
	const multiSystem = systemIds.length > 0
	const multiTarget = targetIds.size > 0
	const labels = getMonitorLabels(compareMonitors, (m, target) => {
		const systemName = getSystemName(m.system)
		return multiSystem && multiTarget ? `${systemName} · ${target}` : multiTarget ? target : systemName
	})

	return {
		/** Systems that can be selected to compare against. */
		systemOptions,
		selectedSystemIds: new Set(systemIds),
		/** Other targets on the opened monitor's system that can be selected. */
		targetOptions,
		selectedTargetIds: targetIds,
		/** Whether one more target or system still fits within MAX_COMPARE_MONITORS lines. */
		canAddTarget: (targetMonitors.length + 1) * (systemIds.length + 1) <= MAX_COMPARE_MONITORS,
		canAddSystem: targetMonitors.length * (systemIds.length + 2) <= MAX_COMPARE_MONITORS,
		/** Monitors to chart, starting with the opened one. */
		compareMonitors,
		getLabel: (m: NetworkMonitorRecord) => labels.get(m.id) ?? getMonitorTarget(m),
	}
}
