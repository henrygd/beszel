import { useEffect, useState } from 'preact/hooks'
import { pb } from '@/lib/stores'
import { SystemRecord } from '@/types'
import { DataTable } from '../server-table/data-table'

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
			<DataTable data={systems} />
			{/* <pre>{JSON.stringify(systems, null, 2)}</pre> */}
		</>
	)
}
