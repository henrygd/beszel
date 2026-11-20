import { expect, test } from "bun:test"
import { MeterState } from "@/lib/enums"
import { connectedWiFi, primaryWiFi, primaryWiFiSignal, wifiColor, wifiSignalState } from "./wifi"
import type { SystemInfo } from "@/types"

const system = (wf?: SystemInfo["wf"], status: "up" | "down" = "up") => ({ status, info: { wf } as SystemInfo })

test("current state gates panel, not retained history", () => {
	expect(connectedWiFi(system())).toEqual([])
	const nullWifi = system()
	// Exercise malformed stored data without widening the normal wire type.
	Reflect.set(nullWifi.info, "wf", null)
	expect(connectedWiFi(nullWifi)).toEqual([])
	expect(connectedWiFi(system({}))).toEqual([])
	expect(connectedWiFi(system({ wlan0: { r: -50 } }, "down"))).toEqual([])
	expect(connectedWiFi(system({ wlan0: { r: -50 } }))).toHaveLength(1)
	expect(connectedWiFi(system({}))).toHaveLength(0)
	expect(connectedWiFi(system({ wlan0: { s: "new", r: -60 } }))[0][0]).toBe("wlan0")
})

test("multiple interfaces retain independent stable identities and colors", () => {
	const connections = connectedWiFi(system({ wlan1: { s: "same" }, wlan0: { s: "same", r: -40 } }))
	expect(connections.map(([id]) => id)).toEqual(["wlan0", "wlan1"])
	expect(wifiColor(connections[0][0])).toBe(wifiColor("wlan0"))
	expect(wifiColor("wlan0")).not.toBe(wifiColor("wlan1"))
})

test("primaryWiFiSignal returns the strongest current native RSSI", () => {
	expect(primaryWiFiSignal(system({ wlan0: { r: -63 }, wlan1: { r: -48 }, wlan2: {} }))).toBe(-48)
	expect(primaryWiFiSignal(system({ wlan0: {} }))).toBeUndefined()
	expect(primaryWiFiSignal(system({ wlan0: { r: -48 } }, "down"))).toBeUndefined()
})

test("primaryWiFi prefers stations over stronger access points", () => {
	const repeater = connectedWiFi(system({ ap0: { r: -35, a: true }, wlan0: { r: -80 }, wlan1: {} }))
	expect(primaryWiFi(repeater)?.[0]).toBe("wlan0")
	expect(primaryWiFi(connectedWiFi(system({ wlan0: {}, ap0: { r: -35, a: true } })))?.[0]).toBe("wlan0")
	const router = connectedWiFi(system({ ap0: { a: true }, ap1: { r: -60, a: true }, ap2: { r: -45, a: true } }))
	expect(primaryWiFi(router)?.[0]).toBe("ap2")
	expect(primaryWiFi([])).toBeUndefined()
})

test("wifiSignalState thresholds", () => {
	expect(wifiSignalState(-40)).toBe(MeterState.Good)
	expect(wifiSignalState(-65)).toBe(MeterState.Good)
	expect(wifiSignalState(-66)).toBe(MeterState.Warn)
	expect(wifiSignalState(-75)).toBe(MeterState.Warn)
	expect(wifiSignalState(-76)).toBe(MeterState.Crit)
})
