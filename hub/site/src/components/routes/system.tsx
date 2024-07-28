import { $updatedSystem, $systems, pb, $chartTime } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord, SystemStatsRecord } from '@/types'
import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'
import Spinner from '../spinner'
import { ClockArrowUp, CpuIcon, GlobeIcon } from 'lucide-react'
import ChartTimeSelect from '../charts/chart-time-select'
import { chartTimeData, cn, getPbTimestamp } from '@/lib/utils'
import { Separator } from '../ui/separator'
import { scaleTime } from 'd3-scale'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../ui/tooltip'

const CpuChart = lazy(() => import('../charts/cpu-chart'))
const ContainerCpuChart = lazy(() => import('../charts/container-cpu-chart'))
const MemChart = lazy(() => import('../charts/mem-chart'))
const ContainerMemChart = lazy(() => import('../charts/container-mem-chart'))
const DiskChart = lazy(() => import('../charts/disk-chart'))
const DiskIoChart = lazy(() => import('../charts/disk-io-chart'))
const BandwidthChart = lazy(() => import('../charts/bandwidth-chart'))

export default function ServerDetail({ name }: { name: string }) {
	const systems = useStore($systems)
	const updatedSystem = useStore($updatedSystem)
	const chartTime = useStore($chartTime)
	const [ticks, setTicks] = useState([] as number[])
	const [server, setServer] = useState({} as SystemRecord)
	const [containers, setContainers] = useState([] as ContainerStatsRecord[])

	const [systemStats, setSystemStats] = useState([] as SystemStatsRecord[])
	const [cpuChartData, setCpuChartData] = useState([] as { time: number; cpu: number }[])
	const [memChartData, setMemChartData] = useState(
		[] as { time: number; mem: number; memUsed: number; memCache: number }[]
	)
	const [diskChartData, setDiskChartData] = useState(
		[] as { time: number; disk: number; diskUsed: number }[]
	)
	const [diskIoChartData, setDiskIoChartData] = useState(
		[] as { time: number; read: number; write: number }[]
	)
	const [bandwidthChartData, setBandwidthChartData] = useState(
		[] as { time: number; sent: number; recv: number }[]
	)
	const [dockerCpuChartData, setDockerCpuChartData] = useState(
		[] as Record<string, number | string>[]
	)
	const [dockerMemChartData, setDockerMemChartData] = useState(
		[] as Record<string, number | string>[]
	)

	useEffect(() => {
		document.title = `${name} / Beszel`
		return () => {
			resetCharts()
			$chartTime.set('1h')
		}
	}, [name])

	const resetCharts = useCallback(() => {
		setSystemStats([])
		setCpuChartData([])
		setMemChartData([])
		setDiskChartData([])
		setBandwidthChartData([])
		setDockerCpuChartData([])
		setDockerMemChartData([])
	}, [])

	useEffect(resetCharts, [chartTime])

	useEffect(() => {
		if (server.id && server.name === name) {
			return
		}
		const matchingServer = systems.find((s) => s.name === name) as SystemRecord
		if (matchingServer) {
			setServer(matchingServer)
		}
	}, [name, server, systems])

	// get stats
	useEffect(() => {
		if (!server.id || !chartTime) {
			return
		}
		pb.collection<SystemStatsRecord>('system_stats')
			.getFullList({
				filter: pb.filter('system={:id} && created > {:created} && type={:type}', {
					id: server.id,
					created: getPbTimestamp(chartTime),
					type: chartTimeData[chartTime].type,
				}),
				fields: 'created,stats',
				sort: 'created',
			})
			.then((records) => {
				// console.log('sctats', records)
				setSystemStats(records)
			})
	}, [server, chartTime])

	useEffect(() => {
		if (updatedSystem.id === server.id) {
			setServer(updatedSystem)
		}
	}, [updatedSystem])

	// create cpu / mem / disk data for charts
	useEffect(() => {
		if (!systemStats.length) {
			return
		}
		const cpuData = [] as typeof cpuChartData
		const memData = [] as typeof memChartData
		const diskData = [] as typeof diskChartData
		const diskIoData = [] as typeof diskIoChartData
		const networkData = [] as typeof bandwidthChartData
		for (let { created, stats } of systemStats) {
			const time = new Date(created).getTime()
			cpuData.push({ time, cpu: stats.cpu })
			memData.push({
				time,
				mem: stats.m,
				memUsed: stats.mu,
				memCache: stats.mb,
			})
			diskData.push({ time, disk: stats.d, diskUsed: stats.du })
			diskIoData.push({ time, read: stats.dr, write: stats.dw })
			networkData.push({ time, sent: stats.ns, recv: stats.nr })
		}
		setCpuChartData(cpuData)
		setMemChartData(memData)
		setDiskChartData(diskData)
		setDiskIoChartData(diskIoData)
		setBandwidthChartData(networkData)
	}, [systemStats])

	useEffect(() => {
		if (!systemStats.length) {
			return
		}
		const now = new Date()
		const startTime = chartTimeData[chartTime].getOffset(now)
		const scale = scaleTime([startTime.getTime(), now], [0, cpuChartData.length])
		setTicks(scale.ticks(chartTimeData[chartTime].ticks).map((d) => d.getTime()))
	}, [chartTime, systemStats])

	// get container stats
	useEffect(() => {
		if (!server.id || !chartTime) {
			return
		}
		pb.collection<ContainerStatsRecord>('container_stats')
			.getFullList({
				filter: pb.filter('system={:id} && created > {:created} && type={:type}', {
					id: server.id,
					created: getPbTimestamp(chartTime),
					type: chartTimeData[chartTime].type,
				}),
				fields: 'created,stats',
				sort: 'created',
			})
			.then((records) => {
				setContainers(records)
			})
	}, [server, chartTime])

	// container stats for charts
	useEffect(() => {
		// console.log('containers', containers)
		const dockerCpuData = [] as Record<string, number | string>[]
		const dockerMemData = [] as Record<string, number | string>[]

		for (let { created, stats } of containers) {
			const time = new Date(created).getTime()
			let cpuData = { time } as (typeof dockerCpuChartData)[0]
			let memData = { time } as (typeof dockerMemChartData)[0]
			for (let container of stats) {
				cpuData[container.n] = container.c
				memData[container.n] = container.m
			}
			dockerCpuData.push(cpuData)
			dockerMemData.push(memData)
		}
		// console.log('containerMemData', containerMemData)
		setDockerCpuChartData(dockerCpuData)
		setDockerMemChartData(dockerMemData)
	}, [containers])
	const uptime = useMemo(() => {
		let uptime = server.info?.u || 0
		if (uptime < 172800) {
			return `${Math.trunc(uptime / 3600)} hours`
		}
		return `${Math.trunc(server.info?.u / 86400)} days`
	}, [server.info?.u])

	if (!server.id) {
		return null
	}

	return (
		<div className="grid gap-4 mb-10">
			<Card>
				<div className="grid gap-2 px-6 pt-4 pb-5">
					<h1 className="text-[1.6rem] font-semibold">{server.name}</h1>
					<div className="flex flex-wrap items-center gap-3 gap-y-2 text-sm opacity-90">
						<div className="capitalize flex gap-2 items-center">
							<span className={cn('relative flex h-3 w-3')}>
								{server.status === 'up' && (
									<span
										className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"
										style={{ animationDuration: '1.5s' }}
									></span>
								)}
								<span
									className={cn('relative inline-flex rounded-full h-3 w-3', {
										'bg-green-500': server.status === 'up',
										'bg-red-500': server.status === 'down',
										'bg-primary/40': server.status === 'paused',
										'bg-yellow-500': server.status === 'pending',
									})}
								></span>
							</span>
							{server.status}
						</div>
						<Separator orientation="vertical" className="h-4 bg-primary/30" />
						<div className="flex gap-1.5">
							<GlobeIcon className="h-4 w-4 mt-[1px]" /> {server.host}
						</div>
						{server.info?.u && (
							<TooltipProvider>
								<Tooltip>
									<Separator orientation="vertical" className="h-4 bg-primary/30" />
									<TooltipTrigger asChild>
										<div className="flex gap-1.5">
											<ClockArrowUp className="h-4 w-4 mt-[1px]" /> {uptime}
										</div>
									</TooltipTrigger>
									<TooltipContent>Uptime</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						)}
						{server.info?.m && (
							<>
								<Separator orientation="vertical" className="h-4 bg-primary/30" />
								<div className="flex gap-1.5">
									<CpuIcon className="h-4 w-4 mt-[1px]" />
									{server.info.m} ({server.info.c}c / {server.info.t}t)
								</div>
							</>
						)}
					</div>
				</div>
			</Card>

			<ChartCard title="Total CPU Usage" description="Average system-wide CPU utilization">
				<CpuChart chartData={cpuChartData} ticks={ticks} />
			</ChartCard>

			{dockerCpuChartData.length > 0 && (
				<ChartCard title="Docker CPU Usage" description="CPU utilization of docker containers">
					<ContainerCpuChart chartData={dockerCpuChartData} ticks={ticks} />
				</ChartCard>
			)}

			<ChartCard title="Total Memory Usage" description="Precise utilization at the recorded time">
				<MemChart chartData={memChartData} ticks={ticks} />
			</ChartCard>

			{dockerMemChartData.length > 0 && (
				<ChartCard title="Docker Memory Usage" description="Memory usage of docker containers">
					<ContainerMemChart chartData={dockerMemChartData} ticks={ticks} />
				</ChartCard>
			)}

			<ChartCard
				title="Disk Usage"
				description="Usage of partition where the root filesystem is mounted"
			>
				<DiskChart chartData={diskChartData} ticks={ticks} />
			</ChartCard>

			<ChartCard title="Disk I/O" description="Throughput of root filesystem">
				<DiskIoChart chartData={diskIoChartData} ticks={ticks} />
			</ChartCard>

			<ChartCard title="Bandwidth" description="Network traffic of public interfaces">
				<BandwidthChart chartData={bandwidthChartData} ticks={ticks} />
			</ChartCard>
		</div>
	)
}

function ChartCard({
	title,
	description,
	children,
}: {
	title: string
	description: string
	children: React.ReactNode
}) {
	return (
		<Card className="pb-4 col-span-full">
			<CardHeader className="pb-5 pt-4 relative space-y-1">
				<CardTitle className="text-xl sm:text-2xl">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
				<div className="w-full pt-1 sm:w-40 sm:absolute top-1.5 right-3.5">
					<ChartTimeSelect />
				</div>
			</CardHeader>
			<CardContent className={'pl-1 w-[calc(100%-1.6em)] h-52 relative'}>
				<Suspense fallback={<Spinner />}>{children}</Suspense>
			</CardContent>
		</Card>
	)
}
