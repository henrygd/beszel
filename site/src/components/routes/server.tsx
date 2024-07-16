import { $updatedSystem, $systems, pb } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord, SystemStatsRecord } from '@/types'
import { Suspense, lazy, useEffect, useMemo, useState } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'
import Spinner from '../spinner'
import { ClockArrowUp, CpuIcon, GlobeIcon } from 'lucide-react'
import ChartTimeSelect from '../charts/chart-time-select'
import { cn, getPbTimestamp } from '@/lib/utils'
import { Separator } from '../ui/separator'

const CpuChart = lazy(() => import('../charts/cpu-chart'))
const ContainerCpuChart = lazy(() => import('../charts/container-cpu-chart'))
const MemChart = lazy(() => import('../charts/mem-chart'))
const ContainerMemChart = lazy(() => import('../charts/container-mem-chart'))
const DiskChart = lazy(() => import('../charts/disk-chart'))

function timestampToBrowserTime(timestamp: string) {
	const date = new Date(timestamp)
	return date.toLocaleString()
}

export default function ServerDetail({ name }: { name: string }) {
	const servers = useStore($systems)
	const updatedSystem = useStore($updatedSystem)
	const [server, setServer] = useState({} as SystemRecord)
	const [containers, setContainers] = useState([] as ContainerStatsRecord[])

	const [serverStats, setServerStats] = useState([] as SystemStatsRecord[])
	const [cpuChartData, setCpuChartData] = useState([] as { time: number; cpu: number }[])
	const [memChartData, setMemChartData] = useState(
		[] as { time: number; mem: number; memUsed: number; memCache: number }[]
	)
	const [diskChartData, setDiskChartData] = useState(
		[] as { time: number; disk: number; diskUsed: number }[]
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
			console.log('unmounting')
			setServer({} as SystemRecord)
			setCpuChartData([])
			setMemChartData([])
			setDiskChartData([])
			setDockerCpuChartData([])
			setDockerMemChartData([])
		}
	}, [name])

	useEffect(() => {
		if (server?.id && server.name === name) {
			return
		}
		const matchingServer = servers.find((s) => s.name === name) as SystemRecord
		if (matchingServer) {
			setServer(matchingServer)
		}
	}, [name, server])

	// if visiting directly, make sure server gets set when servers are loaded
	// useEffect(() => {
	// 	if (!('id' in server)) {
	// 		const matchingServer = servers.find((s) => s.name === name) as SystemRecord
	// 		if (matchingServer) {
	// 			console.log('setting server')
	// 			setServer(matchingServer)
	// 		}
	// 	}
	// }, [servers])

	// get stats
	useEffect(() => {
		if (!('id' in server)) {
			console.log('no id in server')
			return
		} else {
			console.log('id in server')
		}
		pb.collection<SystemStatsRecord>('system_stats')
			.getFullList({
				filter: `system="${server.id}" && created > "${getPbTimestamp('1h')}"`,
				fields: 'created,stats',
				sort: '-created',
			})
			.then((records) => {
				// console.log('sctats', records)
				setServerStats(records)
			})
	}, [server])

	useEffect(() => {
		if (updatedSystem.id === server.id) {
			setServer(updatedSystem)
		}
	}, [updatedSystem])

	// get cpu data
	useEffect(() => {
		if (!serverStats.length) {
			return
		}

		console.log('stats', serverStats)
		// let maxCpu = 0
		const cpuData = [] as typeof cpuChartData
		const memData = [] as typeof memChartData
		const diskData = [] as typeof diskChartData
		for (let { created, stats } of serverStats) {
			const time = new Date(created).getTime()
			cpuData.push({ time, cpu: stats.cpu })
			memData.push({
				time,
				mem: stats.m,
				memUsed: stats.mu,
				memCache: stats.mb,
			})
			diskData.push({ time, disk: stats.d, diskUsed: stats.du })
		}
		setCpuChartData(cpuData.reverse())
		setMemChartData(memData.reverse())
		setDiskChartData(diskData.reverse())
	}, [serverStats])

	useEffect(() => {
		pb.collection<ContainerStatsRecord>('container_stats')
			.getFullList({
				filter: `system="${server.id}" && created > "${getPbTimestamp('1h')}"`,
				fields: 'created,stats',
				sort: '-created',
			})
			.then((records) => {
				setContainers(records)
			})
	}, [server])

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
		setDockerCpuChartData(dockerCpuData.reverse())
		setDockerMemChartData(dockerMemData.reverse())
	}, [containers])
	const uptime = useMemo(() => {
		console.log('making uptime')
		let uptime = server.info?.u || 0
		if (uptime < 172800) {
			return `${Math.floor(uptime / 3600)} hours`
		}
		return `${Math.floor(server.info?.u / 86400)} days`
	}, [server.info?.u])

	if (!('id' in server)) {
		return null
	}

	return (
		<div className="grid gap-5 mb-10">
			<Card>
				<div className="grid gap-1.5 px-6 pt-4 pb-5">
					<h1 className="text-[1.6rem] font-semibold">{server.name}</h1>
					<div className="flex flex-wrap items-center gap-3 text-sm opacity-90">
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
						<div className="flex gap-1.5 items-center">
							<GlobeIcon className="h-4 w-4" /> {server.host}
						</div>
						<Separator orientation="vertical" className="h-4 bg-primary/30" />
						<div className="flex gap-1.5 items-center">
							<ClockArrowUp className="h-4 w-4" /> {uptime}
						</div>
						<Separator orientation="vertical" className="h-4 bg-primary/30" />
						<div className="flex gap-1.5 items-center">
							<CpuIcon className="h-4 w-4" />
							{server.info?.m} ({server.info?.c}c / {server.info.t}t)
						</div>
					</div>
				</div>
			</Card>

			<ChartCard
				title="Total CPU Usage"
				description="Average system-wide CPU utilization as a percentage"
			>
				<CpuChart chartData={cpuChartData} />
			</ChartCard>

			{dockerCpuChartData.length > 0 && (
				<ChartCard title="Docker CPU Usage" description="CPU utilization of docker containers">
					<ContainerCpuChart chartData={dockerCpuChartData} />
				</ChartCard>
			)}
			<ChartCard title="Total Memory Usage" description="Precise utilization at the recorded time">
				<MemChart chartData={memChartData} />
			</ChartCard>

			{dockerMemChartData.length > 0 && (
				<ChartCard title="Docker Memory Usage" description="Memory usage of docker containers">
					<ContainerMemChart chartData={dockerMemChartData} />
				</ChartCard>
			)}
			<ChartCard title="Disk Usage" description="Precise usage at the recorded time">
				<DiskChart chartData={diskChartData} />
			</ChartCard>
			<Card>
				<CardHeader>
					<CardTitle className={'mb-3'}>{server.name}</CardTitle>
					<CardDescription>
						{server.ip} - last updated: {timestampToBrowserTime(server.updated)}
					</CardDescription>
				</CardHeader>
				<CardContent>
					<pre>{JSON.stringify(server, null, 2)}</pre>
				</CardContent>
			</Card>
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
			<CardHeader className="pb-5 pt-4">
				<CardTitle className="flex gap-2 items-center justify-between -mb-1.5">
					{title}
					<ChartTimeSelect className="translate-y-1" />
				</CardTitle>
				<CardDescription>{description}</CardDescription>
			</CardHeader>
			<CardContent className={'pl-1 w-[calc(100%-1.6em)] h-52 relative'}>
				<Suspense fallback={<Spinner />}>{children}</Suspense>
			</CardContent>
		</Card>
	)
}
