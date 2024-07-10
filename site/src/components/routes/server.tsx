import { $servers, pb } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord } from '@/types'
import { useEffect, useState } from 'react'
import { useRoute } from 'wouter'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'

function timestampToBrowserTime(timestamp: string) {
	const date = new Date(timestamp)
	return date.toLocaleString()
}

export function ServerDetail() {
	const servers = useStore($servers)
	const [_, params] = useRoute('/server/:name')
	const [server, setServer] = useState({} as SystemRecord)
	const [containers, setContainers] = useState([] as ContainerStatsRecord[])
	// const [serverId, setServerId] = useState('')

	useEffect(() => {
		document.title = params!.name
	}, [])

	useEffect(() => {
		if ($servers.get().length === 0) {
			console.log('skipping')
			return
		}
		console.log('running')
		const matchingServer = servers.find((s) => s.name === params!.name) as SystemRecord

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
