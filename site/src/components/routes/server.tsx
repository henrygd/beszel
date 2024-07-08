import { pb } from '@/lib/stores'
import { SystemRecord } from '@/types'
import { useEffect, useState } from 'preact/hooks'
import { useRoute } from 'wouter-preact'

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
			<h1>{node.name}</h1>
			<pre>{JSON.stringify(node, null, 2)}</pre>
		</>
	)
}
