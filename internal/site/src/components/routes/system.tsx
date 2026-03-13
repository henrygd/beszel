import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { timeTicks } from "d3-time"
import { subscribeKeys } from "nanostores"
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { getPbTimestamp, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import {
	$allSystemsById,
	$allSystemsByName,
	$chartTime,
	$containerFilter,
	$direction,
	$systems,
	$userSettings,
} from "@/lib/stores"
import { chartTimeData, compareSemVer, listen, parseSemVer, useBrowserStorage } from "@/lib/utils"
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
import { CoreMetricsTab, DisksTab, GpuTab, ContainersTab, ServicesTab } from "./system/tabs"

type ChartTimeData = {
	time: number
	data: { ticks: number[]; domain: number[] }
	chartTime: ChartTimes
}

const cache = new Map<string, ChartTimeData | SystemStatsRecord[] | ContainerStatsRecord[]>()

function getTimeData(chartTime: ChartTimes, lastCreated: number) {
	const cached = cache.get("td") as ChartTimeData | undefined
	if (cached?.chartTime === chartTime && (!lastCreated || cached.time >= lastCreated)) {
		return cached.data
	}
	const now = new Date()
	const startTime = chartTimeData[chartTime].getOffset(now)
	const ticks = timeTicks(startTime, now, chartTimeData[chartTime].ticks ?? 12).map((d) => d.getTime())
	const data = { ticks, domain: [startTime.getTime(), now.getTime()] }
	cache.set("td", { time: now.getTime(), data, chartTime })
	return data
}

function addEmptyValues<T extends { created: string | number | null }>(
	prevRecords: T[],
	newRecords: T[],
	expectedInterval: number
): T[] {
	const result: T[] = []
	let prevTime = (prevRecords.at(-1)?.created ?? 0) as number
	for (const record of newRecords) {
		if (record.created !== null) {
			record.created = new Date(record.created).getTime()
			if (prevTime && record.created - prevTime > expectedInterval * 1.5) {
				result.push({ created: null, ...("stats" in record ? { stats: null } : {}) } as T)
			}
			prevTime = record.created
		}
		result.push(record)
	}
	return result
}

async function getStats<T extends SystemStatsRecord | ContainerStatsRecord>(
	collection: string,
	system: SystemRecord,
	chartTime: ChartTimes
): Promise<T[]> {
	const lastCached = (cache.get(`${system.id}_${chartTime}_${collection}`) as T[] | undefined)?.at(-1)?.created as number
	return pb.collection<T>(collection).getFullList({
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
	const direction = useStore($direction)
	const userSettings = useStore($userSettings)
	const [grid, setGrid] = useBrowserStorage("grid", true)
	const [system, setSystem] = useState({} as SystemRecord)
	const [systemStats, setSystemStats] = useState<SystemStatsRecord[]>([])
	const [containerData, setContainerData] = useState<ChartData["containerData"]>([])
	const [activeTab, setActiveTab] = useBrowserStorage("system-tab", "overview")
	const [details, setDetails] = useState<SystemDetailsRecord>({} as SystemDetailsRecord)
	const persistChartTime = useRef(false)

	useEffect(() => {
		return () => {
			if (!persistChartTime.current) $chartTime.set($userSettings.get().chartTime)
			persistChartTime.current = false
			setSystemStats([])
			setContainerData([])
			setDetails({} as SystemDetailsRecord)
			$containerFilter.set("")
		}
	}, [id])

	useEffect(() => {
		if (!systems.length) return
		const store = $allSystemsById.get()[id] ? $allSystemsById : $allSystemsByName
		return subscribeKeys(store, [id], (newSystems) => {
			const sys = newSystems[id]
			if (sys) {
				setSystem(sys)
				document.title = `${sys.name} / Beszel`
			}
		})
	}, [id, systems.length])

	useEffect(() => {
		if (parseSemVer(system?.info?.v) < parseSemVer("0.13.0")) $chartTime.set("1h")
	}, [system?.info?.v])

	useEffect(() => {
		if (!system.id || system.info?.m) return
		pb.collection<SystemDetailsRecord>("system_details")
			.getOne(system.id, {
				fields: "hostname,kernel,cores,threads,cpu,os,os_name,arch,memory,podman",
				headers: { "Cache-Control": "public, max-age=60" },
			})
			.then(setDetails)
	}, [system.id])

	const makeContainerData = useCallback((containers: ContainerStatsRecord[]) => {
		return containers.map(({ created, stats }) => {
			if (!created) return { created: null } as ChartData["containerData"][0]
			const result: ChartData["containerData"][0] = { created: new Date(created).getTime() }
			for (const c of stats) result[c.n] = c
			return result
		})
	}, [])

	// biome-ignore lint/correctness/useExhaustiveDependencies: not necessary
	useEffect(() => {
		if (!system.id || chartTime !== "1m") return
		let unsub = () => {}

		if (system.status !== SystemStatus.Up || parseSemVer(system?.info?.v).minor < 13) {
			$chartTime.set("1h")
			return
		}
		pb.realtime
			.subscribe(
				"rt_metrics",
				(data: { container: ContainerStatsRecord[]; info: SystemInfo; stats: SystemStats }) => {
					if (data.container?.length) {
						const newData = makeContainerData([{ created: Date.now(), stats: data.container } as unknown as ContainerStatsRecord])
						setContainerData((prev) => addEmptyValues(prev, prev.slice(-59).concat(newData), 1000))
					}
					setSystemStats((prev) =>
						addEmptyValues(prev, prev.slice(-59).concat({ created: Date.now(), stats: data.stats } as SystemStatsRecord), 1000)
					)
				},
				{ query: { system: system.id } }
			)
			.then((us) => { unsub = us })
		return () => unsub?.()
	}, [chartTime, system.id])

	// biome-ignore lint/correctness/useExhaustiveDependencies: not necessary
	useEffect(() => {
		if (!system.id || !chartTime || chartTime === "1m") return
		Promise.allSettled([
			getStats<SystemStatsRecord>("system_stats", system, chartTime),
			getStats<ContainerStatsRecord>("container_stats", system, chartTime),
		]).then(([sysResult, contResult]) => {
			const { expectedInterval } = chartTimeData[chartTime]
			const ssKey = `${system.id}_${chartTime}_system_stats`
			let sysData = (cache.get(ssKey) || []) as SystemStatsRecord[]
			if (sysResult.status === "fulfilled" && sysResult.value.length) {
				sysData = sysData.concat(addEmptyValues(sysData, sysResult.value, expectedInterval)).slice(-100)
				cache.set(ssKey, sysData)
			}
			setSystemStats(sysData)

			const csKey = `${system.id}_${chartTime}_container_stats`
			let contData = (cache.get(csKey) || []) as ContainerStatsRecord[]
			if (contResult.status === "fulfilled" && contResult.value.length) {
				contData = contData.concat(addEmptyValues(contData, contResult.value, expectedInterval)).slice(-100)
				cache.set(csKey, contData)
			}
			setContainerData(makeContainerData(contData))
		})
	}, [system, chartTime])

	useEffect(() => {
		if (!systems.length) return
		const handleKeyUp = (e: KeyboardEvent) => {
			if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return
			const idx = systems.findIndex((s) => s.id === id)
			if (idx === -1 || systems.length <= 1) return
			const offset = e.key === "ArrowLeft" || e.key === "h" ? -1 : e.key === "ArrowRight" || e.key === "l" ? 1 : 0
			if (offset) {
				persistChartTime.current = true
				navigate(getPagePath($router, "system", { id: systems[(idx + offset + systems.length) % systems.length].id }))
			}
		}
		return listen(document, "keyup", handleKeyUp)
	}, [id, systems])

	// biome-ignore lint/correctness/useExhaustiveDependencies: not necessary
	const chartData: ChartData = useMemo(() => {
		const lastCreated = Math.max((systemStats.at(-1)?.created as number) ?? 0, (containerData.at(-1)?.created as number) ?? 0)
		return {
			systemStats,
			containerData,
			chartTime,
			orientation: direction === "rtl" ? "right" : "left",
			...getTimeData(chartTime, lastCreated),
			agentVersion: parseSemVer(system?.info?.v),
		}
	}, [systemStats, containerData, direction])

	if (!system.id) return null

	const hasGpuData = Object.keys(systemStats.at(-1)?.stats?.g ?? {}).length > 0
	const hasContainers = containerData.length > 0 && compareSemVer(chartData.agentVersion, parseSemVer("0.14.0")) >= 0
	const isLinux = !(details?.os ?? system.info?.os)
	const hasServices = isLinux && compareSemVer(chartData.agentVersion, parseSemVer("0.16.0")) >= 0
	const tabCount = 2 + +hasGpuData + +hasContainers + +hasServices

	if (userSettings.disableTabs) {
		return (
			<div className="grid gap-4 mb-14 overflow-x-clip">
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
										return data?.stats?.b?.[0] ?? (data?.stats?.ns ?? 0) * 1024 * 1024
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
										return data?.stats?.b?.[1] ?? (data?.stats?.nr ?? 0) * 1024 * 1024
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
					{chartData.agentVersion?.minor > 12 && (
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
		)
	}

	return (
		<div className="grid gap-4 mb-14 overflow-x-clip">
			<InfoBar system={system} chartData={chartData} grid={grid} setGrid={setGrid} details={details} />

			<Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
				<Card className="p-1">
					<TabsList className="w-full h-11 grid" style={{ gridTemplateColumns: `repeat(${tabCount}, 1fr)` }}>
						<TabsTrigger value="overview" className="h-9"><Trans>Overview</Trans></TabsTrigger>
						<TabsTrigger value="disks" className="h-9"><Trans>Disks</Trans></TabsTrigger>
						{hasGpuData && <TabsTrigger value="gpu" className="h-9"><Trans>GPU</Trans></TabsTrigger>}
						{hasContainers && <TabsTrigger value="containers" className="h-9"><Trans>Containers</Trans></TabsTrigger>}
						{hasServices && <TabsTrigger value="services" className="h-9"><Trans>Services</Trans></TabsTrigger>}
					</TabsList>
				</Card>

				<TabsContent value="overview" className="mt-4">
					<CoreMetricsTab chartData={chartData} grid={grid} />
				</TabsContent>

				<TabsContent value="disks" className="mt-4">
					<DisksTab chartData={chartData} grid={grid} systemId={system.id} />
				</TabsContent>

				{hasGpuData && (
					<TabsContent value="gpu" className="mt-4">
						<GpuTab chartData={chartData} grid={grid} />
					</TabsContent>
				)}

				{hasContainers && (
					<TabsContent value="containers" className="mt-4">
						<ContainersTab chartData={chartData} grid={grid} system={system} />
					</TabsContent>
				)}

				{hasServices && (
					<TabsContent value="services" className="mt-4">
						<ServicesTab systemId={system.id} />
					</TabsContent>
				)}
			</Tabs>
		</div>
	)
})
