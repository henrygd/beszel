import { expect, test } from "bun:test"
import { connectedWiFi, strongestWiFiSignal, wifiColor } from "./wifi"
import type { SystemInfo } from "@/types"

const system = (wifi?: SystemInfo["wifi"], status: "up" | "down" = "up") => ({ status, info: { wifi } as SystemInfo })

test("current state gates panel, not retained history", () => {
	expect(connectedWiFi(system())).toEqual([])
	expect(connectedWiFi(system(null))).toEqual([])
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

test("strongestWiFiSignal returns the strongest current native RSSI", () => {
	expect(strongestWiFiSignal(system({ wlan0: { r: -63 }, wlan1: { r: -48 }, wlan2: {} }))).toBe(-48)
	expect(strongestWiFiSignal(system({ wlan0: {} }))).toBeUndefined()
	expect(strongestWiFiSignal(system({ wlan0: { r: -48 } }, "down"))).toBeUndefined()
})
