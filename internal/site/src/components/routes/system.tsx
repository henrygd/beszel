import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { timeTicks } from "d3-time"
import { XIcon } from "lucide-react"
import { subscribeKeys } from "nanostores"
import React, { type JSX, lazy, memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import AreaChartDefault, { type DataPoint } from "@/components/charts/area-chart"
import ContainerChart from "@/components/charts/container-chart"
import DiskChart from "@/components/charts/disk-chart"
import GpuPowerChart from "@/components/charts/gpu-power-chart"
import { useContainerChartConfigs } from "@/components/charts/hooks"
import LoadAverageChart from "@/components/charts/load-average-chart"
import MemChart from "@/components/charts/mem-chart"
import SwapChart from "@/components/charts/swap-chart"
import TemperatureChart from "@/components/charts/temperature-chart"
import { getPbTimestamp, pb } from "@/lib/api"
import { ChartType, SystemStatus, Unit } from "@/lib/enums"
import { batteryStateTranslations } from "@/lib/i18n"
import {
	$allSystemsById,
	$allSystemsByName,
	$chartTime,
	$containerFilter,
	$direction,
	$maxValues,
	$systems,
	$temperatureFilter,
	$userSettings,
} from "@/lib/stores"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import {
	chartTimeData,
	cn,
	compareSemVer,
	decimalString,
	formatBytes,
	listen,
	parseSemVer,
	toFixedFloat,
	useBrowserStorage,
} from "@/lib/utils"
import type {
	ChartData,
	ChartTimes,
	ContainerStatsRecord,
	GPUData,
	SystemDetailsRecord,
	SystemInfo,
	SystemRecord,
	SystemStats,
	SystemStatsRecord,
} from "@/types"
import { $router, navigate } from "../router"
import Spinner from "../spinner"
import { Button } from "../ui/button"
import { Card, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { ChartAverage, ChartMax } from "../ui/icons"
import { Input } from "../ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select"
import NetworkSheet from "./system/network-sheet"
import CpuCoresSheet from "./system/cpu-sheet"
import LineChartDefault from "../charts/line-chart"
import { pinnedAxisDomain } from "../ui/chart"
import InfoBar from "./system/info-bar"

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

	// const buffer = chartTime === "1m" ? 400 : 20_000
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

function dockerOrPodman(str: string, isPodman: boolean): string {
	if (isPodman) {
		return str.replace("docker", "podman").replace("Docker", "Podman")
	}
	return str
}

export default memo(function SystemDetail({ id }: { id: string }) {
	const direction = useStore($direction)
	const { t } = useLingui()
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
	const isLongerChart = !["1m", "1h"].includes(chartTime) // true if chart time is not 1m or 1h
	const userSettings = $userSettings.get()
	const chartWrapRef = useRef<HTMLDivElement>(null)
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
				e.metaKey
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
		// check if there are any GPUs with engines
		for (const id in lastGpus) {
			hasGpuData = true
			if (lastGpus[id].e !== undefined) {
				hasGpuEnginesData = true
				break
			}
		}
		// check if there are any GPUs with power data
		for (let i = 0; i < systemStats.length && !hasGpuPowerData; i++) {
			const gpus = systemStats[i].stats?.g
			if (!gpus) continue
			for (const id in gpus) {
				if (gpus[id].p !== undefined || gpus[id].pp !== undefined) {
					hasGpuPowerData = true
					break
				}
			}
		}
	}

	const isLinux = !(details?.os ?? system.info?.os)
	const isPodman = details?.podman ?? system.info?.p ?? false

	return (
		<>
			<div ref={chartWrapRef} className="grid gap-4 mb-14 overflow-x-clip">
				{/* system info */}
				<InfoBar system={system} chartData={chartData} grid={grid} setGrid={setGrid} details={details} />

				{/* <Tabs defaultValue="overview" className="w-full">
					<TabsList className="w-full h-11">
						<TabsTrigger value="overview" className="w-full h-9">Overview</TabsTrigger>
						<TabsTrigger value="containers" className="w-full h-9">Containers</TabsTrigger>
						<TabsTrigger value="smart" className="w-full h-9">S.M.A.R.T.</TabsTrigger>
					</TabsList>
					<TabsContent value="smart">
					</TabsContent>
				</Tabs> */}

				{/* main charts */}
				<div className="grid xl:grid-cols-2 gap-4">
					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`CPU Usage`}
						description={t`Average system-wide CPU utilization`}
						cornerEl={
							<div className="flex gap-2">
								{maxValSelect}
								<CpuCoresSheet chartData={chartData} dataEmpty={dataEmpty} grid={grid} maxValues={maxValues} />
							</div>
						}
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={maxValues}
							dataPoints={[
								{
									label: t`CPU Usage`,
									dataKey: ({ stats }) => (showMax ? stats?.cpum : stats?.cpu),
									color: 1,
									opacity: 0.4,
								},
							]}
							tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
							contentFormatter={({ value }) => `${decimalString(value)}%`}
							domain={pinnedAxisDomain()}
						/>
					</ChartCard>

					{containerFilterBar && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={dockerOrPodman(t`Docker CPU Usage`, isPodman)}
							description={t`Average CPU utilization of containers`}
							cornerEl={containerFilterBar}
						>
							<ContainerChart
								chartData={chartData}
								dataKey="c"
								chartType={ChartType.CPU}
								chartConfig={containerChartConfigs.cpu}
							/>
						</ChartCard>
					)}

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Memory Usage`}
						description={t`Precise utilization at the recorded time`}
						cornerEl={maxValSelect}
					>
						<MemChart chartData={chartData} showMax={showMax} />
					</ChartCard>

					{containerFilterBar && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={dockerOrPodman(t`Docker Memory Usage`, isPodman)}
							description={dockerOrPodman(t`Memory usage of docker containers`, isPodman)}
							cornerEl={containerFilterBar}
						>
							<ContainerChart
								chartData={chartData}
								dataKey="m"
								chartType={ChartType.Memory}
								chartConfig={containerChartConfigs.memory}
							/>
						</ChartCard>
					)}

					<ChartCard empty={dataEmpty} grid={grid} title={t`Disk Usage`} description={t`Usage of root partition`}>
						<DiskChart chartData={chartData} dataKey="stats.du" diskSize={systemStats.at(-1)?.stats.d ?? NaN} />
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Disk I/O`}
						description={t`Throughput of root filesystem`}
						cornerEl={maxValSelect}
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={maxValues}
							dataPoints={[
								{
									label: t({ message: "Write", comment: "Disk write" }),
									dataKey: ({ stats }: SystemStatsRecord) => {
										if (showMax) {
											return stats?.dio?.[1] ?? (stats?.dwm ?? 0) * 1024 * 1024
										}
										return stats?.dio?.[1] ?? (stats?.dw ?? 0) * 1024 * 1024
									},
									color: 3,
									opacity: 0.3,
								},
								{
									label: t({ message: "Read", comment: "Disk read" }),
									dataKey: ({ stats }: SystemStatsRecord) => {
										if (showMax) {
											return stats?.diom?.[0] ?? (stats?.drm ?? 0) * 1024 * 1024
										}
										return stats?.dio?.[0] ?? (stats?.dr ?? 0) * 1024 * 1024
									},
									color: 1,
									opacity: 0.3,
								},
							]}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, true, userSettings.unitDisk, false)
								return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
							}}
							contentFormatter={({ value }) => {
								const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, false)
								return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
							}}
							showTotal={true}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Bandwidth`}
						cornerEl={
							<div className="flex gap-2">
								{maxValSelect}
								<NetworkSheet chartData={chartData} dataEmpty={dataEmpty} grid={grid} maxValues={maxValues} />
							</div>
						}
						description={t`Network traffic of public interfaces`}
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={maxValues}
							dataPoints={[
								{
									label: t`Sent`,
									// use bytes if available, otherwise multiply old MB (can remove in future)
									dataKey(data: SystemStatsRecord) {
										if (showMax) {
											return data?.stats?.bm?.[0] ?? (data?.stats?.nsm ?? 0) * 1024 * 1024
										}
										return data?.stats?.b?.[0] ?? data?.stats?.ns * 1024 * 1024
									},
									color: 5,
									opacity: 0.2,
								},
								{
									label: t`Received`,
									dataKey(data: SystemStatsRecord) {
										if (showMax) {
											return data?.stats?.bm?.[1] ?? (data?.stats?.nrm ?? 0) * 1024 * 1024
										}
										return data?.stats?.b?.[1] ?? data?.stats?.nr * 1024 * 1024
									},
									color: 2,
									opacity: 0.2,
								},
							]
								// try to place the lesser number in front for better visibility
								.sort(() => (systemStats.at(-1)?.stats.b?.[1] ?? 0) - (systemStats.at(-1)?.stats.b?.[0] ?? 0))}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
								return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
							}}
							contentFormatter={(data) => {
								const { value, unit } = formatBytes(data.value, true, userSettings.unitNet, false)
								return `${decimalString(value, value >= 100 ? 1 : 2)} ${unit}`
							}}
							showTotal={true}
						/>
					</ChartCard>

					{containerFilterBar && containerData.length > 0 && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={dockerOrPodman(t`Docker Network I/O`, isPodman)}
							description={dockerOrPodman(t`Network traffic of docker containers`, isPodman)}
							cornerEl={containerFilterBar}
						>
							<ContainerChart
								chartData={chartData}
								chartType={ChartType.Network}
								dataKey="n"
								chartConfig={containerChartConfigs.network}
							/>
						</ChartCard>
					)}

					{/* Swap chart */}
					{(systemStats.at(-1)?.stats.su ?? 0) > 0 && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={t`Swap Usage`}
							description={t`Swap space used by the system`}
						>
							<SwapChart chartData={chartData} />
						</ChartCard>
					)}

					{/* Load Average chart */}
					{chartData.agentVersion?.minor >= 12 && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={t`Load Average`}
							description={t`System load averages over time`}
							legend={true}
						>
							<LoadAverageChart chartData={chartData} />
						</ChartCard>
					)}

					{/* Temperature chart */}
					{systemStats.at(-1)?.stats.t && (
						<div ref={temperatureChartRef} className={cn("odd:last-of-type:col-span-full", { "col-span-full": !grid })}>
							<ChartCard
								empty={dataEmpty}
								grid={grid}
								title={t`Temperature`}
								description={t`Temperatures of system sensors`}
								cornerEl={<FilterBar store={$temperatureFilter} />}
								legend={Object.keys(systemStats.at(-1)?.stats.t ?? {}).length < 12}
							>
								<TemperatureChart chartData={chartData} />
							</ChartCard>
						</div>
					)}

					{/* Battery chart */}
					{systemStats.at(-1)?.stats.bat && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={t`Battery`}
							description={`${t({
								message: "Current state",
								comment: "Context: Battery state",
							})}: ${batteryStateTranslations[systemStats.at(-1)?.stats.bat?.[1] ?? 0]()}`}
						>
							<AreaChartDefault
								chartData={chartData}
								maxToggled={maxValues}
								dataPoints={[
									{
										label: t`Charge`,
										dataKey: ({ stats }) => stats?.bat?.[0],
										color: 1,
										opacity: 0.35,
									},
								]}
								domain={[0, 100]}
								tickFormatter={(val) => `${val}%`}
								contentFormatter={({ value }) => `${value}%`}
							/>
						</ChartCard>
					)}
					{/* GPU power draw chart */}
					{hasGpuPowerData && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={t`GPU Power Draw`}
							description={t`Average power consumption of GPUs`}
						>
							<GpuPowerChart chartData={chartData} />
						</ChartCard>
					)}
				</div>

				{/* Non-power GPU charts */}
				{hasGpuData && (
					<div className="grid xl:grid-cols-2 gap-4">
						{hasGpuEnginesData && (
							<ChartCard
								legend={true}
								empty={dataEmpty}
								grid={grid}
								title={t`GPU Engines`}
								description={t`Average utilization of GPU engines`}
							>
								<GpuEnginesChart chartData={chartData} />
							</ChartCard>
						)}
						{lastGpus &&
							Object.keys(lastGpus).map((id) => {
								const gpu = lastGpus[id] as GPUData
								return (
									<div key={id} className="contents">
										<ChartCard
											className={cn(grid && "!col-span-1")}
											empty={dataEmpty}
											grid={grid}
											title={`${gpu.n} ${t`Usage`}`}
											description={t`Average utilization of ${gpu.n}`}
										>
											<AreaChartDefault
												chartData={chartData}
												dataPoints={[
													{
														label: t`Usage`,
														dataKey: ({ stats }) => stats?.g?.[id]?.u ?? 0,
														color: 1,
														opacity: 0.35,
													},
												]}
												tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
												contentFormatter={({ value }) => `${decimalString(value)}%`}
											/>
										</ChartCard>

										{(gpu.mt ?? 0) > 0 && (
											<ChartCard
												empty={dataEmpty}
												grid={grid}
												title={`${gpu.n} VRAM`}
												description={t`Precise utilization at the recorded time`}
											>
												<AreaChartDefault
													chartData={chartData}
													dataPoints={[
														{
															label: t`Usage`,
															dataKey: ({ stats }) => stats?.g?.[id]?.mu ?? 0,
															color: 2,
															opacity: 0.25,
														},
													]}
													max={gpu.mt}
													tickFormatter={(val) => {
														const { value, unit } = formatBytes(val, false, Unit.Bytes, true)
														return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
													}}
													contentFormatter={({ value }) => {
														const { value: convertedValue, unit } = formatBytes(value, false, Unit.Bytes, true)
														return `${decimalString(convertedValue)} ${unit}`
													}}
												/>
											</ChartCard>
										)}
									</div>
								)
							})}
					</div>
				)}

				{/* extra filesystem charts */}
				{Object.keys(systemStats.at(-1)?.stats.efs ?? {}).length > 0 && (
					<div className="grid xl:grid-cols-2 gap-4">
						{Object.keys(systemStats.at(-1)?.stats.efs ?? {}).map((extraFsName) => {
							return (
								<div key={extraFsName} className="contents">
									<ChartCard
										empty={dataEmpty}
										grid={grid}
										title={`${extraFsName} ${t`Usage`}`}
										description={t`Disk usage of ${extraFsName}`}
									>
										<DiskChart
											chartData={chartData}
											dataKey={({ stats }: SystemStatsRecord) => stats?.efs?.[extraFsName]?.du}
											diskSize={systemStats.at(-1)?.stats.efs?.[extraFsName].d ?? NaN}
										/>
									</ChartCard>
									<ChartCard
										empty={dataEmpty}
										grid={grid}
										title={`${extraFsName} I/O`}
										description={t`Throughput of ${extraFsName}`}
										cornerEl={maxValSelect}
									>
										<AreaChartDefault
											chartData={chartData}
											dataPoints={[
												{
													label: t`Write`,
													dataKey: ({ stats }) => {
														if (showMax) {
															return (
																stats?.efs?.[extraFsName]?.wbm || (stats?.efs?.[extraFsName]?.wm ?? 0) * 1024 * 1024
															)
														}
														return stats?.efs?.[extraFsName]?.wb || (stats?.efs?.[extraFsName]?.w ?? 0) * 1024 * 1024
													},
													color: 3,
													opacity: 0.3,
												},
												{
													label: t`Read`,
													dataKey: ({ stats }) => {
														if (showMax) {
															return (
																stats?.efs?.[extraFsName]?.rbm ?? (stats?.efs?.[extraFsName]?.rm ?? 0) * 1024 * 1024
															)
														}
														return stats?.efs?.[extraFsName]?.rb ?? (stats?.efs?.[extraFsName]?.r ?? 0) * 1024 * 1024
													},
													color: 1,
													opacity: 0.3,
												},
											]}
											maxToggled={maxValues}
											tickFormatter={(val) => {
												const { value, unit } = formatBytes(val, true, userSettings.unitDisk, false)
												return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
											}}
											contentFormatter={({ value }) => {
												const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, false)
												return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
											}}
										/>
									</ChartCard>
								</div>
							)
						})}
					</div>
				)}

				{compareSemVer(chartData.agentVersion, parseSemVer("0.15.0")) >= 0 && <LazySmartTable systemId={system.id} />}

				{containerData.length > 0 && compareSemVer(chartData.agentVersion, parseSemVer("0.14.0")) >= 0 && (
					<LazyContainersTable systemId={system.id} />
				)}

				{isLinux && compareSemVer(chartData.agentVersion, parseSemVer("0.16.0")) >= 0 && (
					<LazySystemdTable systemId={system.id} />
				)}
			</div>

			{/* add space for tooltip if lots of sensors */}
			{bottomSpacing > 0 && <span className="block" style={{ height: bottomSpacing }} />}
		</>
	)
})

function GpuEnginesChart({ chartData }: { chartData: ChartData }) {
	const dataPoints: DataPoint[] = []
	const engines = Object.keys(chartData.systemStats?.at(-1)?.stats.g?.[0]?.e ?? {}).sort()
	for (const engine of engines) {
		dataPoints.push({
			label: engine,
			dataKey: ({ stats }: SystemStatsRecord) => stats?.g?.[0]?.e?.[engine] ?? 0,
			color: `hsl(${140 + (((engines.indexOf(engine) * 360) / engines.length) % 360)}, 65%, 52%)`,
			opacity: 0.35,
		})
	}
	return (
		<LineChartDefault
			legend={true}
			chartData={chartData}
			dataPoints={dataPoints}
			tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
			contentFormatter={({ value }) => `${decimalString(value)}%`}
		/>
	)
}

function FilterBar({ store = $containerFilter }: { store?: typeof $containerFilter }) {
	const storeValue = useStore(store)
	const [inputValue, setInputValue] = useState(storeValue)
	const { t } = useLingui()

	useEffect(() => {
		setInputValue(storeValue)
	}, [storeValue])

	useEffect(() => {
		if (inputValue === storeValue) {
			return
		}
		const handle = window.setTimeout(() => store.set(inputValue), 80)
		return () => clearTimeout(handle)
	}, [inputValue, storeValue, store])

	const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
		const value = e.target.value
		setInputValue(value)
	}, [])

	const handleClear = useCallback(() => {
		setInputValue("")
		store.set("")
	}, [store])

	return (
		<>
			<Input
				placeholder={t`Filter...`}
				className="ps-4 pe-8 w-full sm:w-44"
				onChange={handleChange}
				value={inputValue}
			/>
			{inputValue && (
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label="Clear"
					className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
					onClick={handleClear}
				>
					<XIcon className="h-4 w-4" />
				</Button>
			)}
		</>
	)
}

const SelectAvgMax = memo(({ max }: { max: boolean }) => {
	const Icon = max ? ChartMax : ChartAverage
	return (
		<Select value={max ? "max" : "avg"} onValueChange={(e) => $maxValues.set(e === "max")}>
			<SelectTrigger className="relative ps-10 pe-5 w-full sm:w-44">
				<Icon className="h-4 w-4 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				<SelectItem key="avg" value="avg">
					<Trans>Average</Trans>
				</SelectItem>
				<SelectItem key="max" value="max">
					<Trans comment="Chart select field. Please try to keep this short.">Max 1 min</Trans>
				</SelectItem>
			</SelectContent>
		</Select>
	)
})

export function ChartCard({
	title,
	description,
	children,
	grid,
	empty,
	cornerEl,
	legend,
	className,
}: {
	title: string
	description: string
	children: React.ReactNode
	grid?: boolean
	empty?: boolean
	cornerEl?: JSX.Element | null
	legend?: boolean
	className?: string
}) {
	const { isIntersecting, ref } = useIntersectionObserver()

	return (
		<Card
			className={cn("pb-2 sm:pb-4 odd:last-of-type:col-span-full min-h-full", { "col-span-full": !grid }, className)}
			ref={ref}
		>
			<CardHeader className="pb-5 pt-4 gap-1 relative max-sm:py-3 max-sm:px-4">
				<CardTitle className="text-xl sm:text-2xl">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
				{cornerEl && <div className="py-1 grid sm:justify-end sm:absolute sm:top-3.5 sm:end-3.5">{cornerEl}</div>}
			</CardHeader>
			<div className={cn("ps-0 w-[calc(100%-1.3em)] relative group", legend ? "h-54 md:h-56" : "h-48 md:h-52")}>
				{
					<Spinner
						msg={empty ? t`Waiting for enough records to display` : undefined}
						// className="group-has-[.opacity-100]:opacity-0 transition-opacity"
						className="group-has-[.opacity-100]:invisible duration-100"
					/>
				}
				{isIntersecting && children}
			</div>
		</Card>
	)
}

const ContainersTable = lazy(() => import("../containers-table/containers-table"))

function LazyContainersTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <ContainersTable systemId={systemId} />}
		</div>
	)
}

const SmartTable = lazy(() => import("./system/smart-table"))

function LazySmartTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SmartTable systemId={systemId} />}
		</div>
	)
}

const SystemdTable = lazy(() => import("../systemd-table/systemd-table"))

function LazySystemdTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver()
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SystemdTable systemId={systemId} />}
		</div>
	)
}
