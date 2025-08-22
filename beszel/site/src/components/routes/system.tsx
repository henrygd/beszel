import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import {
	$systems,
	pb,
	$chartTime,
	$containerFilter,
	$userSettings,
	$direction,
	$maxValues,
	$temperatureFilter,
} from "@/lib/stores"
import { ChartData, ChartTimes, ContainerStatsRecord, GPUData, SystemRecord, SystemStatsRecord } from "@/types"
import { ChartType, Unit, Os } from "@/lib/enums"
import React, { lazy, memo, useCallback, useEffect, useMemo, useRef, useState, type JSX } from "react";
import { Card, CardHeader, CardTitle, CardDescription } from "../ui/card"
import { useStore } from "@nanostores/react"
import Spinner from "../spinner"
import { ClockArrowUp, CpuIcon, GlobeIcon, LayoutGridIcon, MonitorIcon, XIcon } from "lucide-react"
import ChartTimeSelect from "../charts/chart-time-select"
import {
	chartTimeData,
	cn,
	decimalString,
	formatBytes,
	getHostDisplayValue,
	getPbTimestamp,
	listen,
	parseSemVer,
	toFixedFloat,
	useLocalStorage,
} from "@/lib/utils"
import { Separator } from "../ui/separator"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "../ui/tooltip"
import { Button } from "../ui/button"
import { Input } from "../ui/input"
import { ChartAverage, ChartMax, Rows, TuxIcon, WindowsIcon, AppleIcon, FreeBsdIcon } from "../ui/icons"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select"
import { timeTicks } from "d3-time"
import { useLingui } from "@lingui/react/macro"
import { $router, navigate } from "../router"
import { getPagePath } from "@nanostores/router"

const AreaChartDefault = lazy(() => import("../charts/area-chart"))
const ContainerChart = lazy(() => import("../charts/container-chart"))
const MemChart = lazy(() => import("../charts/mem-chart"))
const DiskChart = lazy(() => import("../charts/disk-chart"))
const SwapChart = lazy(() => import("../charts/swap-chart"))
const TemperatureChart = lazy(() => import("../charts/temperature-chart"))
const GpuPowerChart = lazy(() => import("../charts/gpu-power-chart"))
const LoadAverageChart = lazy(() => import("../charts/load-average-chart"))

const cache = new Map<string, any>()

