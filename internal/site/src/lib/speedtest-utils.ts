import type { SpeedtestRecord } from "@/types"

/** Speedtest interval limits in minutes, matching the speedtests collection. */
export const MIN_SPEEDTEST_INTERVAL = 15
export const MAX_SPEEDTEST_INTERVAL = 10080

export const DEFAULT_SPEEDTEST_INTERVAL = 360

/** Format an interval in minutes as a short duration, e.g. "30m" or "6h". */
export function formatSpeedtestInterval(minutes: number) {
	if (minutes % 1440 === 0) return `${minutes / 1440}d`
	if (minutes % 60 === 0) return `${minutes / 60}h`
	return `${minutes}m`
}

/**
 * Label for the server a speedtest uses: its name and location. For automatic
 * speedtests this is the server of the latest run. Falls back to the ID only for
 * a pinned server whose name is unknown, e.g. one created through the API.
 * A non-default interface is appended so tests of one server can be told apart.
 */
export function getSpeedtestServerLabel(
	speedtest: Pick<SpeedtestRecord, "server_id" | "server_name" | "server_location"> &
		Partial<Pick<SpeedtestRecord, "interface">>
) {
	let name = [speedtest.server_name, speedtest.server_location].filter(Boolean).join(" — ")
	if (!name && speedtest.server_id) name = `#${speedtest.server_id}`
	return name && speedtest.interface ? `${name} (${speedtest.interface})` : name
}
