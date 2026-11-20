import type { SystemStatsRecord } from "@/types"

export function memoryBandwidthTotal(stats: SystemStatsRecord["stats"], showMax: boolean) {
	const bandwidth = showMax ? (stats?.mbwm ?? stats?.mbw) : stats?.mbw
	if (!bandwidth) return undefined
	// A chart window or averaged record can mix split and total-only sources.
	// Sum both average fields, but never add independent read/write peaks or
	// label a split source's read peak as a total peak.
	if (showMax && (!stats?.mbwt || bandwidth[1] !== 0)) return undefined
	return bandwidth[0] + bandwidth[1]
}
