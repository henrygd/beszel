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
		if (!system.id || chartTime === "1m") return
		let unsub = () => {}
		if (chartTime !== "1m") return

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

	return (
		<div className="grid gap-4 mb-14 overflow-x-clip">
			<InfoBar system={system} chartData={chartData} grid={grid} setGrid={setGrid} details={details} />

			<Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
				<Card className="p-1">
					<TabsList className="w-full h-11 grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
						<TabsTrigger value="overview" className="h-9"><Trans>Core Metrics</Trans></TabsTrigger>
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
