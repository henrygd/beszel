import { Suspense, lazy, useEffect } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'

const SystemsTable = lazy(() => import('../systems-table/systems-table'))

export default function () {
	useEffect(() => {
		document.title = 'Dashboard / Beszel'
	}, [])

	return (
		<Card>
			<CardHeader className="pb-2 md:pb-5 px-4 sm:px-7 max-sm:pt-5">
				<CardTitle className="mb-1.5">All Systems</CardTitle>
				<CardDescription>
					Updated in real time. Press{' '}
					<kbd className="pointer-events-none inline-flex h-5 select-none items-center gap-0.5 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground opacity-100">
						<span className="text-xs">⌘</span>K
					</kbd>{' '}
					to open the command palette.
				</CardDescription>
			</CardHeader>
			<CardContent className="max-sm:p-2">
				<Suspense>
					<SystemsTable />
				</Suspense>
			</CardContent>
		</Card>
	)
}
