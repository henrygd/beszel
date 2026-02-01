import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { timeTicks } from "d3-time"
import { subscribeKeys } from "nanostores"
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useContainerChartConfigs } from "@/components/charts/hooks"
import { getPbTimestamp, pb } from "@/lib/api"
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
import {
	chartTimeData,
	compareSemVer,
	listen,
	parseSemVer,
	useBrowserStorage,
} from "@/lib/utils"
import type {
	ChartData,
	ChartTimes,
	ContainerStatsRecord,
	SystemDetailsRecord,
	SystemInfo,
	SystemRecord,
	SystemStats,
	SystemStatsRecord,
} from "@/types"
import { $router, navigate } from "../router"
import { Card } from "../ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../ui/tabs"
import InfoBar from "./system/info-bar"
import {
	CoreMetricsTab,
	DisksTab,
	GpuTab,
	ContainersTab,
	ServicesTab,
	FilterBar,
	SelectAvgMax,
} from "./system/tabs"

type ChartTimeData = {
	time: number
	data: {
		ticks: number[]
		domain: number[]
	}
	chartTime: ChartTimes
}

const cache = new Map<string, ChartTimeData | SystemStatsRecord[] | ContainerStatsRecord[]>()

// create ticks and domain for charts
function getTimeData(chartTime: ChartTimes, lastCreated: number) {
	const cached = cache.get("td") as ChartTimeData | undefined
	if (cached && cached.chartTime === chartTime) {
		if (!lastCreated || cached.time >= lastCreated) {
			return cached.data
		}
	}

	const now = new Date(Date.now())
	const startTime = chartTimeData[chartTime].getOffset(now)
	const ticks = timeTicks(startTime, now, chartTimeData[chartTime].ticks ?? 12).map((date) => date.getTime())
	const data = {
		ticks,
		domain: [chartTimeData[chartTime].getOffset(now).getTime(), now.getTime()],
	}
	cache.set("td", { time: now.getTime(), data, chartTime })
	return data
}

// add empty values between records to make gaps if interval is too large
function addEmptyValues<T extends { created: string | number | null }>(
	prevRecords: T[],
	newRecords: T[],
	expectedInterval: number
): T[] {
	const modifiedRecords: T[] = []
	let prevTime = (prevRecords.at(-1)?.created ?? 0) as number
	for (let i = 0; i < newRecords.length; i++) {
		const record = newRecords[i]
		if (record.created !== null) {
			record.created = new Date(record.created).getTime()
		}
		if (prevTime && record.created !== null) {
			const interval = record.created - prevTime
			// if interval is too large, add a null record
			if (interval > expectedInterval / 2 + expectedInterval) {
				modifiedRecords.push({ created: null, ...("stats" in record ? { stats: null } : {}) } as T)
			}
		}
		if (record.created !== null) {
			prevTime = record.created
		}
		modifiedRecords.push(record)
	}
	return modifiedRecords
}

async function getStats<T extends SystemStatsRecord | ContainerStatsRecord>(
	collection: string,
	system: SystemRecord,
	chartTime: ChartTimes
): Promise<T[]> {
	const cachedStats = cache.get(`${system.id}_${chartTime}_${collection}`) as T[] | undefined
	const lastCached = cachedStats?.at(-1)?.created as number
	return await pb.collection<T>(collection).getFullList({
		filter: pb.filter("system={:id} && created > {:created} && type={:type}", {
			id: system.id,
			created: getPbTimestamp(chartTime, lastCached ? new Date(lastCached + 1000) : undefined),
			type: chartTimeData[chartTime].type,
		}),
		fields: "created,stats",
		sort: "created",
	})
}

