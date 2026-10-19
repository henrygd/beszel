import type { SystemStatsRecord } from "@/types"

/** Include historical channels even if the newest sample is unavailable. */
export function fanControlChannels(records: Pick<SystemStatsRecord, "stats">[]): string[] {
	const channels = new Set<string>()
	for (const { stats } of records) {
		for (const [key, level] of Object.entries(stats?.fc ?? {})) {
			if (typeof level === "number" && Number.isFinite(level) && level >= 0 && level <= 100) channels.add(key)
		}
	}
	return [...channels].sort()
}

export function fanControlLevel(record: Pick<SystemStatsRecord, "stats">, channel: string): number | undefined {
	const level = record.stats?.fc?.[channel]
	return typeof level === "number" && Number.isFinite(level) && level >= 0 && level <= 100 ? level : undefined
}

export function fanControlColor(channel: string): string {
	let hash = 0
	for (const char of channel) hash = (Math.imul(hash, 31) + char.charCodeAt(0)) | 0
	return `hsl(${(hash >>> 0) % 360}, 60%, 55%)`
}
