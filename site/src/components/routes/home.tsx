import { useEffect, useState } from 'preact/hooks'
import { pb } from '@/lib/stores'
import { SystemRecord } from '@/types'
import { DataTable } from '../server-table/data-table'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'

export function Home() {
	const [systems, setSystems] = useState([] as SystemRecord[])

	useEffect(() => {
		document.title = 'Home'
	}, [])

	useEffect(() => {
		pb.collection<SystemRecord>('systems')
			.getFullList({
				sort: 'name',
			})
			.then((items) => {
				setSystems(items)
			})

		// pb.collection<SystemRecord>('systems').subscribe('*', (e) => {
		// 	setSystems((curSystems) => {
		// 		const i = curSystems.findIndex((s) => s.id === e.record.id)
		// 		if (i > -1) {
		// 			const newSystems = [...curSystems]
		// 			newSystems[i] = e.record
		// 			return newSystems
		// 		} else {
		// 			return [...curSystems, e.record]
		// 		}
		// 	})
		// })
		// return () => pb.collection('systems').unsubscribe('*')
	}, [])

	// if (!systems.length) return <>Loading...</>

	return (
		<>
			<Card>
				<CardHeader>
					<CardTitle className={'mb-3'}>All Systems</CardTitle>
					<CardDescription>
						Press{' '}
						<kbd className="pointer-events-none inline-flex h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground opacity-100">
							<span className="text-xs">⌘</span>K
						</kbd>{' '}
						to open the command palette.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<DataTable data={systems} />
				</CardContent>
			</Card>
			{/* <pre>{JSON.stringify(systems, null, 2)}</pre> */}
		</>
	)
}
