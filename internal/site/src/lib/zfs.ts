import type { SystemStatsRecord } from "@/types"

/**
 * Agents only report ZFS ARC memory (`mz`) on hosts that expose the ZFS kstats,
 * so the field is absent on every other host. Charts must gate the series on
 * data actually present in the window instead of registering it unconditionally.
 */
export function hasZfsArcStats(systemStats: SystemStatsRecord[]): boolean {
	return systemStats.some((record) => typeof record.stats?.mz === "number")
}
