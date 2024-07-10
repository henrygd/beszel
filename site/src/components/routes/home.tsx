import { Suspense, lazy, useEffect } from 'react'
import { $servers, pb } from '@/lib/stores'
// import { DataTable } from '../server-table/data-table'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { SystemRecord } from '@/types'
import { updateServerList } from '@/lib/utils'

const DataTable = lazy(() => import('../server-table/data-table'))

export default function () {
	useEffect(() => {
		document.title = 'Home'
	}, [])

	useEffect(updateServerList, [])

	useEffect(() => {
		pb.collection<SystemRecord>('systems').subscribe('*', (e) => {
			const curServers = $servers.get()
			const newServers = []
			console.log('e', e)
			if (e.action === 'delete') {
				for (const server of curServers) {
					if (server.id !== e.record.id) {
						newServers.push(server)
					}
				}
			} else {
				let found = 0
				for (const server of curServers) {
					if (server.id === e.record.id) {
						found = newServers.push(e.record)
					} else {
						newServers.push(server)
					}
				}
				if (!found) {
					newServers.push(e.record)
				}
			}
			$servers.set(newServers)
		})
		return () => {
			pb.collection('systems').unsubscribe('*')
		}
	}, [])

	return (
		<>
			<Card>
				<CardHeader>
					<CardTitle className={'mb-1.5'}>All Servers</CardTitle>
					<CardDescription>
						Updated in real time. Press{' '}
						<kbd className="pointer-events-none inline-flex h-5 select-none items-center gap-0.5 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground opacity-100">
							<span className="text-xs">⌘</span>K
						</kbd>{' '}
						to open the command palette.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<Suspense>
						<DataTable />
					</Suspense>
				</CardContent>
			</Card>
		</>
	)
}
