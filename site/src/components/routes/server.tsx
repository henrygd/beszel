import { $servers, pb } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord } from '@/types'
import { Suspense, lazy, useEffect, useState } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'
import Spinner from '../spinner'
// import { CpuChart } from '../cpu-chart'

const CpuChart = lazy(() => import('../cpu-chart'))

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

	useEffect(() => {
		document.title = name
	}, [])

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
			.getList(1, 2, {
				filter: `system="${matchingServer.id}"`,
				fields: 'created,stats',
				sort: '-created',
			})
			.then((records) => {
				console.log('records', records)
				setContainers(records.items)
			})
	}, [servers])

	return (
		<>
			<div className="grid grid-cols-1 gap-10">
				<Card>
					<CardHeader>
						<CardTitle>CPU Usage</CardTitle>
						<CardDescription>Showing total visitors for the last 30 minutes</CardDescription>
					</CardHeader>
					<CardContent className="pl-0 w-[calc(100%-2em)] h-72 relative">
						<Suspense fallback={<Spinner />}>
							<CpuChart />
						</Suspense>
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

			<Card>
				<CardHeader>
					<CardTitle className={'mb-3'}>Containers</CardTitle>
				</CardHeader>
				<CardContent>
					<pre>{JSON.stringify(containers, null, 2)}</pre>
				</CardContent>
			</Card>
		</>
	)
}