export default memo(function SystemDetail({ id }: { id: string }) {
	const systems = useStore($systems)
	const chartTime = useStore($chartTime)
	const maxValues = useStore($maxValues)
	const [grid, setGrid] = useBrowserStorage("grid", true)
	const [system, setSystem] = useState({} as SystemRecord)
	const [systemStats, setSystemStats] = useState([] as SystemStatsRecord[])
	const [containerData, setContainerData] = useState([] as ChartData["containerData"])
	const temperatureChartRef = useRef<HTMLDivElement>(null)
	const persistChartTime = useRef(false)
	const [bottomSpacing, setBottomSpacing] = useState(0)
	const [chartLoading, setChartLoading] = useState(true)
	const isLongerChart = !["1m", "1h"].includes(chartTime)
	const userSettings = $userSettings.get()
	const chartWrapRef = useRef<HTMLDivElement>(null)
	const [activeTab, setActiveTab] = useBrowserStorage("system-tab", "overview")
	const [details, setDetails] = useState<SystemDetailsRecord>({} as SystemDetailsRecord)
	const direction = useStore($direction)

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
	// biome-ignore lint/correctness/useExhaustiveDependencies: not necessary
	useEffect(() => {
		let unsub = () => { }
		if (!system.id || chartTime !== "1m") {
			return
		}
		if (system.status !== SystemStatus.Up || parseSemVer(system?.info?.v).minor < 13) {
			$chartTime.set("1h")
			return
		}
		pb.realtime
			.subscribe(
				`rt_metrics`,
				(data: { container: ContainerStatsRecord[]; info: SystemInfo; stats: SystemStats }) => {
					if (data.container?.length > 0) {
						const newContainerData = makeContainerData([
							{ created: Date.now(), stats: data.container } as unknown as ContainerStatsRecord,
						])
						setContainerData((prevData) => addEmptyValues(prevData, prevData.slice(-59).concat(newContainerData), 1000))
					}
					setSystemStats((prevStats) =>
						addEmptyValues(
							prevStats,
							prevStats.slice(-59).concat({ created: Date.now(), stats: data.stats } as SystemStatsRecord),
							1000
						)
					)
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

	// biome-ignore lint/correctness/useExhaustiveDependencies: not necessary
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
			agentVersion: parseSemVer(system?.info?.v),
		}
	}, [systemStats, containerData, direction])

	// Share chart config computation for all container charts
	const containerChartConfigs = useContainerChartConfigs(containerData)

	// make container stats for charts
	const makeContainerData = useCallback((containers: ContainerStatsRecord[]) => {
		const containerData = [] as ChartData["containerData"]
		for (let { created, stats } of containers) {
			if (!created) {
				// @ts-expect-error add null value for gaps
				containerData.push({ created: null })
				continue
			}
			created = new Date(created).getTime()
			// @ts-expect-error not dealing with this rn
			const containerStats: ChartData["containerData"][0] = { created }
			for (const container of stats) {
				containerStats[container.n] = container
			}
			containerData.push(containerStats)
		}
		return containerData
	}, [])

	// get stats
	// biome-ignore lint/correctness/useExhaustiveDependencies: not necessary
	useEffect(() => {
		if (!system.id || !chartTime || chartTime === "1m") {
			return
		}
		// loading: true
		setChartLoading(true)
		Promise.allSettled([
			getStats<SystemStatsRecord>("system_stats", system, chartTime),
			getStats<ContainerStatsRecord>("container_stats", system, chartTime),
		]).then(([systemStats, containerStats]) => {
			// loading: false
			setChartLoading(false)

			const { expectedInterval } = chartTimeData[chartTime]
			// make new system stats
			const ss_cache_key = `${system.id}_${chartTime}_system_stats`
			let systemData = (cache.get(ss_cache_key) || []) as SystemStatsRecord[]
			if (systemStats.status === "fulfilled" && systemStats.value.length) {
				systemData = systemData.concat(addEmptyValues(systemData, systemStats.value, expectedInterval))
				if (systemData.length > 120) {
					systemData = systemData.slice(-100)
				}
				cache.set(ss_cache_key, systemData)
			}
			setSystemStats(systemData)
			// make new container stats
			const cs_cache_key = `${system.id}_${chartTime}_container_stats`
			let containerData = (cache.get(cs_cache_key) || []) as ContainerStatsRecord[]
			if (containerStats.status === "fulfilled" && containerStats.value.length) {
				containerData = containerData.concat(addEmptyValues(containerData, containerStats.value, expectedInterval))
				if (containerData.length > 120) {
					containerData = containerData.slice(-100)
				}
				cache.set(cs_cache_key, containerData)
			}
			setContainerData(makeContainerData(containerData))
		})
	}, [system, chartTime])

	/** Space for tooltip if more than 10 sensors and no containers table */
	useEffect(() => {
		const sensors = Object.keys(systemStats.at(-1)?.stats.t ?? {})
		if (!temperatureChartRef.current || sensors.length < 10 || containerData.length > 0) {
			setBottomSpacing(0)
			return
		}
		const tooltipHeight = (sensors.length - 10) * 17.8 - 40
		const wrapperEl = chartWrapRef.current as HTMLDivElement
		const wrapperRect = wrapperEl.getBoundingClientRect()
		const chartRect = temperatureChartRef.current.getBoundingClientRect()
		const distanceToBottom = wrapperRect.bottom - chartRect.bottom
		setBottomSpacing(tooltipHeight - distanceToBottom)
	}, [])

	// keyboard navigation between systems
	useEffect(() => {
		if (!systems.length) {
			return
		}
		const handleKeyUp = (e: KeyboardEvent) => {
			if (
				e.target instanceof HTMLInputElement ||
				e.target instanceof HTMLTextAreaElement ||
				e.shiftKey ||
				e.ctrlKey ||
				e.metaKey ||
				e.altKey
			) {
				return
			}
			const currentIndex = systems.findIndex((s) => s.id === id)
			if (currentIndex === -1 || systems.length <= 1) {
				return
			}
			switch (e.key) {
				case "ArrowLeft":
				case "h": {
					const prevIndex = (currentIndex - 1 + systems.length) % systems.length
					persistChartTime.current = true
					return navigate(getPagePath($router, "system", { id: systems[prevIndex].id }))
				}
				case "ArrowRight":
				case "l": {
					const nextIndex = (currentIndex + 1) % systems.length
					persistChartTime.current = true
					return navigate(getPagePath($router, "system", { id: systems[nextIndex].id }))
				}
			}
		}
		return listen(document, "keyup", handleKeyUp)
	}, [id, systems])

	if (!system.id) {
		return null
	}

	// select field for switching between avg and max values
	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null
	const showMax = maxValues && isLongerChart

	const containerFilterBar = containerData.length ? <FilterBar /> : null

	const dataEmpty = !chartLoading && chartData.systemStats.length === 0
	const lastGpus = systemStats.at(-1)?.stats?.g

	let hasGpuData = false
	let hasGpuEnginesData = false
	let hasGpuPowerData = false

	if (lastGpus) {
		// check if there are any GPUs at all
		hasGpuData = Object.keys(lastGpus).length > 0
		// check if there are any GPUs with engines or power data
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

	const isLinux = !(details?.os ?? system.info?.os)

	return (
		<>
			<div ref={chartWrapRef} className="grid gap-4 mb-14 overflow-x-clip">
				{/* system info */}
				<InfoBar system={system} chartData={chartData} grid={grid} setGrid={setGrid} details={details} />

				<Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
					<Card className="p-1">
						<TabsList className="w-full h-11 grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
							<TabsTrigger value="overview" className="h-9">
								<Trans>Core Metrics</Trans>
							</TabsTrigger>
							<TabsTrigger value="disks" className="h-9">
								<Trans>Disks</Trans>
							</TabsTrigger>
							{hasGpuData && (
								<TabsTrigger value="gpu" className="h-9">
									<Trans>GPU</Trans>
								</TabsTrigger>
							)}
							{containerData.length > 0 && compareSemVer(chartData.agentVersion, parseSemVer("0.14.0")) >= 0 && (
								<TabsTrigger value="containers" className="h-9">
									<Trans>Containers</Trans>
								</TabsTrigger>
							)}
							{isLinux && compareSemVer(chartData.agentVersion, parseSemVer("0.16.0")) >= 0 && (
								<TabsTrigger value="services" className="h-9">
									<Trans>Services</Trans>
								</TabsTrigger>
							)}
						</TabsList>
					</Card>

					<TabsContent value="overview" className="mt-4">
						<CoreMetricsTab
							chartData={chartData}
							grid={grid}
							dataEmpty={dataEmpty}
							maxValSelect={maxValSelect}
							showMax={showMax}
							systemStats={systemStats}
							temperatureChartRef={temperatureChartRef}
							maxValues={maxValues}
							userSettings={userSettings}
						/>
					</TabsContent>

					<TabsContent value="disks" className="mt-4">
						<DisksTab
							chartData={chartData}
							grid={grid}
							dataEmpty={dataEmpty}
							maxValSelect={maxValSelect}
							showMax={showMax}
							systemStats={systemStats}
							systemId={system.id}
							userSettings={userSettings}
						/>
					</TabsContent>

					{hasGpuData && (
						<TabsContent value="gpu" className="mt-4">
							<GpuTab
								chartData={chartData}
								grid={grid}
								dataEmpty={dataEmpty}
								hasGpuPowerData={hasGpuPowerData}
								hasGpuEnginesData={hasGpuEnginesData}
								systemStats={systemStats}
							/>
						</TabsContent>
					)}

					{containerData.length > 0 && compareSemVer(chartData.agentVersion, parseSemVer("0.14.0")) >= 0 && (
						<TabsContent value="containers" className="mt-4">
							<ContainersTab
								chartData={chartData}
								grid={grid}
								dataEmpty={dataEmpty}
								containerFilterBar={containerFilterBar}
								containerChartConfigs={containerChartConfigs}
								system={system}
							/>
						</TabsContent>
					)}

					{isLinux && compareSemVer(chartData.agentVersion, parseSemVer("0.16.0")) >= 0 && (
						<TabsContent value="services" className="mt-4">
							<ServicesTab systemId={system.id} />
						</TabsContent>
					)}
				</Tabs>
			</div>

			{/* add space for tooltip if lots of sensors */}
			{bottomSpacing > 0 && <span className="block" style={{ height: bottomSpacing }} />}
		</>
	)
})
