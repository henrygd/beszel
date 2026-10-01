import { chartTimeData } from "@/lib/utils"
import type { ChartTimes, SpeedtestRecord, SpeedtestStatsRecord } from "@/types"
import { useEffect, useMemo, useRef, useState } from "react"
import { appendData } from "@/components/routes/system/chart-data"
import { pb, getPbTimestamp } from "@/lib/api"
import { toast } from "@/components/ui/use-toast"
import { applyMonitorEvents } from "@/lib/use-network-monitors"
import { mergeSpeedtestCompareStats } from "@/lib/speedtest-compare"
import type { RecordListOptions, RecordSubscription } from "pocketbase"

const SPEEDTEST_FIELDS =
	"id,system,server_id,interface,interval,enabled,download,upload,ping,jitter,loss,download_latency,download_jitter,upload_latency,upload_jitter,server_name,server_location,isp,url,error,last_run,updated"

const SPEEDTEST_STATS_FIELDS =
	"speedtest,download,upload,ping,jitter,loss,server_name,error,created,download_latency,download_jitter,upload_latency,upload_jitter"

async function fetchSpeedtests(system?: string) {
	try {
		return await pb.collection<SpeedtestRecord>("speedtests").getFullList({
			fields: SPEEDTEST_FIELDS,
			filter: system ? pb.filter("system={:system}", { system }) : undefined,
		})
	} catch (error) {
		toast({
			title: "Error",
			description: (error as Error)?.message,
			variant: "destructive",
		})
		return []
	}
}

/** Load speedtests, optionally for one system, and keep them updated in realtime. */
export function useSpeedtests({ systemId }: { systemId?: string }) {
	const [speedtests, setSpeedtests] = useState<SpeedtestRecord[]>([])
	const [isLoading, setIsLoading] = useState(true)
	const pendingEvents = useRef(new Map<string, RecordSubscription<SpeedtestRecord>>())
	const batchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

	useEffect(() => {
		let cancelled = false
		setIsLoading(true)
		setSpeedtests([])
		fetchSpeedtests(systemId).then((speedtests) => {
			if (cancelled) return
			setSpeedtests(speedtests)
			setIsLoading(false)
		})
		return () => {
			cancelled = true
		}
	}, [systemId])

	useEffect(() => {
		let cancelled = false
		let unsubscribe: (() => void) | undefined

		function flushPendingEvents() {
			batchTimeout.current = null
			if (!pendingEvents.current.size) {
				return
			}
			const events = pendingEvents.current
			pendingEvents.current = new Map()
			setSpeedtests((current) => applyMonitorEvents(current, events.values(), systemId))
		}

		const pbOptions: RecordListOptions = { fields: SPEEDTEST_FIELDS }
		if (systemId) {
			pbOptions.filter = pb.filter("system = {:system}", { system: systemId })
		}

		;(async () => {
			try {
				unsubscribe = await pb.collection<SpeedtestRecord>("speedtests").subscribe(
					"*",
					(event) => {
						pendingEvents.current.set(event.record.id, event)
						if (!batchTimeout.current) {
							batchTimeout.current = setTimeout(flushPendingEvents, 50)
						}
					},
					pbOptions
				)
				if (cancelled) unsubscribe()
			} catch (error) {
				console.error("Failed to subscribe to speedtests", error)
			}
		})()

		return () => {
			cancelled = true
			if (batchTimeout.current !== null) {
				clearTimeout(batchTimeout.current)
				batchTimeout.current = null
			}
			pendingEvents.current.clear()
			unsubscribe?.()
		}
	}, [systemId])

	return { speedtests, isLoading }
}

const statsCache = new Map<string, SpeedtestStatsRecord[]>()

