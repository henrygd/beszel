import './index.css'
import React, { Suspense, lazy, useEffect } from 'react'
import ReactDOM from 'react-dom/client'
import Home from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import { $alerts, $authenticated, $router, $systems, navigate, pb } from './lib/stores.ts'
import { ModeToggle } from './components/mode-toggle.tsx'
import {
	cn,
	isAdmin,
	updateAlerts,
	updateFavicon,
	updateRecordList,
	updateServerList,
} from './lib/utils.ts'
import { buttonVariants } from './components/ui/button.tsx'
import { DatabaseBackupIcon, Github, LogOutIcon, LogsIcon, UserIcon } from 'lucide-react'
import { useStore } from '@nanostores/react'
import { Toaster } from './components/ui/toaster.tsx'
import { Logo } from './components/logo.tsx'
import {
	TooltipProvider,
	Tooltip,
	TooltipTrigger,
	TooltipContent,
} from '@/components/ui/tooltip.tsx'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from './components/ui/dropdown-menu.tsx'
import { AlertRecord, SystemRecord } from './types'

const ServerDetail = lazy(() => import('./components/routes/server.tsx'))
const CommandPalette = lazy(() => import('./components/command-palette.tsx'))
const LoginPage = lazy(() => import('./components/login.tsx'))

const App = () => {
	const page = useStore($router)
	const authenticated = useStore($authenticated)
	const servers = useStore($systems)

	useEffect(() => {
		// change auth store on auth change
		pb.authStore.onChange(() => {
			$authenticated.set(pb.authStore.isValid)
		})
		// get servers / alerts
		updateServerList()
		updateAlerts()
		// subscribe to real time updates for systems / alerts
		pb.collection<SystemRecord>('systems').subscribe('*', (e) => {
			updateRecordList(e, $systems)
		})
		pb.collection<AlertRecord>('alerts').subscribe('*', (e) => {
			updateRecordList(e, $alerts)
		})
		return () => {
			pb.collection('systems').unsubscribe('*')
			pb.collection('alerts').unsubscribe('*')
		}
	}, [])

	// update favicon
	useEffect(() => {
		if (!authenticated || !servers.length) {
			updateFavicon('/favicon.svg')
		} else {
			let up = false
			for (const server of servers) {
				if (server.status === 'down') {
					updateFavicon('/favicon-red.svg')
					return () => updateFavicon('/favicon.svg')
				} else if (server.status === 'up') {
					up = true
				}
			}
			updateFavicon(up ? '/favicon-green.svg' : '/favicon.svg')
			return () => updateFavicon('/favicon.svg')
		}
		return () => {
			updateFavicon('/favicon.svg')
		}
	}, [authenticated, servers])

	if (!page) {
		return <h1 className="text-3xl text-center my-14">404</h1>
	} else if (page.path === '/') {
		return <Home />
	} else if (page.route === 'server') {
		return (
			<Suspense>
				<ServerDetail name={page.params.name} />
			</Suspense>
		)
	}
}

const Layout = () => {
	const authenticated = useStore($authenticated)

	if (!authenticated) {
		return (
			<Suspense>
				<LoginPage />
			</Suspense>
		)
	}

	return (
		<>
			<div className="container">
				<div className="flex items-center h-16 bg-card px-6 border bt-0 rounded-md my-5">
					<a
						href="/"
						aria-label="Home"
						className={'p-2 pl-0'}
						onClick={(e) => {
							e.preventDefault()
							navigate('/')
						}}
					>
						<Logo className="h-[1.2em] fill-foreground" />
					</a>

					<div className={'flex ml-auto'}>
						<ModeToggle />
						<TooltipProvider delayDuration={300}>
							<Tooltip>
								<TooltipTrigger asChild>
									<a
										title={'Github'}
										aria-label="Github repo"
										href={'https://github.com/henrygd'}
										className={cn('', buttonVariants({ variant: 'ghost', size: 'icon' }))}
									>
										<Github className="h-[1.2rem] w-[1.2rem]" />
									</a>
								</TooltipTrigger>
								<TooltipContent>
									<p>Github Repository</p>
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<a
									aria-label="User Actions"
									href={'https://github.com/henrygd'}
									className={cn('', buttonVariants({ variant: 'ghost', size: 'icon' }))}
								>
									<UserIcon className="h-[1.2rem] w-[1.2rem]" />
								</a>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem onSelect={() => pb.authStore.clear()}>
									<LogOutIcon className="mr-2.5 h-4 w-4" />
									<span>Log out</span>
								</DropdownMenuItem>
								{isAdmin() && (
									<>
										<DropdownMenuSeparator />
										<DropdownMenuItem asChild>
											<a href="/_/#/logs">
												<LogsIcon className="mr-2.5 h-4 w-4" />
												<span>Logs</span>
											</a>
										</DropdownMenuItem>
										<DropdownMenuItem asChild>
											<a href="/_/#/settings/backups">
												<DatabaseBackupIcon className="mr-2.5 h-4 w-4" />
												<span>Backups</span>
											</a>
										</DropdownMenuItem>
									</>
								)}
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</div>
			<div className="container mb-14 relative">
				<App />
				<Suspense>
					<CommandPalette />
				</Suspense>
			</div>
		</>
	)
}

ReactDOM.createRoot(document.getElementById('app')!).render(
	<React.StrictMode>
		<ThemeProvider>
			<Layout />
			<Toaster />
		</ThemeProvider>
	</React.StrictMode>
)
