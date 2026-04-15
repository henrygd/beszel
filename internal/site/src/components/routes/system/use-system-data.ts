import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { subscribeKeys } from "nanostores"
import { useEffect, useMemo, useRef, useState } from "react"
import { useContainerChartConfigs } from "@/components/charts/hooks"
import { pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import {
	$allSystemsById,
	$allSystemsByName,
	$chartTime,
	$containerFilter,
	$direction,
	$maxValues,
	$systems,
	$userSettings,
} from "@/lib/stores"
import { chartTimeData, listen, parseSemVer, useBrowserStorage } from "@/lib/utils"
import type {
	ChartData,
	ContainerStatsRecord,
	SystemDetailsRecord,
	SystemInfo,
	SystemRecord,
	SystemStats,
	SystemStatsRecord,
} from "@/types"
import { $router, navigate } from "../../router"
import { appendData, cache, getStats, getTimeData, makeContainerData, makeContainerPoint } from "./chart-data"

export type SystemData = ReturnType<typeof useSystemData>

export function useSystemData(id: string) {
	const direction = useStore($direction)
	const systems = useStore($systems)
	const chartTime = useStore($chartTime)
	const maxValues = useStore($maxValues)
	const [grid, setGrid] = useBrowserStorage("grid", true)
	const [displayMode, setDisplayMode] = useBrowserStorage<"default" | "tabs">("displayMode", "default")
	const [activeTab, setActiveTabRaw] = useState("core")
	const [mountedTabs, setMountedTabs] = useState(() => new Set<string>(["core"]))
	const tabsRef = useRef<string[]>(["core", "disk"])

	function setActiveTab(tab: string) {
		setActiveTabRaw(tab)
		setMountedTabs((prev) => (prev.has(tab) ? prev : new Set([...prev, tab])))
	}
	const [system, setSystem] = useState({} as SystemRecord)
	const [systemStats, setSystemStats] = useState([] as SystemStatsRecord[])
	const [containerData, setContainerData] = useState([] as ChartData["containerData"])
	const persistChartTime = useRef(false)
	const statsRequestId = useRef(0)
	const [chartLoading, setChartLoading] = useState(true)
	const [details, setDetails] = useState<SystemDetailsRecord>({} as SystemDetailsRecord)

	useEffect(() => {
		return () => {
			if (!persistChartTime.current) {
				$chartTime.set($userSettings.get().chartTime)
			}
			persistChartTime.current = false
			setSystemStats([])
			setContainerData([])
			setDetails({} as SystemDetailsRecord)
			$containerFilter.set("")
		}
	}, [id])

	// find matching system and update when it changes
	useEffect(() => {
		if (!systems.length) {
			return
		}
		// allow old system-name slug to work
		const store = $allSystemsById.get()[id] ? $allSystemsById : $allSystemsByName
		return subscribeKeys(store, [id], (newSystems) => {
			const sys = newSystems[id]
			if (sys) {
				setSystem(sys)
				document.title = `${sys?.name} / Beszel`
			}
		})
	}, [id, systems.length])

	// hide 1m chart time if system agent version is less than 0.13.0
	useEffect(() => {
		if (parseSemVer(system?.info?.v) < parseSemVer("0.13.0")) {
			$chartTime.set("1h")
		}
	}, [system?.info?.v])

	// fetch system details
	useEffect(() => {
		// if system.info.m exists, agent is old version without system details
		if (!system.id || system.info?.m) {
			return
		}
		pb.collection<SystemDetailsRecord>("system_details")
			.getOne(system.id, {
				fields: "hostname,kernel,cores,threads,cpu,os,os_name,arch,memory,podman",
				headers: {
					"Cache-Control": "public, max-age=60",
				},
			})
			.then(setDetails)
	}, [system.id])

	// subscribe to realtime metrics if chart time is 1m
	useEffect(() => {
		let unsub = () => {}
		if (!system.id || chartTime !== "1m") {
			return
		}
		if (system.status !== SystemStatus.Up || parseSemVer(system?.info?.v).minor < 13) {
			$chartTime.set("1h")
			return
		}
		let isFirst = true
		pb.realtime
			.subscribe(
				`rt_metrics`,
				(data: { container: ContainerStatsRecord[]; info: SystemInfo; stats: SystemStats }) => {
					const now = Date.now()
					const statsPoint = { created: now, stats: data.stats } as SystemStatsRecord
					const containerPoint =
						data.container?.length > 0
							? makeContainerPoint(now, data.container as unknown as ContainerStatsRecord["stats"])
							: null
					// on first message, make sure we clear out data from other time periods
					if (isFirst) {
						isFirst = false
						setSystemStats([statsPoint])
						setContainerData(containerPoint ? [containerPoint] : [])
						return
					}
					setSystemStats((prev) => appendData(prev, [statsPoint], 1000, 60))
					if (containerPoint) {
						setContainerData((prev) => appendData(prev, [containerPoint], 1000, 60))
					}
				},
				{ query: { system: system.id } }
			)
			.then((us) => {
				unsub = us
			})
		return () => {
			unsub?.()
		}
	}, [chartTime, system.id])

	const agentVersion = useMemo(() => parseSemVer(system?.info?.v), [system?.info?.v])

	const chartData: ChartData = useMemo(() => {
		const lastCreated = Math.max(
			(systemStats.at(-1)?.created as number) ?? 0,
			(containerData.at(-1)?.created as number) ?? 0
		)
		return {
			systemStats,
			containerData,
			chartTime,
			orientation: direction === "rtl" ? "right" : "left",
			...getTimeData(chartTime, lastCreated),
			agentVersion,
		}
	}, [systemStats, containerData, direction])

	// Share chart config computation for all container charts
	const containerChartConfigs = useContainerChartConfigs(containerData)

	// get stats when system "changes." (Not just system to system,
	// also when new info comes in via systemManager realtime connection, indicating an update)
	useEffect(() => {
		if (!system.id || !chartTime || chartTime === "1m") {
			return
		}

		const systemId = system.id
		const { expectedInterval } = chartTimeData[chartTime]
		const ss_cache_key = `${systemId}_${chartTime}_system_stats`
		const cs_cache_key = `${systemId}_${chartTime}_container_stats`
		const requestId = ++statsRequestId.current

		const cachedSystemStats = cache.get(ss_cache_key) as SystemStatsRecord[] | undefined
		const cachedContainerData = cache.get(cs_cache_key) as ChartData["containerData"] | undefined

		// Render from cache immediately if available
		if (cachedSystemStats?.length) {
			setSystemStats(cachedSystemStats)
			setContainerData(cachedContainerData || [])
			setChartLoading(false)

			// Skip the fetch if the latest cached point is recent enough that no new point is expected yet
			const lastCreated = cachedSystemStats.at(-1)?.created as number | undefined
			if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
				return
			}
		} else {
			setChartLoading(true)
		}

		Promise.allSettled([
			getStats<SystemStatsRecord>("system_stats", systemId, chartTime),
			getStats<ContainerStatsRecord>("container_stats", systemId, chartTime),
		]).then(([systemStats, containerStats]) => {
			// If another request has been made since this one, ignore the results
			if (requestId !== statsRequestId.current) {
				return
			}

			setChartLoading(false)

			// make new system stats
			let systemData = (cache.get(ss_cache_key) || []) as SystemStatsRecord[]
			if (systemStats.status === "fulfilled" && systemStats.value.length) {
				systemData = appendData(systemData, systemStats.value, expectedInterval, 100)
				cache.set(ss_cache_key, systemData)
			}
			setSystemStats(systemData)
			// make new container stats
			let containerData = (cache.get(cs_cache_key) || []) as ChartData["containerData"]
			if (containerStats.status === "fulfilled" && containerStats.value.length) {
				containerData = appendData(containerData, makeContainerData(containerStats.value), expectedInterval, 100)
				cache.set(cs_cache_key, containerData)
			}
			setContainerData(containerData)
		})
	}, [system, chartTime])

	// keyboard navigation between systems
	// in tabs mode: arrow keys switch tabs, shift+arrow switches systems
	// in default mode: arrow keys switch systems
	useEffect(() => {
		if (!systems.length) {
			return
		}
		const handleKeyUp = (e: KeyboardEvent) => {
			if (
				e.target instanceof HTMLInputElement ||
				e.target instanceof HTMLTextAreaElement ||
				e.ctrlKey ||
				e.metaKey ||
				e.altKey
			) {
				return
			}

			const isLeft = e.key === "ArrowLeft" || e.key === "h"
			const isRight = e.key === "ArrowRight" || e.key === "l"
			if (!isLeft && !isRight) {
				return
			}

			// in tabs mode, plain arrows switch tabs, shift+arrows switch systems
			if (displayMode === "tabs") {
				if (!e.shiftKey) {
					// skip if focused in tablist (Radix handles it natively)
					if (e.target instanceof HTMLElement && e.target.closest('[role="tablist"]')) {
						return
					}
					const tabs = tabsRef.current
					const currentIdx = tabs.indexOf(activeTab)
					const nextIdx = isLeft ? (currentIdx - 1 + tabs.length) % tabs.length : (currentIdx + 1) % tabs.length
					setActiveTab(tabs[nextIdx])
					return
				}
			} else if (e.shiftKey) {
				return
			}

			const currentIndex = systems.findIndex((s) => s.id === id)
			if (currentIndex === -1 || systems.length <= 1) {
				return
			}
			if (isLeft) {
				const prevIndex = (currentIndex - 1 + systems.length) % systems.length
				persistChartTime.current = true
				setActiveTabRaw("core")
				setMountedTabs(new Set(["core"]))
				return navigate(getPagePath($router, "system", { id: systems[prevIndex].id }))
			}
			if (isRight) {
				const nextIndex = (currentIndex + 1) % systems.length
				persistChartTime.current = true
				setActiveTabRaw("core")
				setMountedTabs(new Set(["core"]))
				return navigate(getPagePath($router, "system", { id: systems[nextIndex].id }))
			}
		}
		return listen(document, "keyup", handleKeyUp)
	}, [id, systems, displayMode, activeTab])

	// derived values
	const isLongerChart = !["1m", "1h"].includes(chartTime)
	const showMax = maxValues && isLongerChart
	const dataEmpty = !chartLoading && chartData.systemStats.length === 0
	const lastGpus = systemStats.at(-1)?.stats?.g
	const isPodman = details?.podman ?? system.info?.p ?? false

	let hasGpuData = false
	let hasGpuEnginesData = false
	let hasGpuPowerData = false

	if (lastGpus) {
		hasGpuData = Object.keys(lastGpus).length > 0
		for (let i = 0; i < systemStats.length && (!hasGpuEnginesData || !hasGpuPowerData); i++) {
			const gpus = systemStats[i].stats?.g
			if (!gpus) continue
			for (const id in gpus) {
				if (!hasGpuEnginesData && gpus[id].e !== undefined) {
					hasGpuEnginesData = true
				}
				if (!hasGpuPowerData && (gpus[id].p !== undefined || gpus[id].pp !== undefined)) {
					hasGpuPowerData = true
				}
				if (hasGpuEnginesData && hasGpuPowerData) break
			}
		}
	}

	return {
		system,
		systemStats,
		containerData,
		chartData,
		containerChartConfigs,
		details,
		grid,
		setGrid,
		displayMode,
		setDisplayMode,
		activeTab,
		setActiveTab,
		mountedTabs,
		tabsRef,
		maxValues,
		isLongerChart,
		showMax,
		dataEmpty,
		isPodman,
		lastGpus,
		hasGpuData,
		hasGpuEnginesData,
		hasGpuPowerData,
	}
}
