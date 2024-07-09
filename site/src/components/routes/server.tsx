import { pb } from '@/lib/stores'
import { SystemRecord } from '@/types'
import { useEffect, useState } from 'preact/hooks'
import { useRoute } from 'wouter-preact'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'

export function ServerDetail() {
	const [_, params] = useRoute('/server/:name')
	const [node, setNode] = useState({} as SystemRecord)

	useEffect(() => {
		document.title = params!.name
	}, [])

	useEffect(() => {
		pb.collection<SystemRecord>('systems')
			.getFirstListItem(`name="${params!.name}"`)
			.then((record) => {
				setNode(record)
			})
	})

	return (
		<>
			<Card>
				<CardHeader>
					<CardTitle className={'mb-3'}>{node.name}</CardTitle>
					<CardDescription>5.342.34.234</CardDescription>
				</CardHeader>
				<CardContent>
					<pre>{JSON.stringify(node, null, 2)}</pre>
				</CardContent>
			</Card>
		</>
	)
}
