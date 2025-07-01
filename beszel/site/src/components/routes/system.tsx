import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import {
	$systems,
	pb,
	$chartTime,
	$containerFilter,
	$stackFilter,
	$containerColors,
	$userSettings,
	$direction,
	$maxValues,
	$temperatureFilter,
} from "@/lib/stores"
import { ChartData, ChartTimes, ContainerStatsRecord, GPUData, SystemRecord, SystemStatsRecord } from "@/types"
import { ChartType, Os } from "@/lib/enums"
import React, { lazy, memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "../ui/card"
import { useStore } from "@nanostores/react"
import Spinner from "../spinner"
import { ClockArrowUp, CpuIcon, GlobeIcon, LayoutGridIcon, MonitorIcon, XIcon, ServerIcon, ContainerIcon } from "lucide-react"
import ChartTimeSelect from "../charts/chart-time-select"
import {
	chartTimeData,
	cn,
	getHostDisplayValue,
	getPbTimestamp,
	getSizeAndUnit,
	listen,
	toFixedFloat,
	useLocalStorage,
	generateStackBasedColors,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../ui/tabs"

const AreaChartDefault = lazy(() => import("../charts/area-chart"))
const ContainerChart = lazy(() => import("../charts/container-chart"))
const MemChart = lazy(() => import("../charts/mem-chart"))
const DiskChart = lazy(() => import("../charts/disk-chart"))
const SwapChart = lazy(() => import("../charts/swap-chart"))
const TemperatureChart = lazy(() => import("../charts/temperature-chart"))
const GpuPowerChart = lazy(() => import("../charts/gpu-power-chart"))

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

function MultiSelectDropdown({ 
	options, 
	selectedValues, 
	onChange, 
	placeholder,
	filtered = false
}: { 
	options: Array<{ value: string; label: string; color?: string }>
	selectedValues: string[]
	onChange: (value: string) => void
	placeholder: string
	filtered?: boolean
}) {
	const [isOpen, setIsOpen] = useState(false)
	const [searchTerm, setSearchTerm] = useState("")
	const { t } = useLingui()
	const dropdownRef = useRef<HTMLDivElement>(null)
	const searchInputRef = useRef<HTMLInputElement>(null)

	// Close dropdown when clicking outside
	useEffect(() => {
		function handleClickOutside(event: MouseEvent) {
			if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
				setIsOpen(false)
			}
		}

		if (isOpen) {
			document.addEventListener('mousedown', handleClickOutside)
		}

		return () => {
			document.removeEventListener('mousedown', handleClickOutside)
		}
	}, [isOpen])

	// Filter options based on search term
	const filteredOptions = useMemo(() => {
		if (!searchTerm) return options
		return options.filter(option => 
			option.label.toLowerCase().includes(searchTerm.toLowerCase())
		)
	}, [options, searchTerm])

	// Focus search input when dropdown opens
	useEffect(() => {
		if (isOpen && searchInputRef.current) {
			setTimeout(() => searchInputRef.current?.focus(), 0)
		} else if (!isOpen) {
			setSearchTerm("")
		}
	}, [isOpen])

	const displayText = selectedValues.includes("all") 
		? t`All Selected`
		: selectedValues.length === 0 
		? (filtered ? `${placeholder} (filtered)` : placeholder)
		: selectedValues.length === 1
		? options.find(opt => opt.value === selectedValues[0])?.label || selectedValues[0]
		: `${selectedValues.length} selected`

	return (
		<div className="relative" ref={dropdownRef}>
			<button
				type="button"
				onClick={() => setIsOpen(!isOpen)}
				className="flex h-10 w-full items-center justify-between rounded-md border bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1"
			>
				<span className="truncate">{displayText}</span>
				<svg
					className={`ml-2 h-4 w-4 transition-transform ${isOpen ? 'rotate-180' : ''}`}
					fill="none"
					stroke="currentColor"
					viewBox="0 0 24 24"
				>
					<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
				</svg>
			</button>
			
			{isOpen && (
				<div className="absolute z-50 w-full mt-1 bg-popover border border-input rounded-md shadow-lg max-h-48 overflow-hidden">
					{/* Search input */}
					<div className="p-2 border-b border-border">
						<input
							ref={searchInputRef}
							type="text"
							placeholder={t`Search...`}
							value={searchTerm}
							onChange={(e) => setSearchTerm(e.target.value)}
							className="w-full px-2 py-1 text-sm bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-1"
							onKeyDown={(e) => {
								if (e.key === 'Escape') {
									setIsOpen(false)
								}
							}}
						/>
					</div>
					
					{/* Options list */}
					<div className="max-h-36 overflow-auto">
						{filteredOptions.length === 0 ? (
							<div className="px-3 py-2 text-sm text-muted-foreground">
								{t`No options found`}
							</div>
						) : (
							filteredOptions.map((option) => (
						<div
							key={option.value}
							onClick={() => {
								onChange(option.value)
								if (option.value === "all") {
									setIsOpen(false)
								}
							}}
							className="relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 ps-8 pe-2 text-sm outline-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
						>
							<span className="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
								{selectedValues.includes(option.value) && (
									<svg
										className="h-4 w-4 text-primary"
										fill="currentColor"
										viewBox="0 0 20 20"
									>
										<path
											fillRule="evenodd"
											d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
											clipRule="evenodd"
										/>
									</svg>
								)}
							</span>
							<div className="flex items-center gap-2">
								{option.color && (
									<div
										className="h-3 w-3 rounded-sm shrink-0"
										style={{ backgroundColor: option.color }}
									/>
								)}
								<span className="truncate">{option.label}</span>
							</div>
						</div>
					)))}
					</div>
				</div>
			)}
		</div>
	)
}

function ContainerFilterBar({ containerData, containerColors, isVolumeChart = false }: { containerData: ChartData["containerData"], containerColors: Record<string, string>, isVolumeChart?: boolean }) {
	const { t } = useLingui()
	const containerFilter = useStore($containerFilter)
	const stackFilter = useStore($stackFilter)

	// Get all container and stack data for filtering
	const containerStackData = useMemo(() => {
		const containerMap = new Map<string, { name: string; stack: string; color: string }>()
		const stackSet = new Set<string>()
		
		for (let data of containerData) {
			for (let key in data) {
				if (key && key !== "created") {
					// Exclude orphaned volumes for non-volume charts
					if (!isVolumeChart && key.startsWith("(orphaned volume)")) continue
					
					const container = data[key] as any
					const stackName = container && container.p ? container.p : "—"
					const color = containerColors[key] || `hsl(${Math.random() * 360}, 60%, 55%)`
					
					containerMap.set(key, {
						name: key,
						stack: stackName,
						color: color
					})
					stackSet.add(stackName)
				}
			}
		}
		
		return {
			containers: Array.from(containerMap.values()).sort((a, b) => a.name.localeCompare(b.name)),
			stacks: Array.from(stackSet).sort()
		}
	}, [containerData, containerColors, isVolumeChart])

	// Filter containers based on selected stacks
	const filteredContainers = useMemo(() => {
		if (stackFilter.length === 0 || stackFilter.includes("all")) {
			return containerStackData.containers
		}
		return containerStackData.containers.filter(container => 
			stackFilter.includes(container.stack)
		)
	}, [containerStackData.containers, stackFilter])

	// Filter stacks based on selected containers
	const filteredStacks = useMemo(() => {
		if (containerFilter.length === 0 || containerFilter.includes("all")) {
			return containerStackData.stacks
		}
		const selectedContainerStacks = new Set<string>()
		containerStackData.containers.forEach(container => {
			if (containerFilter.includes(container.name)) {
				selectedContainerStacks.add(container.stack)
			}
		})
		return containerStackData.stacks.filter(stack => 
			selectedContainerStacks.has(stack)
		)
	}, [containerStackData.stacks, containerStackData.containers, containerFilter])

	const handleContainerChange = (value: string) => {
		if (value === "all") {
			$containerFilter.set([])
		} else if (containerFilter.includes(value)) {
			// Remove if already selected
			$containerFilter.set(containerFilter.filter(c => c !== value))
		} else {
			// Add to selection
			$containerFilter.set([...containerFilter, value])
		}
	}

	const handleStackChange = (value: string) => {
		if (value === "all") {
			$stackFilter.set([])
		} else if (stackFilter.includes(value)) {
			// Remove if already selected
			$stackFilter.set(stackFilter.filter(s => s !== value))
		} else {
			// Add to selection
			$stackFilter.set([...stackFilter, value])
		}
	}

	const selectedContainers = containerFilter.length === 0 ? ["all"] : containerFilter
	const selectedStacks = stackFilter.length === 0 ? ["all"] : stackFilter

	// Only show if there are containers
	if (containerStackData.containers.length === 0) {
		return null
	}

	return (
		<Card className="mb-4">
			<CardHeader className="pb-2 pt-2">
				<span className="text-xs text-muted-foreground font-semibold">{t`Filtering`}</span>
			</CardHeader>
			<CardContent className="pt-0 pb-2">
				<div className="grid grid-cols-1 xl:grid-cols-2 gap-3">
					<div>
						<label className="text-xs text-muted-foreground mb-1 block">{t`Containers`}</label>
						<MultiSelectDropdown
							options={[
								{ value: "all", label: t`All Containers`, color: undefined },
								...filteredContainers.map(container => ({
									value: container.name,
									label: container.name,
									color: container.color
								}))
							]}
							selectedValues={containerFilter.length === 0 ? ["all"] : containerFilter}
							onChange={handleContainerChange}
							placeholder={t`Select containers`}
							filtered={stackFilter.length > 0 && !stackFilter.includes("all")}
						/>
					</div>
					
					{containerStackData.stacks.length > 1 && (
						<div>
							<label className="text-xs text-muted-foreground mb-1 block">{t`Stacks`}</label>
							<MultiSelectDropdown
								options={[
									{ value: "all", label: t`All Stacks`, color: undefined },
									...filteredStacks.map(stackName => ({
										value: stackName,
										label: stackName,
										color: (() => {
											const firstContainerInStack = containerStackData.containers.find(container => 
												container.stack === stackName
											)
											return firstContainerInStack ? firstContainerInStack.color : `hsl(${Math.random() * 360}, 60%, 55%)`
										})()
									}))
								]}
								selectedValues={stackFilter.length === 0 ? ["all"] : stackFilter}
								onChange={handleStackChange}
								placeholder={t`Select stacks`}
								filtered={containerFilter.length > 0 && !containerFilter.includes("all")}
							/>
						</div>
					)}
				</div>
			</CardContent>
		</Card>
	)
}

export default function SystemDetail({ name }: { name: string }) {
	const direction = useStore($direction)
	const { t } = useLingui()
	const systems = useStore($systems)
	const chartTime = useStore($chartTime)
	const maxValues = useStore($maxValues)
	const containerFilter = useStore($containerFilter)
	const stackFilter = useStore($stackFilter)
	const [grid, setGrid] = useLocalStorage("grid", true)
	const [system, setSystem] = useState({} as SystemRecord)
	const [systemStats, setSystemStats] = useState([] as SystemStatsRecord[])
	const [containerData, setContainerData] = useState([] as ChartData["containerData"])
	const netCardRef = useRef<HTMLDivElement>(null)
	const persistChartTime = useRef(false)
	const [bottomSpacing, setBottomSpacing] = useState(0)
	const [chartLoading, setChartLoading] = useState(true)
	const isLongerChart = chartTime !== "1h"
	const containerColors = useStore($containerColors)

	useEffect(() => {
		document.title = `${name} / Beszel`
		return () => {
			if (!persistChartTime.current) {
				$chartTime.set($userSettings.get().chartTime)
			}
			persistChartTime.current = false
			setSystemStats([])
			setContainerData([])
			$containerFilter.set([])
			$stackFilter.set([])
			$containerColors.set({})
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
			makeContainerData(containerData)
		})
	}, [system, chartTime])

	// make container stats for charts
	const makeContainerData = useCallback((containers: ContainerStatsRecord[]) => {
		const containerData = [] as ChartData["containerData"]
		
		// Extract all unique containers with their project information
		const allContainers = new Map<string, any>()
		for (let { stats } of containers) {
			for (let container of stats) {
				allContainers.set(container.n, container)
			}
		}
		
		// Generate stack-based colors for all containers
		if (allContainers.size > 0) {
			const colorMap = generateStackBasedColors(Array.from(allContainers.values()))
			$containerColors.set(colorMap)
		}
		
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
												<LayoutGridIcon className="h-[1.2rem] w-[1.2rem] opacity-85" />
											) : (
												<Rows className="h-[1.3rem] w-[1.3rem] opacity-85" />
											)}
										</Button>
									</TooltipTrigger>
									<TooltipContent>{t`Toggle grid`}</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						</div>
					</div>
				</Card>

				{/* Tabbed interface for system and Docker stats */}
				<Tabs defaultValue="system" className="w-full">
					<TabsList className="grid w-full grid-cols-2 mb-4">
						<TabsTrigger value="system" className="flex items-center gap-2">
							<ServerIcon className="h-4 w-4" />
							{t`System Stats`}
						</TabsTrigger>
						<TabsTrigger value="docker" className="flex items-center gap-2">
							<ContainerIcon className="h-4 w-4" />
							{dockerOrPodman(t`Docker Stats`, system)}
						</TabsTrigger>
					</TabsList>

					{/* System Stats Tab */}
					<TabsContent value="system" className="space-y-4">
						{/* main charts */}
						<div className="grid xl:grid-cols-2 gap-4">
							<ChartCard
								empty={dataEmpty}
								grid={grid}
								title={t`CPU Usage`}
								description={t`Average system-wide CPU utilization`}
								cornerEl={maxValSelect}
							>
								<AreaChartDefault chartData={chartData} chartName="CPU Usage" maxToggled={maxValues} unit="%" />
							</ChartCard>

							<ChartCard
								empty={dataEmpty}
								grid={grid}
								title={t`Memory Usage`}
								description={t`Precise utilization at the recorded time`}
							>
								<MemChart chartData={chartData} />
							</ChartCard>

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
								<AreaChartDefault chartData={chartData} chartName="dio" maxToggled={maxValues} />
							</ChartCard>

							<ChartCard
								empty={dataEmpty}
								grid={grid}
								title={t`Bandwidth`}
								cornerEl={maxValSelect}
								description={t`Network traffic of public interfaces`}
							>
								<AreaChartDefault chartData={chartData} chartName="bw" maxToggled={maxValues} />
							</ChartCard>

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
									const sizeFormatter = (value: number, decimals?: number) => {
										const { v, u } = getSizeAndUnit(value, false)
										return toFixedFloat(v, decimals || 1) + u
									}
									return (
										<div key={id} className="contents">
											<ChartCard
												empty={dataEmpty}
												grid={grid}
												title={`${gpu.n} ${t`Usage`}`}
												description={t`Average utilization of ${gpu.n}`}
											>
												<AreaChartDefault chartData={chartData} chartName={`g.${id}.u`} unit="%" />
											</ChartCard>
											<ChartCard
												empty={dataEmpty}
												grid={grid}
												title={`${gpu.n} VRAM`}
												description={t`Precise utilization at the recorded time`}
											>
												<AreaChartDefault
													chartData={chartData}
													chartName={`g.${id}.mu`}
													max={gpu.mt}
													tickFormatter={sizeFormatter}
													contentFormatter={(value) => sizeFormatter(value, 2)}
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
												<AreaChartDefault chartData={chartData} chartName={`efs.${extraFsName}`} maxToggled={maxValues} />
											</ChartCard>
										</div>
									)
								})}
							</div>
						)}
					</TabsContent>

					{/* Docker Stats Tab */}
					<TabsContent value="docker" className="space-y-4">
						{/* Docker Container Charts */}
						{containerData.length > 0 ? (
							<>
								<ContainerFilterBar containerData={containerData} containerColors={containerColors} isVolumeChart={false} />
								<div className="grid xl:grid-cols-2 gap-4">
									<ChartCard
										empty={dataEmpty}
										grid={grid}
										title={dockerOrPodman(t`Docker CPU Usage`, system)}
										description={t`Average CPU utilization of containers`}
									>
										<ContainerChart chartData={chartData} dataKey="c" chartType={ChartType.CPU} />
									</ChartCard>
									<ChartCard
										empty={dataEmpty}
										grid={grid}
										title={dockerOrPodman(t`Docker Memory Usage`, system)}
										description={t`Average memory utilization of containers`}
									>
										<ContainerChart chartData={chartData} dataKey="m" chartType={ChartType.Memory} />
									</ChartCard>
									<ChartCard
										empty={dataEmpty}
										grid={grid}
										title={dockerOrPodman(t`Docker Network Usage`, system)}
										description={t`Average network utilization of containers`}
									>
										<ContainerChart chartData={chartData} dataKey="n" chartType={ChartType.Network} />
									</ChartCard>
									{/* Docker Volumes Chart */}
									<ChartCard
										empty={dataEmpty}
										grid={grid}
										title={dockerOrPodman(t`Docker Volumes`, system)}
										description={t`Storage usage of Docker volumes`}
									>
										{(() => {
											// Check if any containers have volume data, accounting for current filters
											const hasVolumes = containerData.some(stats => {
												if (!stats.created) return false
												return Object.entries(stats).some(([containerName, container]) => {
													if (containerName === "created") return false
													
													// Apply container filter
													if (containerFilter.length > 0 && !containerFilter.includes(containerName)) {
														return false
													}
													
													// Apply stack filter
													if (stackFilter.length > 0 && typeof container === 'object' && container) {
														const stackName = (container as any).p || "—"
														if (!stackFilter.includes(stackName)) {
															return false
														}
													}
													
													return container && typeof container === 'object' && 'v' in container && 
														container.v && Object.keys(container.v).length > 0
												})
											})
											
											return hasVolumes ? (
												<ContainerChart chartData={chartData} dataKey="v" chartType={ChartType.Volume} />
											) : (
												<div className="flex items-center justify-center h-full opacity-100">
													<div className="text-center space-y-2">
														<ContainerIcon className="h-8 w-8 mx-auto text-muted-foreground" />
														<p className="text-sm text-muted-foreground">
															{containerFilter.length > 0 || stackFilter.length > 0 
																? t`No volumes for the selected containers`
																: t`No volumes found`
															}
														</p>
													</div>
												</div>
											)
										})()}
									</ChartCard>
									{/* Docker Health+Uptime Combined Chart */}
									<ChartCard
										empty={dataEmpty}
										grid={grid}
										title={dockerOrPodman(t`Docker Health & Uptime`, system)}
										description={t`Health status and Uptime of containers`}
									>
										{(() => {
											// Check if any containers have health or uptime data, accounting for current filters
											const hasHealthUptime = containerData.some(stats => {
												if (!stats.created) return false
												return Object.entries(stats).some(([containerName, container]) => {
													if (containerName === "created") return false
													
													// Apply container filter
													if (containerFilter.length > 0 && !containerFilter.includes(containerName)) {
														return false
													}
													
													// Apply stack filter
													if (stackFilter.length > 0 && typeof container === 'object' && container) {
														const stackName = (container as any).p || "—"
														if (!stackFilter.includes(stackName)) {
															return false
														}
													}
													
													return container && typeof container === 'object' && 
														(('h' in container && container.h) || ('u' in container && container.u))
												})
											})
											
											return hasHealthUptime ? (
												<ContainerChart chartData={chartData} dataKey="h" chartType={ChartType.HealthUptime} />
											) : (
												<div className="flex items-center justify-center h-full opacity-100">
													<div className="text-center space-y-2">
														<ContainerIcon className="h-8 w-8 mx-auto text-muted-foreground" />
														<p className="text-sm text-muted-foreground">
															{containerFilter.length > 0 || stackFilter.length > 0 
																? t`No health/uptime data for the selected containers`
																: t`No health/uptime data found`
															}
														</p>
													</div>
												</div>
											)
										})()}
									</ChartCard>
								</div>
							</>
						) : (
							<Card className="flex items-center justify-center h-64">
								<div className="text-center space-y-2">
									<ContainerIcon className="h-12 w-12 mx-auto text-muted-foreground" />
									<h3 className="text-lg font-semibold">{t`No containers found`}</h3>
									<p className="text-sm text-muted-foreground">
										{t`This system doesn't have any running Docker containers.`}
									</p>
								</div>
							</Card>
						)}
					</TabsContent>
				</Tabs>
			</div>

			{/* add space for tooltip if more than 12 containers */}
			{bottomSpacing > 0 && <span className="block" style={{ height: bottomSpacing }} />}
		</>
	)
}

function FilterBar({ store = $containerFilter }: { store?: any }) {
	const containerFilter = useStore(store)
	const { t } = useLingui()

	const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
		const value = e.target.value
		if (Array.isArray(containerFilter)) {
			if (value) {
				store.set([value])
			} else {
				store.set([])
			}
		} else {
			store.set(value)
		}
	}, [store, containerFilter])

	return (
		<>
			<Input
				placeholder={t`Filter...`}
				className="ps-4 pe-8"
				value={Array.isArray(containerFilter) ? containerFilter.join(', ') : containerFilter}
				onChange={handleChange}
			/>
			{(Array.isArray(containerFilter) ? containerFilter.length > 0 : !!containerFilter) && (
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label="Clear"
					className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
					onClick={() => store.set(Array.isArray(containerFilter) ? [] : "")}
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
