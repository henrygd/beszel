import { chartTimeData } from "@/lib/utils"
import type { ChartTimes, SpeedtestRecord, SpeedtestStatsRecord } from "@/types"
import { useEffect, useMemo, useRef, useState } from "react"
import { pb, getPbTimestamp } from "@/lib/api"
import { toast } from "@/components/ui/use-toast"
import { applyMonitorEvents } from "@/lib/use-network-monitors"
import { mergeSpeedtestRuns } from "@/lib/speedtest-compare"
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

/**
 * Load the runs of one or more speedtests within the chart time range, append new runs in realtime,
 * and merge them for the charts (see mergeSpeedtestRuns).
 */
export function useSpeedtestStats({
	speedtests,
	chartTime,
	enabled = true,
}: {
	speedtests: Pick<SpeedtestRecord, "id" | "interval">[]
	chartTime: ChartTimes
	enabled?: boolean
}) {
	const key = `${chartTime}:${speedtests.map((s) => s.id).join(",")}`
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
				// Runs that arrived in realtime during the fetch are kept; merging drops duplicates.
				if (!cancelled) setResult((prev) => ({ key, runs: prev.key === key ? [...runs, ...prev.runs] : runs }))
			})
			.catch((error) => {
				if (!cancelled) console.error("Failed to fetch speedtest stats:", error)
			})

		;(async () => {
			try {
				unsubscribe = await pb.collection<SpeedtestStatsRecord>("speedtest_stats").subscribe(
					"*",
					(event) => {
						if (cancelled || event.action !== "create") return
						// Drop runs that have fallen out of the selected time range.
						const cutoff = chartTimeData[chartTime].getOffset(new Date()).getTime()
						setResult((prev) => {
							const kept = prev.key === key ? prev.runs.filter((run) => (run.created ?? 0) > cutoff) : []
							return { key, runs: [...kept, event.record] }
						})
					},
					{ fields: SPEEDTEST_STATS_FIELDS, filter: pb.filter(idsFilter, params) }
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
	}, [key, chartTime, enabled])

	const intervals = useMemo(() => new Map(speedtests.map((s) => [s.id, s.interval * 60_000])), [speedtests])
	const runs = result.key === key ? result.runs : []
	return useMemo(() => mergeSpeedtestRuns(runs, intervals), [runs, intervals])
}
