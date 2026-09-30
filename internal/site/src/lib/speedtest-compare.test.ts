import { expect, test } from "bun:test"
import type { SpeedtestRecord, SpeedtestStatsRecord } from "@/types"
import { getSpeedtestCompareState, MAX_COMPARE_SPEEDTESTS, mergeSpeedtestCompareStats } from "./speedtest-compare"

function st(id: string, system: string, server_id: number) {
	return {
		id,
		system,
		server_id,
		server_name: server_id ? `Server ${server_id}` : "",
		server_location: "",
	} as SpeedtestRecord
}

const systemNames: Record<string, string> = { a: "Alpha", b: "Bravo", c: "Charlie" }

function state(
	speedtest: SpeedtestRecord,
	speedtests: SpeedtestRecord[],
	selectedSystemIds: string[] = [],
	selectedServerIds: string[] = []
) {
	return getSpeedtestCompareState({
		speedtest,
		localSpeedtests: speedtests,
		otherSpeedtests: speedtests,
		selectedSystemIds: new Set(selectedSystemIds),
		selectedServerIds: new Set(selectedServerIds),
		getSystemName: (id) => systemNames[id] ?? id,
		getServerLabel: (s) => s.server_name || "Automatic",
	})
}

const ids = (speedtests: SpeedtestRecord[]) => speedtests.map((s) => s.id)

// a tests automatic and 1; b tests automatic and 1; c tests only automatic
const a0 = st("a0", "a", 0)
const a1 = st("a1", "a", 1)
const b0 = st("b0", "b", 0)
const b1 = st("b1", "b", 1)
const c0 = st("c0", "c", 0)
const all = [a0, a1, b0, b1, c0]

test("offers other servers on the system and systems testing the same server", () => {
	const s = state(a0, all)
	expect(ids(s.serverOptions)).toEqual(["a1"])
	expect(s.systemOptions).toEqual(["b", "c"])
	expect(ids(s.compareSpeedtests)).toEqual(["a0"])
})

test("matches speedtests across systems by server", () => {
	expect(state(a1, all).systemOptions).toEqual(["b"])
})

test("selected servers limit the systems to ones testing all of them", () => {
	const s = state(a0, all, [], ["a1"])
	expect(s.systemOptions).toEqual(["b"])
	expect(ids(s.compareSpeedtests)).toEqual(["a0", "a1"])
	expect(s.getLabel(a1)).toBe("Server 1")
})

test("selected systems limit the servers to ones they all test", () => {
	const s = state(a0, all, ["c"])
	expect(ids(s.serverOptions)).toEqual([])
	expect(ids(s.compareSpeedtests)).toEqual(["a0", "c0"])
	expect(s.getLabel(c0)).toBe("Charlie")
})

test("charts every selected server for every selected system", () => {
	const s = state(a0, all, ["b"], ["a1"])
	expect(ids(s.compareSpeedtests)).toEqual(["a0", "a1", "b0", "b1"])
	expect(s.getLabel(b1)).toBe("Bravo · Server 1")
})

test("ignores selections that are no longer options", () => {
	const s = state(a0, all, ["c", "missing"], ["a1"])
	expect([...s.selectedSystemIds]).toEqual([])
	expect(ids(s.compareSpeedtests)).toEqual(["a0", "a1"])
})

test("caps the number of charted speedtests", () => {
	const servers = Array.from({ length: MAX_COMPARE_SPEEDTESTS + 5 }, (_, i) => st(`a${i + 1}`, "a", i + 1))
	const s = state(a0, [a0, ...servers], [], ids(servers))
	expect(s.compareSpeedtests).toHaveLength(MAX_COMPARE_SPEEDTESTS)
	expect(s.canAddServer).toBe(false)
})

test("merges runs by time and leaves out failed runs", () => {
	const run = (speedtest: string, created: number, error = "") =>
		({ speedtest, created, error, download: created }) as SpeedtestStatsRecord
	const merged = mergeSpeedtestCompareStats([run("a", 2), run("b", 1), run("b", 2), run("a", 3, "failed")])
	expect(merged.map((r) => r.created)).toEqual([1, 2])
	expect(Object.keys(merged[1].stats).sort()).toEqual(["a", "b"])
})