// create ticks and domain for charts
function getTimeData(chartTime: ChartTimes, lastCreated: number) {
	const cached = cache.get("td")
	if (cached && cached.chartTime === chartTime) {
		if (!lastCreated || cached.time >= lastCreated) {
			return cached.data
		}
	}

	const now = new Date()
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
function addEmptyValues<T extends SystemStatsRecord | ContainerStatsRecord>(
	prevRecords: T[],
	newRecords: T[],
	expectedInterval: number
) {
	const modifiedRecords: T[] = []
	let prevTime = (prevRecords.at(-1)?.created ?? 0) as number
	for (let i = 0; i < newRecords.length; i++) {
		const record = newRecords[i]
		record.created = new Date(record.created).getTime()
		if (prevTime) {
			const interval = record.created - prevTime
			// if interval is too large, add a null record
			if (interval > expectedInterval / 2 + expectedInterval) {
				// @ts-ignore
				modifiedRecords.push({ created: null, stats: null })
			}
		}
		prevTime = record.created
		modifiedRecords.push(record)
	}
	return modifiedRecords
}

async function getStats<T>(collection: string, system: SystemRecord, chartTime: ChartTimes): Promise<T[]> {
	const lastCached = cache.get(`${system.id}_${chartTime}_${collection}`)?.at(-1)?.created as number
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

function dockerOrPodman(str: string, system: SystemRecord) {
	if (system.info.p) {
		str = str.replace("docker", "podman").replace("Docker", "Podman")
	}
	return str
}

export default function SystemDetail({ name }: { name: string }) {
	const direction = useStore($direction)
	const { t } = useLingui()
	const systems = useStore($systems)
	const chartTime = useStore($chartTime)
	const maxValues = useStore($maxValues)
	const [grid, setGrid] = useLocalStorage("grid", true)
	const [system, setSystem] = useState({} as SystemRecord)
	const [systemStats, setSystemStats] = useState([] as SystemStatsRecord[])
	const [containerData, setContainerData] = useState([] as ChartData["containerData"])
	const netCardRef = useRef<HTMLDivElement>(null)
	const persistChartTime = useRef(false)
	const [containerFilterBar, setContainerFilterBar] = useState(null as null | JSX.Element)
	const [bottomSpacing, setBottomSpacing] = useState(0)
	const [chartLoading, setChartLoading] = useState(true)
	const isLongerChart = chartTime !== "1h"
	const userSettings = $userSettings.get()

	useEffect(() => {
		document.title = `${name} / Beszel`
		return () => {
			if (!persistChartTime.current) {
				$chartTime.set($userSettings.get().chartTime)
			}
			persistChartTime.current = false
			setSystemStats([])
			setContainerData([])
			setContainerFilterBar(null)
			$containerFilter.set("")
		}
	}, [name])

	// function resetCharts() {
	// 	setSystemStats([])
	// 	setContainerData([])
	// }

	// useEffect(resetCharts, [chartTime])

	// find matching system
	useEffect(() => {
		if (system.id && system.name === name) {
			return
		}
		const matchingSystem = systems.find((s) => s.name === name) as SystemRecord
		if (matchingSystem) {
			setSystem(matchingSystem)
		}
	}, [name, system, systems])

	// update system when new data is available
	useEffect(() => {
		if (!system.id) {
			return
		}
		pb.collection<SystemRecord>("systems").subscribe(system.id, (e) => {
			setSystem(e.record)
		})
		return () => {
			pb.collection("systems").unsubscribe(system.id)
		}
	}, [system.id])

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

	// get stats
	useEffect(() => {
		if (!system.id || !chartTime) {
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
			if (containerData.length) {
				!containerFilterBar && setContainerFilterBar(<FilterBar />)
			} else if (containerFilterBar) {
				setContainerFilterBar(null)
			}
			makeContainerData(containerData)
		})
	}, [system, chartTime])

	// make container stats for charts
	const makeContainerData = useCallback((containers: ContainerStatsRecord[]) => {
		const containerData = [] as ChartData["containerData"]
		for (let { created, stats } of containers) {
			if (!created) {
				// @ts-ignore add null value for gaps
				containerData.push({ created: null })
				continue
			}
			created = new Date(created).getTime()
			// @ts-ignore not dealing with this rn
			let containerStats: ChartData["containerData"][0] = { created }
			for (let container of stats) {
				containerStats[container.n] = container
			}
			containerData.push(containerStats)
		}
		setContainerData(containerData)
	}, [])

	// values for system info bar
	const systemInfo = useMemo(() => {
		if (!system.info) {
			return []
		}

		const osInfo = {
			[Os.Linux]: {
				Icon: TuxIcon,
				value: system.info.k,
				label: t({ comment: "Linux kernel", message: "Kernel" }),
			},
			[Os.Darwin]: {
				Icon: AppleIcon,
				value: `macOS ${system.info.k}`,
			},
			[Os.Windows]: {
				Icon: WindowsIcon,
				value: system.info.k,
			},
			[Os.FreeBSD]: {
				Icon: FreeBsdIcon,
				value: system.info.k,
			},
		}

		let uptime: React.ReactNode
		if (system.info.u < 172800) {
			const hours = Math.trunc(system.info.u / 3600)
			uptime = <Plural value={hours} one="# hour" other="# hours" />
		} else {
			uptime = <Plural value={Math.trunc(system.info?.u / 86400)} one="# day" other="# days" />
		}
		return [
			{ value: getHostDisplayValue(system), Icon: GlobeIcon },
			{
				value: system.info.h,
				Icon: MonitorIcon,
				label: "Hostname",
				// hide if hostname is same as host or name
				hide: system.info.h === system.host || system.info.h === system.name,
			},
			{ value: uptime, Icon: ClockArrowUp, label: t`Uptime`, hide: !system.info.u },
			osInfo[system.info.os ?? Os.Linux],
			{
				value: `${system.info.m} (${system.info.c}c${system.info.t ? `/${system.info.t}t` : ""})`,
				Icon: CpuIcon,
				hide: !system.info.m,
			},
		] as {
			value: string | number | undefined
			label?: string
			Icon: any
			hide?: boolean
		}[]
	}, [system.info])

	/** Space for tooltip if more than 12 containers */
	useEffect(() => {
		if (!netCardRef.current || !containerData.length) {
			setBottomSpacing(0)
			return
		}
		const tooltipHeight = (Object.keys(containerData[0]).length - 11) * 17.8 - 40
		const wrapperEl = document.getElementById("chartwrap") as HTMLDivElement
		const wrapperRect = wrapperEl.getBoundingClientRect()
		const chartRect = netCardRef.current.getBoundingClientRect()
		const distanceToBottom = wrapperRect.bottom - chartRect.bottom
		setBottomSpacing(tooltipHeight - distanceToBottom)
	}, [netCardRef, containerData])

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
			const currentIndex = systems.findIndex((s) => s.name === name)
			if (currentIndex === -1 || systems.length <= 1) {
				return
			}
			switch (e.key) {
				case "ArrowLeft":
				case "h":
					const prevIndex = (currentIndex - 1 + systems.length) % systems.length
					persistChartTime.current = true
					return navigate(getPagePath($router, "system", { name: systems[prevIndex].name }))
				case "ArrowRight":
				case "l":
					const nextIndex = (currentIndex + 1) % systems.length
					persistChartTime.current = true
					return navigate(getPagePath($router, "system", { name: systems[nextIndex].name }))
			}
		}
		return listen(document, "keyup", handleKeyUp)
	}, [name, systems])

	if (!system.id) {
		return null
	}

	// select field for switching between avg and max values
	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null
	const showMax = chartTime !== "1h" && maxValues

	// if no data, show empty message
	const dataEmpty = !chartLoading && chartData.systemStats.length === 0
	const lastGpuVals = Object.values(systemStats.at(-1)?.stats.g ?? {})
	const hasGpuData = lastGpuVals.length > 0
	const hasGpuPowerData = lastGpuVals.some((gpu) => gpu.p !== undefined)

	let translatedStatus: string = system.status
	if (system.status === "up") {
		translatedStatus = t({ message: "Up", comment: "Context: System is up" })
	} else if (system.status === "down") {
		translatedStatus = t({ message: "Down", comment: "Context: System is down" })
	}

	return (
		<>
			<div id="chartwrap" className="grid gap-4 mb-10 overflow-x-clip">
				{/* system info */}
				<Card>
					<div className="grid xl:flex gap-4 px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
						<div>
							<h1 className="text-[1.6rem] font-semibold mb-1.5">{system.name}</h1>
							<div className="flex flex-wrap items-center gap-3 gap-y-2 text-sm opacity-90">
								<div className="capitalize flex gap-2 items-center">
									<span className={cn("relative flex h-3 w-3")}>
										{system.status === "up" && (
											<span
												className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"
												style={{ animationDuration: "1.5s" }}
											></span>
										)}
										<span
											className={cn("relative inline-flex rounded-full h-3 w-3", {
												"bg-green-500": system.status === "up",
												"bg-red-500": system.status === "down",
												"bg-primary/40": system.status === "paused",
												"bg-yellow-500": system.status === "pending",
											})}
										></span>
									</span>
									{translatedStatus}
								</div>
								{systemInfo.map(({ value, label, Icon, hide }, i) => {
									if (hide || !value) {
										return null
									}
									const content = (
										<div className="flex gap-1.5 items-center">
											<Icon className="h-4 w-4" /> {value}
										</div>
									)
									return (
										<div key={i} className="contents">
											<Separator orientation="vertical" className="h-4 bg-primary/30" />
											{label ? (
												<TooltipProvider>
													<Tooltip delayDuration={150}>
														<TooltipTrigger asChild>{content}</TooltipTrigger>
														<TooltipContent>{label}</TooltipContent>
													</Tooltip>
												</TooltipProvider>
											) : (
												content
											)}
										</div>
									)
								})}
							</div>
						</div>
						<div className="xl:ms-auto flex items-center gap-2 max-sm:-mb-1">
							<ChartTimeSelect className="w-full xl:w-40" />
							<TooltipProvider delayDuration={100}>
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											aria-label={t`Toggle grid`}
											variant="outline"
											size="icon"
											className="hidden xl:flex p-0 text-primary"
											onClick={() => setGrid(!grid)}
										>
											{grid ? (
												<LayoutGridIcon className="h-[1.2rem] w-[1.2rem] opacity-75" />
											) : (
												<Rows className="h-[1.3rem] w-[1.3rem] opacity-75" />
											)}
										</Button>
									</TooltipTrigger>
									<TooltipContent>{t`Toggle grid`}</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						</div>
					</div>
				</Card>

				{/* main charts */}
				<div className="grid xl:grid-cols-2 gap-4">
					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`CPU Usage`}
						description={t`Average system-wide CPU utilization`}
						cornerEl={maxValSelect}
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={maxValues}
							dataPoints={[
								{
									label: t`CPU Usage`,
									dataKey: ({ stats }) => (showMax ? stats?.cpum : stats?.cpu),
									color: "1",
									opacity: 0.4,
								},
							]}
							tickFormatter={(val) => toFixedFloat(val, 2) + "%"}
							contentFormatter={({ value }) => decimalString(value) + "%"}
						/>
					</ChartCard>

					{containerFilterBar && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={dockerOrPodman(t`Docker CPU Usage`, system)}
							description={t`Average CPU utilization of containers`}
							cornerEl={containerFilterBar}
						>
							<ContainerChart chartData={chartData} dataKey="c" chartType={ChartType.CPU} />
						</ChartCard>
					)}

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Memory Usage`}
						description={t`Precise utilization at the recorded time`}
					>
						<MemChart chartData={chartData} />
					</ChartCard>

					{containerFilterBar && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={dockerOrPodman(t`Docker Memory Usage`, system)}
							description={dockerOrPodman(t`Memory usage of docker containers`, system)}
							cornerEl={containerFilterBar}
						>
							<ContainerChart chartData={chartData} dataKey="m" chartType={ChartType.Memory} />
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
									dataKey: ({ stats }) => (showMax ? stats?.dwm : stats?.dw),
									color: "3",
									opacity: 0.3,
								},
								{
									label: t({ message: "Read", comment: "Disk read" }),
									dataKey: ({ stats }) => (showMax ? stats?.drm : stats?.dr),
									color: "1",
									opacity: 0.3,
								},
							]}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, true, userSettings.unitDisk, true)
								return toFixedFloat(value, value >= 10 ? 0 : 1) + " " + unit
							}}
							contentFormatter={({ value }) => {
								const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, true)
								return decimalString(convertedValue, convertedValue >= 100 ? 1 : 2) + " " + unit
							}}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Bandwidth`}
						cornerEl={maxValSelect}
						description={t`Network traffic of public interfaces`}
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={maxValues}
							dataPoints={[
								{
									label: t`Sent`,
									// use bytes if available, otherwise multiply old MB (can remove in future)
									dataKey(data) {
										if (showMax) {
											return data?.stats?.bm?.[0] ?? (data?.stats?.nsm ?? 0) * 1024 * 1024
										}
										return data?.stats?.b?.[0] ?? data?.stats?.ns * 1024 * 1024
									},
									color: "5",
									opacity: 0.2,
								},
								{
									label: t`Received`,
									dataKey(data) {
										if (showMax) {
											return data?.stats?.bm?.[1] ?? (data?.stats?.nrm ?? 0) * 1024 * 1024
										}
										return data?.stats?.b?.[1] ?? data?.stats?.nr * 1024 * 1024
									},
									color: "2",
									opacity: 0.2,
								},
							]}
							tickFormatter={(val) => {
								let { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
								return toFixedFloat(value, value >= 10 ? 0 : 1) + " " + unit
							}}
							contentFormatter={(data) => {
								const { value, unit } = formatBytes(data.value, true, userSettings.unitNet, false)
								return decimalString(value, value >= 100 ? 1 : 2) + " " + unit
							}}
						/>
					</ChartCard>

					{containerFilterBar && containerData.length > 0 && (
						<div
							ref={netCardRef}
							className={cn({
								"col-span-full": !grid,
							})}
						>
							<ChartCard
								empty={dataEmpty}
								title={dockerOrPodman(t`Docker Network I/O`, system)}
								description={dockerOrPodman(t`Network traffic of docker containers`, system)}
								cornerEl={containerFilterBar}
							>
								{/* @ts-ignore */}
								<ContainerChart chartData={chartData} chartType={ChartType.Network} dataKey="n" />
							</ChartCard>
						</div>
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
						>
							<LoadAverageChart chartData={chartData} />
						</ChartCard>
					)}

					{/* Temperature chart */}
					{systemStats.at(-1)?.stats.t && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={t`Temperature`}
							description={t`Temperatures of system sensors`}
							cornerEl={<FilterBar store={$temperatureFilter} />}
						>
							<TemperatureChart chartData={chartData} />
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

				{/* GPU charts */}
				{hasGpuData && (
					<div className="grid xl:grid-cols-2 gap-4">
						{Object.keys(systemStats.at(-1)?.stats.g ?? {}).map((id) => {
							const gpu = systemStats.at(-1)?.stats.g?.[id] as GPUData
							return (
								<div key={id} className="contents">
									<ChartCard
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
													color: "1",
													opacity: 0.35,
												},
											]}
											tickFormatter={(val) => toFixedFloat(val, 2) + "%"}
											contentFormatter={({ value }) => decimalString(value) + "%"}
										/>
									</ChartCard>
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
													color: "2",
													opacity: 0.25,
												},
											]}
											max={gpu.mt}
											tickFormatter={(val) => {
												const { value, unit } = formatBytes(val, false, Unit.Bytes, true)
												return toFixedFloat(value, value >= 10 ? 0 : 1) + " " + unit
											}}
											contentFormatter={({ value }) => {
												const { value: convertedValue, unit } = formatBytes(value, false, Unit.Bytes, true)
												return decimalString(convertedValue) + " " + unit
											}}
										/>
									</ChartCard>
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
											dataKey={`stats.efs.${extraFsName}.du`}
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
													dataKey: ({ stats }) => stats?.efs?.[extraFsName]?.[showMax ? "wm" : "w"] ?? 0,
													color: "3",
													opacity: 0.3,
												},
												{
													label: t`Read`,
													dataKey: ({ stats }) => stats?.efs?.[extraFsName]?.[showMax ? "rm" : "r"] ?? 0,
													color: "1",
													opacity: 0.3,
												},
											]}
											maxToggled={maxValues}
											tickFormatter={(val) => {
												const { value, unit } = formatBytes(val, true, userSettings.unitDisk, true)
												return toFixedFloat(value, value >= 10 ? 0 : 1) + " " + unit
											}}
											contentFormatter={({ value }) => {
												const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, true)
												return decimalString(convertedValue, convertedValue >= 100 ? 1 : 2) + " " + unit
											}}
										/>
									</ChartCard>
								</div>
							)
						})}
					</div>
				)}
			</div>

			{/* add space for tooltip if more than 12 containers */}
			{bottomSpacing > 0 && <span className="block" style={{ height: bottomSpacing }} />}
		</>
	)
}

