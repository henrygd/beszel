import type { NetworkMonitorRecord } from "@/types"

// Kept free of UI and store imports so it can be unit tested with bun.

type MonitorTarget = Pick<NetworkMonitorRecord, "target" | "protocol" | "port">

export function getMonitorTarget(monitor: MonitorTarget) {
	if (monitor.protocol !== "tcp") return monitor.target
	const host = monitor.target.includes(":") && !monitor.target.startsWith("[") ? `[${monitor.target}]` : monitor.target
	return `${host}:${monitor.port}`
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
	// every selected system.
	const requiredKeys = [monitor, ...systemTargets.filter((m) => selectedTargetIds.has(m.id))].map(getMonitorIdentityKey)
	const systemOptions = [...keysBySystem]
		.filter(([, keys]) => requiredKeys.every((key) => keys.has(key)))
		.map(([id]) => id)
	// Selections outside the options (e.g. a monitor deleted while the sheet is open) are ignored.
	const systemIds = systemOptions.filter((id) => selectedSystemIds.has(id))

	const targetOptions = systemTargets.filter((m) => {
		const key = getMonitorIdentityKey(m)
		return systemIds.every((id) => keysBySystem.get(id)?.has(key))
	})
	const targetIds = new Set(targetOptions.filter((m) => selectedTargetIds.has(m.id)).map((m) => m.id))

	// Every charted target is also charted for each selected system.
	const targetMonitors = [monitor, ...targetOptions.filter((m) => targetIds.has(m.id))]
	const targetKeys = new Set(targetMonitors.map(getMonitorIdentityKey))
	const selectedSystems = new Set(systemIds)
	const compareMonitors = [
		...targetMonitors,
		...systemMonitors.filter((m) => selectedSystems.has(m.system) && targetKeys.has(getMonitorIdentityKey(m))),
	]

	// Label series by whatever differs between them: system, target, or both.
	const multiSystem = systemIds.length > 0
	const multiTarget = targetIds.size > 0
	const labels = new Map<string, string>()
	const counts = new Map<string, number>()
	for (const m of compareMonitors) {
		const systemName = getSystemName(m.system)
		const target = getMonitorTarget(m)
		const label = multiSystem && multiTarget ? `${systemName} · ${target}` : multiTarget ? target : systemName
		labels.set(m.id, label)
		counts.set(label, (counts.get(label) ?? 0) + 1)
	}
	// DNS lookups of the same name against different servers would otherwise share a label.
	for (const m of compareMonitors) {
		const label = labels.get(m.id) as string
		if ((counts.get(label) ?? 0) > 1) labels.set(m.id, `${label} (${m.server})`)
	}

	return {
		/** Systems that can be selected to compare against. */
		systemOptions,
		selectedSystemIds: new Set(systemIds),
		/** Other targets on the opened monitor's system that can be selected. */
		targetOptions,
		selectedTargetIds: targetIds,
		/** Monitors to chart, starting with the opened one. */
		compareMonitors,
		getLabel: (m: NetworkMonitorRecord) => labels.get(m.id) ?? getMonitorTarget(m),
	}
}
