import type { SpeedtestRecord, SpeedtestStatsRecord } from "@/types"
import { getSpeedtestServerLabel } from "./speedtest-utils"

// Kept free of UI and store imports so it can be unit tested with bun.

/** Most lines a comparison charts, to keep it readable and the stats request's filter short. */
export const MAX_COMPARE_SPEEDTESTS = 24

/**
 * Works out what the speedtest sheet can compare and what it charts: other pinned servers on the
 * opened speedtest's system, and other systems testing the same servers. Servers match by ID, so an
 * automatic speedtest (ID 0) only compares with other systems' automatic ones.
 *
 * Every option charts a line: systems are only offered if they test every charted server, and
 * servers only if every selected system tests them. Selections that are no longer options, or
 * that go past MAX_COMPARE_SPEEDTESTS lines, are ignored.
 */
export function getSpeedtestCompareState({
	speedtest,
	speedtests,
	selectedSystemIds,
	selectedServerIds,
	getSystemName,
}: {
	/** The speedtest whose sheet is open; always charted. */
	speedtest: SpeedtestRecord
	/** Speedtests on the opened speedtest's system and the systems to compare against. */
	speedtests: SpeedtestRecord[]
	selectedSystemIds: Set<string>
	selectedServerIds: Set<string>
	getSystemName: (systemId: string) => string
}) {
	const tested = new Set(speedtests.map((s) => `${s.system}:${s.server_id}`))
	const testsAll = (system: string, servers: SpeedtestRecord[]) =>
		servers.every((s) => tested.has(`${system}:${s.server_id}`))

	const localServers = speedtest.server_id
		? speedtests.filter((s) => s.system === speedtest.system && s.id !== speedtest.id && s.server_id)
		: []
	const servers = [
		speedtest,
		...localServers.filter((s) => selectedServerIds.has(s.id)).slice(0, MAX_COMPARE_SPEEDTESTS - 1),
	]

	const otherSystems = new Set(speedtests.map((s) => s.system).filter((id) => id !== speedtest.system))
	const systemOptions = [...otherSystems].filter((id) => testsAll(id, servers))
	const systems = systemOptions
		.filter((id) => selectedSystemIds.has(id))
		.slice(0, Math.floor(MAX_COMPARE_SPEEDTESTS / servers.length) - 1)
	const serverOptions = localServers.filter((s) => systems.every((id) => testsAll(id, [s])))

	const serverIds = new Set(servers.map((s) => s.server_id))
	const compareSpeedtests = [
		...servers,
		...speedtests.filter((s) => systems.includes(s.system) && serverIds.has(s.server_id)),
	]

	// Label series by whatever differs between them: system, server, or both.
	const label = (s: SpeedtestRecord) => {
		const server = getSpeedtestServerLabel(s)
		if (servers.length === 1) return getSystemName(s.system)
		return systems.length ? `${getSystemName(s.system)} · ${server}` : server
	}

	return {
		systemOptions,
		selectedSystemIds: new Set(systems),
		serverOptions,
		selectedServerIds: new Set(servers.slice(1).map((s) => s.id)),
		/** Whether one more server or system still fits within MAX_COMPARE_SPEEDTESTS lines. */
		canAddServer: (servers.length + 1) * (systems.length + 1) <= MAX_COMPARE_SPEEDTESTS,
		canAddSystem: servers.length * (systems.length + 2) <= MAX_COMPARE_SPEEDTESTS,
		/** Speedtests to chart, starting with the opened one. */
		compareSpeedtests,
		getLabel: label,
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
