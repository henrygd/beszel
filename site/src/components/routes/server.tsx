import { $servers, pb } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord, SystemStatsRecord } from '@/types'
import { Suspense, lazy, useEffect, useState } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'
import Spinner from '../spinner'
import CpuChart from '../charts/cpu-chart'
import MemChart from '../charts/mem-chart'
import DiskChart from '../charts/disk-chart'
import ContainerCpuChart from '../charts/container-cpu-chart'
import { CpuIcon } from 'lucide-react'

// const CpuChart = lazy(() => import('../cpu-chart'))

function timestampToBrowserTime(timestamp: string) {
	const date = new Date(timestamp)
	return date.toLocaleString()
}

// function addColors(objects: Record<string, any>[]) {
// 	objects.forEach((obj, index) => {
// 		const hue = ((index * 360) / objects.length) % 360 // Distribute hues evenly
// 		obj.fill = `hsl(${hue}, 100%, 50%)` // Set fill to HSL color with full saturation and 50% lightness
// 	})
// }

export default function ServerDetail({ name }: { name: string }) {
	const servers = useStore($servers)
	const [server, setServer] = useState({} as SystemRecord)
	const [containers, setContainers] = useState([] as ContainerStatsRecord[])

	const [serverStats, setServerStats] = useState([] as SystemStatsRecord[])
	const [cpuChartData, setCpuChartData] = useState(
		{} as { max: number; data: { time: string; cpu: number }[] }
	)
	const [memChartData, setMemChartData] = useState(
		{} as { time: string; mem: number; memUsed: number }[]
	)
	const [diskChartData, setDiskChartData] = useState(
		{} as { time: string; disk: number; diskUsed: number }[]
	)
	const [containerCpuChartData, setContainerCpuChartData] = useState(
		[] as Record<string, number | string>[]
	)

	useEffect(() => {
		document.title = name
		return () => {
			setContainerCpuChartData([])
			setCpuChartData({} as { max: number; data: { time: string; cpu: number }[] })
			setMemChartData([] as { time: string; mem: number; memUsed: number }[])
			setDiskChartData([] as { time: string; disk: number; diskUsed: number }[])
		}
	}, [name])

	// get stats
	useEffect(() => {
		if (!('name' in server)) {
			return
		}

		pb.collection<SystemStatsRecord>('system_stats')
			.getList(1, 60, {
				filter: `system="${server.id}"`,
				fields: 'created,stats',
				sort: '-created',
			})
			.then((records) => {
				console.log('stats', records)
				setServerStats(records.items)
			})
	}, [server])

	// get cpu data
	useEffect(() => {
		if (!serverStats.length) {
			return
		}
		const cpuData = [] as { time: string; cpu: number }[]
		const memData = [] as { time: string; mem: number; memUsed: number }[]
		const diskData = [] as { time: string; disk: number; diskUsed: number }[]
		for (let { created, stats } of serverStats) {
			cpuData.push({ time: created, cpu: stats.cpu })
			memData.push({ time: created, mem: stats.mem, memUsed: stats.memUsed })
			diskData.push({ time: created, disk: stats.disk, diskUsed: stats.diskUsed })
		}
		setCpuChartData({
			max: Math.ceil(Math.max(...cpuData.map((d) => d.cpu))),
			data: cpuData.reverse(),
		})
		setMemChartData(memData.reverse())
		setDiskChartData(diskData.reverse())
	}, [serverStats])

	useEffect(() => {
		if ($servers.get().length === 0) {
			console.log('skipping')
			return
		}
		console.log('running')
		const matchingServer = servers.find((s) => s.name === name) as SystemRecord

		setServer(matchingServer)

		console.log('matchingServer', matchingServer)
		// pb.collection<SystemRecord>('systems')
		// 	.getOne(serverId)
		// 	.then((record) => {
		// 		setServer(record)
		// 	})

		pb.collection<ContainerStatsRecord>('container_stats')
			.getList(1, 60, {
				filter: `system="${matchingServer.id}"`,
				fields: 'created,stats',
				sort: '-created',
			})
			.then((records) => {
				// console.log('records', records)
				setContainers(records.items)
			})
	}, [servers, name])

	// container stats for charts
	useEffect(() => {
		console.log('containers', containers)
		const containerCpuData = [] as Record<string, number | string>[]

		for (let { created, stats } of containers) {
			let obj = { time: created } as Record<string, number | string>
			for (let { name, cpu } of stats) {
				obj[name] = cpu
			}
			containerCpuData.push(obj)
		}
		setContainerCpuChartData(containerCpuData.reverse())
	}, [containers])

	return (
		<>
			<div className="grid gap-6 mb-10">
				<Card className="pb-2">
					<CardHeader>
						<CardTitle className="flex gap-2 justify-between">
							<span>CPU Usage</span>
							<CpuIcon className="opacity-70" />
						</CardTitle>
						<CardDescription>
							Average usage of the one minute preceding the recorded time
						</CardDescription>
					</CardHeader>
					<CardContent className={'pl-1 w-[calc(100%-2em)] h-52 relative'}>
						<Suspense fallback={<Spinner />}>
							<CpuChart chartData={cpuChartData.data} max={cpuChartData.max} />
						</Suspense>
					</CardContent>
				</Card>
				<Card className="pb-2">
					<CardHeader>
						<CardTitle className="flex gap-2 justify-between">
							<span>Docker CPU Usage</span>
							<CpuIcon className="opacity-70" />
						</CardTitle>{' '}
						<CardDescription>CPU usage of docker containers</CardDescription>
					</CardHeader>
					<CardContent className={'pl-1 w-[calc(100%-2em)] h-52 relative'}>
						<Suspense fallback={<Spinner />}>
							<ContainerCpuChart chartData={containerCpuChartData} max={cpuChartData.max} />
						</Suspense>
					</CardContent>
				</Card>
				<Card className="pb-2">
					<CardHeader>
						<CardTitle>Memory Usage</CardTitle>
						<CardDescription>Precise usage at the recorded time</CardDescription>
					</CardHeader>
					<CardContent className={'pl-1 w-[calc(100%-2em)] h-52 relative'}>
						{/* <Suspense fallback={<Spinner />}> */}
						<MemChart chartData={memChartData} />
						{/* </Suspense> */}
					</CardContent>
				</Card>
				<Card className="pb-2">
					<CardHeader>
						<CardTitle>Disk Usage</CardTitle>
						<CardDescription>Precise usage at the recorded time</CardDescription>
					</CardHeader>
					<CardContent className={'pl-1 w-[calc(100%-2em)] h-52 relative'}>
						{/* <Suspense fallback={<Spinner />}> */}
						<DiskChart chartData={diskChartData} />
						{/* </Suspense> */}
					</CardContent>
				</Card>
			</div>

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

			{/* <Card>
				<CardHeader>
					<CardTitle className={'mb-3'}>Containers</CardTitle>
				</CardHeader>
				<CardContent>
					<pre>{JSON.stringify(containers, null, 2)}</pre>
				</CardContent>
			</Card> */}
		</>
	)
}
