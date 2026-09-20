import type { SystemRecord, WiFi } from "@/types"

// Current system info is independent of the selected historical chart window.
// No fallback to history: missing data, disconnect and offline all hide the panel.
export function connectedWiFi(system: Pick<SystemRecord, "status" | "info">): [string, WiFi][] {
	return system.status === "up" ? Object.entries(system.info?.wifi ?? {}).sort(([a], [b]) => a.localeCompare(b)) : []
}

export function strongestWiFiSignal(system: Pick<SystemRecord, "status" | "info">): number | undefined {
	const signals = connectedWiFi(system)
		.map(([, wifi]) => wifi.r)
		.filter((signal): signal is number => signal !== undefined && Number.isFinite(signal))
	return signals.length ? Math.max(...signals) : undefined
}

export function wifiColor(id: string): string {
	let hash = 0
	for (const char of id) hash = (Math.imul(hash, 31) + char.charCodeAt(0)) | 0
	return `hsl(${(hash >>> 0) % 360}, 65%, 52%)`
}
