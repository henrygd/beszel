import { $servers, pb } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord, SystemStats, SystemStatsRecord } from '@/types'
import { Suspense, lazy, useEffect, useState } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'
import Spinner from '../spinner'
import CpuChart from '../charts/cpu-chart'
import MemChart from '../charts/mem-chart'
import DiskChart from '../charts/disk-chart'
import ContainerCpuChart from '../charts/container-cpu-chart'

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
	const [cpuChartData, setCpuChartData] = useState({} as { time: string; cpu: number }[])
	const [memChartData, setMemChartData] = useState(
		{} as { time: string; mem: number; memUsed: number }[]
	)
	const [diskChartData, setDiskChartData] = useState(
		{} as { time: string; disk: number; diskUsed: number }[]
	)
	const [containerCpuChartData, setContainerCpuChartData] = useState(
		[] as Record<string, number | string>[]
	)

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
		setCpuChartData(cpuData.reverse())
		setMemChartData(memData.reverse())
		setDiskChartData(diskData.reverse())
	}, [serverStats])

	useEffect(() => {
		document.title = name
	}, [name])

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
				obj[name] = cpu * 10
			}
			containerCpuData.push(obj)
		}
		setContainerCpuChartData(containerCpuData.reverse())
		console.log('containerCpuData', containerCpuData)
	}, [containers])

	return (
		<>
			<div className="grid grid-cols-2 gap-6 mb-10">
				<Card className="pb-2 col-span-2">
					<CardHeader>
						<CardTitle>CPU Usage</CardTitle>
						<CardDescription>
							Average usage of the one minute preceding the recorded time
						</CardDescription>
					</CardHeader>
					<CardContent className={'pl-1 w-[calc(100%-2em)] h-52 relative'}>
						{/* <Suspense fallback={<Spinner />}> */}
						<CpuChart chartData={cpuChartData} />
						{/* </Suspense> */}
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
				<Card className="pb-2 col-span-2">
					<CardHeader>
						<CardTitle>Container CPU Usage</CardTitle>
						<CardDescription>
							Average usage of the one minute preceding the recorded time
						</CardDescription>
					</CardHeader>
					<CardContent className={'pl-1 w-[calc(100%-2em)] h-64 relative'}>
						{/* <Suspense fallback={<Spinner />}> */}
						{containerCpuChartData.length > 0 && (
							<ContainerCpuChart chartData={containerCpuChartData} />
						)}
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