function FilterBar({ store = $containerFilter }: { store?: typeof $containerFilter }) {
	const containerFilter = useStore(store)
	const { t } = useLingui()

	const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
		store.set(e.target.value)
	}, [])

	return (
		<>
			<Input placeholder={t`Filter...`} className="ps-4 pe-8" value={containerFilter} onChange={handleChange} />
			{containerFilter && (
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label="Clear"
					className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
					onClick={() => store.set("")}
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
			<SelectTrigger className="relative ps-10 pe-5">
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

function ChartCard({
	title,
	description,
	children,
	grid,
	empty,
	cornerEl,
}: {
	title: string
	description: string
	children: React.ReactNode
	grid?: boolean
	empty?: boolean
	cornerEl?: JSX.Element | null
}) {
	const { isIntersecting, ref } = useIntersectionObserver()

	return (
		<Card className={cn("pb-2 sm:pb-4 odd:last-of-type:col-span-full", { "col-span-full": !grid })} ref={ref}>
			<CardHeader className="pb-5 pt-4 relative space-y-1 max-sm:py-3 max-sm:px-4">
				<CardTitle className="text-xl sm:text-2xl">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
				{cornerEl && <div className="relative py-1 block sm:w-44 sm:absolute sm:top-2.5 sm:end-3.5">{cornerEl}</div>}
			</CardHeader>
			<div className="ps-0 w-[calc(100%-1.5em)] h-48 md:h-52 relative group">
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
