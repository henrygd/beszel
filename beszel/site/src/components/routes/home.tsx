import { Suspense, lazy, useEffect } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { $hubVersion } from '@/lib/stores'
import { useStore } from '@nanostores/react'
import { GithubIcon } from 'lucide-react'
import { Separator } from '../ui/separator'

const SystemsTable = lazy(() => import('../systems-table/systems-table'))

export default function () {
	const hubVersion = useStore($hubVersion)

	useEffect(() => {
		document.title = 'Dashboard / Beszel'
	}, [])

	return (
		<>
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
			{hubVersion && (
				<div className="flex gap-1.5 justify-end items-center pr-3 sm:pr-7 mt-3.5 text-xs opacity-80">
					<a
						href="https://github.com/henrygd/beszel"
						target="_blank"
						className="flex items-center gap-0.5 text-muted-foreground hover:text-foreground duration-75"
					>
						<GithubIcon className="h-3 w-3" /> GitHub
					</a>
					<Separator orientation="vertical" className="h-3 bg-muted-foreground" />
					<a
						href="https://github.com/henrygd/beszel/releases"
						target="_blank"
						className="text-muted-foreground hover:text-foreground duration-75"
					>
						Beszel {hubVersion}
					</a>
				</div>
			)}
		</>
	)
}