/** Load the runs of one speedtest within the chart time range, and append new runs in realtime. */
export function useSpeedtestStats({
	speedtest,
	chartTime,
	enabled = true,
}: {
	speedtest: Pick<SpeedtestRecord, "id" | "interval">
	chartTime: ChartTimes
	enabled?: boolean
}) {
	const { id, interval } = speedtest
	const cacheKey = `${id}:${chartTime}`
	const [stats, setStats] = useState<SpeedtestStatsRecord[]>(() => statsCache.get(cacheKey) ?? [])
	// Missing more than one run leaves a gap in the charts.
	const expectedInterval = interval * 60_000

	useEffect(() => {
		if (!enabled) {
			return
		}
		let cancelled = false
		let unsubscribe: (() => void) | undefined
		const cached = statsCache.get(cacheKey) ?? []
		setStats(cached)

		const append = (newStats: SpeedtestStatsRecord[]) => {
			const lastCreated = (statsCache.get(cacheKey)?.at(-1)?.created as number | undefined) ?? 0
			const fresh = newStats.filter((record) => (record.created ?? 0) > lastCreated)
			if (!fresh.length) return
			// Drop runs that have fallen out of the selected time range.
			const cutoff = chartTimeData[chartTime].getOffset(new Date()).getTime()
			const kept = (statsCache.get(cacheKey) ?? []).filter(
				(record) => record.created === null || record.created > cutoff
			)
			while (kept[0]?.created === null) kept.shift()
			const updated = appendData(kept, fresh, expectedInterval)
			statsCache.set(cacheKey, updated)
			setStats(updated)
		}

		const lastCached = cached.at(-1)?.created
		pb.collection<SpeedtestStatsRecord>("speedtest_stats")
			.getFullList({
				filter: pb.filter("speedtest={:id} && created>{:created}", {
					id,
					created: lastCached ?? getPbTimestamp(chartTime, undefined, true),
				}),
				fields: SPEEDTEST_STATS_FIELDS,
				sort: "created",
			})
			.then((records) => {
				if (!cancelled) append(records)
			})
			.catch((error) => {
				if (!cancelled) console.error("Failed to fetch speedtest stats:", error)
			})

		;(async () => {
			try {
				unsubscribe = await pb.collection<SpeedtestStatsRecord>("speedtest_stats").subscribe(
					"*",
					(event) => {
						if (!cancelled && event.action === "create") append([event.record])
					},
					{ fields: SPEEDTEST_STATS_FIELDS, filter: pb.filter("speedtest={:id}", { id }) }
				)
				if (cancelled) unsubscribe()
			} catch (error) {
				console.error("Failed to subscribe to speedtest stats:", error)
			}
		})()

		return () => {
			cancelled = true
			unsubscribe?.()
		}
	}, [id, chartTime, cacheKey, expectedInterval, enabled])

	// Replace failed runs with gap markers, so the line breaks there without leaving points that
	// have no measurements for the tooltip to show. Done here rather than in the cache, which
	// relies on the last run's timestamp to skip runs it already has.
	return useMemo(() => stats.map((record) => (record.error ? speedtestGapRecord : record)), [stats])
}

/** Gap marker in the same form appendData uses. */
const speedtestGapRecord = { created: null } as SpeedtestStatsRecord

/** Only what comparison charts and labels need. */
const COMPARE_SPEEDTEST_FIELDS = "id,system,server_id,server_name,server_location,interface,interval"

/**
 * Speedtests on all systems except the given one, to compare against.
 * Fetched per open so it also works in single-system tables, which only hold one system's speedtests.
 */
export function useCompareSpeedtests(system: string, enabled = true) {
	const [result, setResult] = useState<{ system: string; speedtests: SpeedtestRecord[] }>({ system, speedtests: [] })

	useEffect(() => {
		if (!enabled) return
		let cancelled = false
		pb.collection<SpeedtestRecord>("speedtests")
			.getFullList({
				fields: COMPARE_SPEEDTEST_FIELDS,
				filter: pb.filter("system!={:system}", { system }),
			})
			.then((speedtests) => {
				if (!cancelled) setResult({ system, speedtests })
			})
			.catch((error) => {
				if (!cancelled) console.error("Failed to fetch compare speedtests:", error)
			})
		return () => {
			cancelled = true
		}
	}, [system, enabled])

	// Keep showing the last result while reopening refreshes it, but never another system's.
	return result.system === system ? result.speedtests : []
}

/** Load the runs of several speedtests within the chart time range, and append new runs in realtime. */
export function useSpeedtestCompareStats({
	speedtestIds,
	chartTime,
	enabled = true,
}: {
	speedtestIds: string[]
	chartTime: ChartTimes
	enabled?: boolean
}) {
	const key = `${chartTime}:${speedtestIds.join(",")}`
	const [result, setResult] = useState<{ key: string; runs: SpeedtestStatsRecord[] }>({ key, runs: [] })

	useEffect(() => {
		if (!enabled) return
		const ids = key.slice(key.indexOf(":") + 1).split(",")
		let cancelled = false
		let unsubscribe: (() => void) | undefined
		const params = Object.fromEntries(ids.map((id, i) => [`id${i}`, id]))
		const idsFilter = `(${ids.map((_, i) => `speedtest={:id${i}}`).join(" || ")})`

		pb.collection<SpeedtestStatsRecord>("speedtest_stats")
			.getFullList({
				filter: pb.filter(`${idsFilter} && created>{:created}`, {
					...params,
					created: getPbTimestamp(chartTime, undefined, true),
				}),
				fields: SPEEDTEST_STATS_FIELDS,
				sort: "created",
			})
			.then((runs) => {
				if (!cancelled) setResult((prev) => ({ key, runs: prev.key === key ? [...runs, ...prev.runs] : runs }))
			})
			.catch((error) => {
				if (!cancelled) console.error("Failed to fetch speedtest compare stats:", error)
			})

		;(async () => {
			try {
				unsubscribe = await pb.collection<SpeedtestStatsRecord>("speedtest_stats").subscribe(
					"*",
					(event) => {
						if (cancelled || event.action !== "create") return
						setResult((prev) => ({ key, runs: prev.key === key ? [...prev.runs, event.record] : [event.record] }))
					},
					{ fields: SPEEDTEST_STATS_FIELDS, filter: pb.filter(idsFilter, params) }
				)
				if (cancelled) unsubscribe()
			} catch (error) {
				console.error("Failed to subscribe to speedtest compare stats:", error)
			}
		})()

		return () => {
			cancelled = true
			unsubscribe?.()
		}
	}, [key, chartTime, enabled])

	const runs = result.key === key ? result.runs : []
	return useMemo(() => mergeSpeedtestCompareStats(runs), [runs])
}
