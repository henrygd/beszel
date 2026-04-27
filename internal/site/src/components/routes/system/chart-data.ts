import { getPbTimestamp, pb } from "@/lib/api"
import { chartTimeData } from "@/lib/utils"
import type {
	ChartData,
	ChartDataContainer,
	ChartTimes,
	ContainerStatsRecord,
	NetworkProbeStatsRecord,
	SystemStatsRecord,
} from "@/types"

type ChartTimeData = {
	time: number
	data: {
		ticks: number[]
		domain: number[]
	}
	chartTime: ChartTimes
}

export const cache = new Map<
	string,
	ChartTimeData | SystemStatsRecord[] | ContainerStatsRecord[] | ChartData["containerData"]
>()

/** Append new records onto prev with gap detection. Converts string `created` values to ms timestamps in place.
 * Pass `maxLen` to cap the result length in one copy instead of slicing again after the call. */
export function appendData<T extends { created: string | number | null }>(
	prev: T[] = [],
	newRecords: T[],
	expectedInterval: number,
	maxLen?: number
): T[] {
	if (!newRecords.length) return prev
	// Pre-trim prev so the single slice() below is the only copy we make
	const trimmed = maxLen && prev.length >= maxLen ? prev.slice(-(maxLen - newRecords.length)) : prev
	const result = trimmed.slice()
	let prevTime = (trimmed.at(-1)?.created as number) ?? 0
	for (const record of newRecords) {
		if (record.created !== null) {
			if (typeof record.created === "string") {
				record.created = new Date(record.created).getTime()
			}
			if (prevTime && (record.created as number) - prevTime > expectedInterval * 1.5) {
				result.push({ created: null, ...("stats" in record ? { stats: null } : {}) } as T)
			}
			prevTime = record.created as number
		}
		result.push(record)
	}
	return result
}

export async function getStats<T extends SystemStatsRecord | ContainerStatsRecord | NetworkProbeStatsRecord>(
	collection: string,
	systemId: string,
	chartTime: ChartTimes,
	cachedStats?: { created: string | number | null }[],
	createdIsNumber?: boolean
): Promise<T[]> {
	const lastCached = cachedStats?.at(-1)?.created as number
	return await pb.collection<T>(collection).getFullList({
		filter: pb.filter("system={:id} && created > {:created} && type={:type}", {
			id: systemId,
			created: getPbTimestamp(chartTime, lastCached ? new Date(lastCached + 1000) : undefined, createdIsNumber),
			type: chartTimeData[chartTime].type,
		}),
		fields: "created,stats",
		sort: "created",
	})
}

export function makeContainerData(containers: ContainerStatsRecord[]): ChartDataContainer[] {
	const result = [] as ChartDataContainer[]
	for (const { created, stats } of containers) {
		if (!created) {
			result.push({ created: null } as ChartDataContainer)
			continue
		}
		result.push(makeContainerPoint(new Date(created).getTime(), stats))
	}
	return result
}

/** Transform a single realtime container stats message into a ChartDataContainer point. */
export function makeContainerPoint(created: number, stats: ContainerStatsRecord["stats"]): ChartDataContainer {
	const point: ChartDataContainer = { created } as ChartDataContainer
	for (const container of stats) {
		;(point as Record<string, unknown>)[container.n] = container
	}
	return point
}

export function dockerOrPodman(str: string, isPodman: boolean): string {
	if (isPodman) {
		return str.replace("docker", "podman").replace("Docker", "Podman")
	}
	return str
}
