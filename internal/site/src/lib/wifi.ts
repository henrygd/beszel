import { MeterState } from "@/lib/enums"
import type { SystemRecord, WiFi } from "@/types"

// Current system info is independent of the selected historical chart window.
// No fallback to history: missing data, disconnect and offline all hide the panel.
export function connectedWiFi(system: Pick<SystemRecord, "status" | "info">): [string, WiFi][] {
	return system.status === "up" ? Object.entries(system.info?.wf ?? {}).sort(([a], [b]) => a.localeCompare(b)) : []
}

/** Strongest connection by RSSI, falling back to the first when none report a signal. */
export function strongestWiFi(connections: [string, WiFi][]): [string, WiFi] | undefined {
	let strongest = connections[0]
	for (const connection of connections) {
		if ((connection[1].r ?? -Infinity) > (strongest[1].r ?? -Infinity)) {
			strongest = connection
		}
	}
	return strongest
}

export function strongestWiFiSignal(system: Pick<SystemRecord, "status" | "info">): number | undefined {
	return strongestWiFi(connectedWiFi(system))?.[1].r
}

/** Signal quality for an RSSI reading: good at -65 dBm or stronger, warn down to -75 dBm, crit below. */
export function wifiSignalState(rssi: number): MeterState {
	return rssi >= -65 ? MeterState.Good : rssi >= -75 ? MeterState.Warn : MeterState.Crit
}

export function wifiColor(id: string): string {
	let hash = 0
	for (const char of id) hash = (Math.imul(hash, 31) + char.charCodeAt(0)) | 0
	return `hsl(${(hash >>> 0) % 360}, 65%, 52%)`
}
