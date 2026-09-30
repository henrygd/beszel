import type { SpeedtestRecord, SpeedtestStatsRecord } from "@/types"

// Kept free of UI and store imports so it can be unit tested with bun.

/** Most lines a comparison charts, to keep it readable and the stats request's filter short. */
export const MAX_COMPARE_SPEEDTESTS = 24

type CompareSpeedtest = Pick<SpeedtestRecord, "id" | "system" | "server_id" | "server_name" | "server_location">

interface SpeedtestCompareInput<T extends CompareSpeedtest> {
	/** The speedtest whose sheet is open; always charted. */
	speedtest: T
	/** Speedtests that may include other servers on the opened speedtest's system. */
	localSpeedtests: T[]
	/** Speedtests that may include other systems' speedtests. */
	otherSpeedtests: T[]
	selectedSystemIds: Set<string>
	selectedServerIds: Set<string>
	getSystemName: (systemId: string) => string
	getServerLabel: (speedtest: T) => string
}

/**
 * Works out what the speedtest sheet can compare and what it charts: other servers tested by the
 * opened speedtest's system, and other systems testing the same servers. Speedtests are matched
 * across systems by server ID, so an automatic speedtest only compares with other systems'
 * automatic ones. Its server can change between runs, so it isn't compared with pinned servers.
 */
export function getSpeedtestCompareState<T extends CompareSpeedtest>({
	speedtest,
	localSpeedtests,
	otherSpeedtests,
	selectedSystemIds,
	selectedServerIds,
	getSystemName,
	getServerLabel,
}: SpeedtestCompareInput<T>) {
	// Other servers are only offered for pinned speedtests, and only pinned ones. Other systems' speedtests
	// match on server ID below, which keeps automatic and pinned speedtests apart.
	const serverTests = speedtest.server_id
		? localSpeedtests.filter((s) => s.system === speedtest.system && s.id !== speedtest.id && s.server_id !== 0)
		: []
	const systemTests = otherSpeedtests.filter((s) => s.system !== speedtest.system)

	const serversBySystem = new Map<string, Set<number>>()
	for (const s of systemTests) {
		const servers = serversBySystem.get(s.system) ?? new Set<number>()
		servers.add(s.server_id)
		serversBySystem.set(s.system, servers)
	}

	// Each picker only offers what fits the other's selection, so every pick charts a line per system:
	// systems must test the opened server and every selected server, and servers must be tested by
	// every selected system. Selections outside the options (e.g. a speedtest deleted while the sheet
	// is open) are ignored, and ones past MAX_COMPARE_SPEEDTESTS lines are dropped, keeping servers
	// over systems and earlier picks over later ones.
	const serverTestsById = new Map(serverTests.map((s) => [s.id, s]))
	const pickedServers = [...selectedServerIds]
		.flatMap((id) => serverTestsById.get(id) ?? [])
		.slice(0, MAX_COMPARE_SPEEDTESTS - 1)
	const serverSpeedtests = [speedtest, ...pickedServers]
	const requiredServers = serverSpeedtests.map((s) => s.server_id)
	const systemOptions = [...serversBySystem]
		.filter(([, servers]) => requiredServers.every((server) => servers.has(server)))
		.map(([id]) => id)
	const systemOptionSet = new Set(systemOptions)
	const systemIds = [...selectedSystemIds]
		.filter((id) => systemOptionSet.has(id))
		.slice(0, Math.floor(MAX_COMPARE_SPEEDTESTS / serverSpeedtests.length) - 1)

	const serverOptions = serverTests.filter((s) => systemIds.every((id) => serversBySystem.get(id)?.has(s.server_id)))
	const serverIds = new Set(pickedServers.map((s) => s.id))

	// Every charted server is also charted for each selected system.
	const chartedServers = new Set(requiredServers)
	const selectedSystems = new Set(systemIds)
	const compareSpeedtests = [
		...serverSpeedtests,
		...systemTests.filter((s) => selectedSystems.has(s.system) && chartedServers.has(s.server_id)),
	]

	// Label series by whatever differs between them: system, server, or both.
	const multiSystem = systemIds.length > 0
	const multiServer = serverIds.size > 0
	const labels = new Map<string, string>()
	for (const s of compareSpeedtests) {
		const systemName = getSystemName(s.system)
		const server = getServerLabel(s)
		labels.set(s.id, multiSystem && multiServer ? `${systemName} · ${server}` : multiServer ? server : systemName)
	}

	return {
		/** Systems that can be selected to compare against. */
		systemOptions,
		selectedSystemIds: new Set(systemIds),
		/** Other servers tested by the opened speedtest's system that can be selected. */
		serverOptions,
		selectedServerIds: serverIds,
		/** Whether one more server or system still fits within MAX_COMPARE_SPEEDTESTS lines. */
		canAddServer: (serverSpeedtests.length + 1) * (systemIds.length + 1) <= MAX_COMPARE_SPEEDTESTS,
		canAddSystem: serverSpeedtests.length * (systemIds.length + 2) <= MAX_COMPARE_SPEEDTESTS,
		/** Speedtests to chart, starting with the opened one. */
		compareSpeedtests,
		getLabel: (s: T) => labels.get(s.id) ?? getServerLabel(s),
	}
}

/** Runs of several speedtests, keyed by speedtest ID, for charting them side by side. */
export interface SpeedtestCompareRecord {
	created: number
	stats: Record<string, SpeedtestStatsRecord>
}

/**
 * Groups runs by time for comparison charts. Speedtests rarely run at the same moment, so most
 * records hold one run; the charts connect each speedtest's runs across the others'. Failed runs
 * are left out, since they have no measurements.
 */
export function mergeSpeedtestCompareStats(runs: SpeedtestStatsRecord[]): SpeedtestCompareRecord[] {
	const byCreated = new Map<number, SpeedtestCompareRecord>()
	for (const run of runs) {
		if (run.error || run.created === null) continue
		const record = byCreated.get(run.created) ?? { created: run.created, stats: {} }
		record.stats[run.speedtest] = run
		byCreated.set(run.created, record)
	}
	return [...byCreated.values()].sort((a, b) => a.created - b.created)
}
