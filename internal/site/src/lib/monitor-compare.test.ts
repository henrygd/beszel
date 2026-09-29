import { expect, test } from "bun:test"
import type { NetworkMonitorRecord } from "@/types"
import { getMonitorCompareState, getMonitorIdentityKey, getMonitorTarget } from "./monitor-compare"

function mon(id: string, system: string, target: string, extra: Partial<NetworkMonitorRecord> = {}) {
	return { id, system, target, protocol: "icmp", port: 0, server: "", interval: 30, ...extra } as NetworkMonitorRecord
}

const systemNames: Record<string, string> = { a: "Alpha", b: "Bravo", c: "Charlie" }

function state(
	monitor: NetworkMonitorRecord,
	monitors: NetworkMonitorRecord[],
	selectedSystemIds: string[] = [],
	selectedTargetIds: string[] = []
) {
	return getMonitorCompareState({
		monitor,
		localMonitors: monitors,
		otherMonitors: monitors,
		selectedSystemIds: new Set(selectedSystemIds),
		selectedTargetIds: new Set(selectedTargetIds),
		getSystemName: (id) => systemNames[id] ?? id,
	})
}

const ids = (monitors: NetworkMonitorRecord[]) => monitors.map((m) => m.id)

// a probes one and two; b probes one and two; c probes only one
const a1 = mon("a1", "a", "one.example")
const a2 = mon("a2", "a", "two.example")
const b1 = mon("b1", "b", "one.example")
const b2 = mon("b2", "b", "two.example")
const c1 = mon("c1", "c", "one.example")
const all = [a1, a2, b1, b2, c1]

test("formats tcp targets with their port", () => {
	expect(getMonitorTarget({ target: "example.com", protocol: "icmp", port: 0 })).toBe("example.com")
	expect(getMonitorTarget({ target: "example.com", protocol: "tcp", port: 443 })).toBe("example.com:443")
	expect(getMonitorTarget({ target: "::1", protocol: "tcp", port: 22 })).toBe("[::1]:22")
})

test("identity ignores the system but not protocol, port or server", () => {
	expect(getMonitorIdentityKey(a1)).toBe(getMonitorIdentityKey(b1))
	expect(getMonitorIdentityKey(a1)).not.toBe(getMonitorIdentityKey({ ...a1, protocol: "http" }))
	expect(getMonitorIdentityKey(a1)).not.toBe(getMonitorIdentityKey({ ...a1, port: 80 }))
	expect(getMonitorIdentityKey(a1)).not.toBe(getMonitorIdentityKey({ ...a1, server: "1.1.1.1" }))
})

test("with nothing selected only the opened monitor is charted", () => {
	const s = state(a1, all)
	expect(s.systemOptions).toEqual(["b", "c"])
	expect(ids(s.targetOptions)).toEqual(["a2"])
	expect(ids(s.compareMonitors)).toEqual(["a1"])
})

test("only offers targets and systems with the same protocol", () => {
	const aHttp = mon("aHttp", "a", "https://two.example", { protocol: "http" })
	const bTcp = mon("bTcp", "b", "one.example", { protocol: "tcp", port: 443 })
	const s = state(a1, [a1, a2, aHttp, bTcp, c1])
	expect(ids(s.targetOptions)).toEqual(["a2"])
	expect(s.systemOptions).toEqual(["c"])
})

test("comparing targets charts them on the opened system, labelled by target", () => {
	const s = state(a1, all, [], ["a2"])
	expect(ids(s.compareMonitors)).toEqual(["a1", "a2"])
	expect(s.getLabel(a1)).toBe("one.example")
	expect(s.getLabel(a2)).toBe("two.example")
})

test("comparing systems charts the opened target on them, labelled by system", () => {
	const s = state(a1, all, ["b", "c"])
	expect(ids(s.compareMonitors)).toEqual(["a1", "b1", "c1"])
	expect(s.getLabel(a1)).toBe("Alpha")
	expect(s.getLabel(c1)).toBe("Charlie")
})

test("each compared target is charted on every selected system", () => {
	const s = state(a1, all, ["b"], ["a2"])
	expect(ids(s.compareMonitors)).toEqual(["a1", "a2", "b1", "b2"])
	expect(s.getLabel(b2)).toBe("Bravo · two.example")
})

test("selecting a system limits targets to ones it also probes", () => {
	const s = state(a1, all, ["c"])
	expect(ids(s.targetOptions)).toEqual([])
})

test("selecting a target limits systems to ones that also probe it", () => {
	const s = state(a1, all, [], ["a2"])
	expect(s.systemOptions).toEqual(["b"])
})

test("the pickers can't produce conflicting selections, but targets win if they conflict", () => {
	const s = state(a1, all, ["c"], ["a2"])
	expect(s.selectedSystemIds.size).toBe(0)
	expect([...s.selectedTargetIds]).toEqual(["a2"])
	expect(ids(s.compareMonitors)).toEqual(["a1", "a2"])
})

test("selected systems that are no longer options are ignored", () => {
	const s = state(a1, all, ["gone"])
	expect(s.selectedSystemIds.size).toBe(0)
	expect(ids(s.compareMonitors)).toEqual(["a1"])
})

test("DNS lookups of the same name against different servers get the server in their label", () => {
	const d1 = mon("d1", "a", "example.com", { protocol: "dns", server: "1.1.1.1" })
	const d2 = mon("d2", "a", "example.com", { protocol: "dns", server: "8.8.8.8" })
	const s = state(d1, [d1, d2], [], ["d2"])
	expect(s.getLabel(d1)).toBe("example.com (1.1.1.1)")
	expect(s.getLabel(d2)).toBe("example.com (8.8.8.8)")
})

test("uses separate local and fetched monitor lists on single-system pages", () => {
	const s = getMonitorCompareState({
		monitor: a1,
		localMonitors: [a1, a2],
		otherMonitors: [b1, b2],
		selectedSystemIds: new Set(["b"]),
		selectedTargetIds: new Set(["a2"]),
		getSystemName: (id) => systemNames[id],
	})
	expect(ids(s.compareMonitors)).toEqual(["a1", "a2", "b1", "b2"])
})
