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
		speedtests,
		selectedSystemIds: new Set(selectedSystemIds),
		selectedServerIds: new Set(selectedServerIds),
		getSystemName: (id) => systemNames[id] ?? id,
	})
}

const ids = (speedtests: SpeedtestRecord[]) => speedtests.map((s) => s.id)

// a tests servers 1 and 2; b tests 1 and 2; c tests only 1; every system also has an automatic test
const a0 = st("a0", "a", 0)
const a1 = st("a1", "a", 1)
const a2 = st("a2", "a", 2)
const b0 = st("b0", "b", 0)
const b1 = st("b1", "b", 1)
const b2 = st("b2", "b", 2)
const c0 = st("c0", "c", 0)
const c1 = st("c1", "c", 1)
const all = [a0, a1, a2, b0, b1, b2, c0, c1]

test("offers other pinned servers on the system and systems testing the same server", () => {
	const s = state(a1, all)
	expect(ids(s.serverOptions)).toEqual(["a2"])
	expect(s.systemOptions).toEqual(["b", "c"])
	expect(ids(s.compareSpeedtests)).toEqual(["a1"])
})

test("matches speedtests across systems by server", () => {
	expect(state(a2, all).systemOptions).toEqual(["b"])
})

test("automatic speedtests only compare with other systems' automatic ones", () => {
	const s = state(a0, all, ["b"], ["a1"])
	expect(ids(s.serverOptions)).toEqual([])
	expect(s.systemOptions).toEqual(["b", "c"])
	expect(ids(s.compareSpeedtests)).toEqual(["a0", "b0"])
	expect(s.getLabel(b0)).toBe("Bravo")
	// A system with only pinned tests has nothing to compare with an automatic one.
	expect(state(a0, [a0, b1]).systemOptions).toEqual([])
})

test("pinned speedtests are never compared with automatic ones", () => {
	const s = state(a1, all, ["b"], ["a0"])
	expect(ids(s.serverOptions)).toEqual(["a2"])
	expect(ids(s.compareSpeedtests)).toEqual(["a1", "b1"])
	// A system with only an automatic test doesn't count as testing any server.
	expect(state(a1, [a1, b0]).systemOptions).toEqual([])
})

test("selected servers limit the systems to ones testing all of them", () => {
	const s = state(a1, all, [], ["a2"])
	expect(s.systemOptions).toEqual(["b"])
	expect(ids(s.compareSpeedtests)).toEqual(["a1", "a2"])
	expect(s.getLabel(a2)).toBe("Server 2")
})

test("selected systems limit the servers to ones they all test", () => {
	const s = state(a1, all, ["c"])
	expect(ids(s.serverOptions)).toEqual([])
	expect(ids(s.compareSpeedtests)).toEqual(["a1", "c1"])
	expect(s.getLabel(c1)).toBe("Charlie")
})

test("charts every selected server for every selected system", () => {
	const s = state(a1, all, ["b"], ["a2"])
	expect(ids(s.compareSpeedtests)).toEqual(["a1", "a2", "b1", "b2"])
	expect(s.getLabel(b2)).toBe("Bravo · Server 2")
})

test("ignores selections that are no longer options", () => {
	const s = state(a1, all, ["c", "missing"], ["a2"])
	expect([...s.selectedSystemIds]).toEqual([])
	expect(ids(s.compareSpeedtests)).toEqual(["a1", "a2"])
})

test("caps the number of charted speedtests", () => {
	const servers = Array.from({ length: MAX_COMPARE_SPEEDTESTS + 5 }, (_, i) => st(`a${i + 1}`, "a", i + 1))
	const opened = st("opened", "a", 1000)
	const s = state(opened, [opened, ...servers], [], ids(servers))
	expect(s.compareSpeedtests).toHaveLength(MAX_COMPARE_SPEEDTESTS)
	expect(s.canAddServer).toBe(false)
})

test("merges runs by time and keeps failed runs apart", () => {
	const run = (speedtest: string, created: number, error = "") =>
		({ speedtest, created, error, download: created }) as SpeedtestStatsRecord
	const merged = mergeSpeedtestCompareStats([run("a", 2), run("b", 1), run("b", 2, "timeout"), run("a", 3, "failed")])
	expect(merged.map((r) => r.created)).toEqual([1, 2, 3])
	expect(Object.keys(merged[1].stats)).toEqual(["a"])
	expect(merged[1].failures).toEqual({ b: "timeout" })
	expect(merged[2].stats).toEqual({})
	expect(merged[2].failures).toEqual({ a: "failed" })
	expect(merged[0].failures).toBeUndefined()
})

test("starts a new segment after each failed run", () => {
	const run = (speedtest: string, created: number, error = "") =>
		({ speedtest, created, error, download: created }) as SpeedtestStatsRecord
	const merged = mergeSpeedtestCompareStats([
		run("a", 1),
		run("b", 2),
		run("a", 3, "failed"),
		run("a", 4),
		run("b", 5),
		run("a", 6, "failed"),
		run("a", 7, "failed"),
		run("a", 8),
	])
	const segmentsOf = (id: string) => merged.filter((r) => r.stats[id]).map((r) => r.segments[id])
	expect(segmentsOf("a")).toEqual([0, 1, 3])
	expect(segmentsOf("b")).toEqual([0, 0])
})
