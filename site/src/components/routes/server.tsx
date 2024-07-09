import { pb } from '@/lib/stores'
import { SystemRecord } from '@/types'
import { useEffect, useState } from 'preact/hooks'
import { useRoute } from 'wouter-preact'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'

export function ServerDetail() {
	const [_, params] = useRoute('/server/:name')
	const [server, setServer] = useState({} as SystemRecord)

	useEffect(() => {
		document.title = params!.name
	}, [])

	useEffect(() => {
		pb.collection<SystemRecord>('systems')
			.getFirstListItem(`name="${params!.name}"`)
			.then((record) => {
				setServer(record)
			})
	})

	return (
		<>
			<Card>
				<CardHeader>
					<CardTitle className={'mb-3'}>{server.name}</CardTitle>
					<CardDescription>5.342.34.234</CardDescription>
				</CardHeader>
				<CardContent>
					<pre>{JSON.stringify(server, null, 2)}</pre>
				</CardContent>
			</Card>
		</>
	)
}
