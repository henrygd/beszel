import { useEffect } from 'preact/hooks'
import { $servers, pb } from '@/lib/stores'
import { DataTable } from '../server-table/data-table'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { useStore } from '@nanostores/preact'
import { SystemRecord } from '@/types'

export function Home() {
	const servers = useStore($servers)
	// const [systems, setSystems] = useState([] as SystemRecord[])

	useEffect(() => {
		document.title = 'Home'
	}, [])

	useEffect(() => {
		console.log('servers', servers)
	}, [servers])

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
		return () => pb.collection('systems').unsubscribe('*')
	}, [])

	return (
		<>
			<Card>
				<CardHeader>
					<CardTitle className={'mb-3'}>All Servers</CardTitle>
					<CardDescription>
						Press{' '}
						<kbd className="pointer-events-none inline-flex h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground opacity-100">
							<span className="text-xs">⌘</span>K
						</kbd>{' '}
						to open the command palette.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<DataTable />
				</CardContent>
			</Card>
		</>
	)
}
