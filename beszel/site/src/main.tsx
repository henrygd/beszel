import './index.css'
import { Suspense, lazy, useEffect, StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import Home from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import {
	$authenticated,
	$systems,
	pb,
	$publicKey,
	$hubVersion,
	$copyContent,
} from './lib/stores.ts'
import { ModeToggle } from './components/mode-toggle.tsx'
import {
	cn,
	updateUserSettings,
	isAdmin,
	isReadOnlyUser,
	updateAlerts,
	updateFavicon,
	updateSystemList,
} from './lib/utils.ts'
import { buttonVariants } from './components/ui/button.tsx'
import {
	DatabaseBackupIcon,
	LockKeyholeIcon,
	LogOutIcon,
	LogsIcon,
	ServerIcon,
	SettingsIcon,
	UserIcon,
	UsersIcon,
} from 'lucide-react'
import { useStore } from '@nanostores/react'
import { Toaster } from './components/ui/toaster.tsx'
import { Logo } from './components/logo.tsx'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
	DropdownMenuLabel,
} from './components/ui/dropdown-menu.tsx'
import { $router, Link } from './components/router.tsx'
import SystemDetail from './components/routes/system.tsx'
import { AddSystemButton } from './components/add-system.tsx'

// const ServerDetail = lazy(() => import('./components/routes/system.tsx'))
const CommandPalette = lazy(() => import('./components/command-palette.tsx'))
const LoginPage = lazy(() => import('./components/login/login.tsx'))
const CopyToClipboardDialog = lazy(() => import('./components/copy-to-clipboard.tsx'))
const Settings = lazy(() => import('./components/routes/settings/layout.tsx'))

const App = () => {
	const page = useStore($router)
	const authenticated = useStore($authenticated)
	const systems = useStore($systems)

	useEffect(() => {
		// change auth store on auth change
		pb.authStore.onChange(() => {
			$authenticated.set(pb.authStore.isValid)
		})
		// get version / public key
		pb.send('/api/beszel/getkey', {}).then((data) => {
			$publicKey.set(data.key)
			$hubVersion.set(data.v)
		})
		// get servers / alerts / settings
		updateSystemList()
		updateAlerts()
		updateUserSettings()
	}, [])

	// update favicon
	useEffect(() => {
		if (!authenticated || !systems.length) {
			updateFavicon('favicon.svg')
		} else {
			let up = false
			for (const system of systems) {
				if (system.status === 'down') {
					updateFavicon('favicon-red.svg')
					return () => updateFavicon('favicon.svg')
				} else if (system.status === 'up') {
					up = true
				}
			}
			updateFavicon(up ? 'favicon-green.svg' : 'favicon.svg')
			return () => updateFavicon('favicon.svg')
		}
		return () => {
			updateFavicon('favicon.svg')
		}
	}, [authenticated, systems])

	if (!page) {
		return <h1 className="text-3xl text-center my-14">404</h1>
	} else if (page.path === '/') {
		return <Home />
	} else if (page.route === 'server') {
		return <SystemDetail name={page.params.name} />
	} else if (page.route === 'settings') {
		return (
			<Suspense>
				<Settings />
			</Suspense>
		)
	}
}

const Layout = () => {
	const authenticated = useStore($authenticated)
	const copyContent = useStore($copyContent)

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
				<div className="flex items-center h-14 md:h-16 bg-card px-4 pr-3 sm:px-6 border bt-0 rounded-md my-4">
					<Link href="/" aria-label="Home" className={'p-2 pl-0'}>
						<Logo className="h-[1.15em] fill-foreground" />
					</Link>

					<div className={'flex ml-auto items-center'}>
						<ModeToggle />
						<Link
							href="/settings/general"
							aria-label="Settings"
							className={cn('', buttonVariants({ variant: 'ghost', size: 'icon' }))}
						>
							<SettingsIcon className="h-[1.2rem] w-[1.2rem]" />
						</Link>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<button
									aria-label="User Actions"
									className={cn('', buttonVariants({ variant: 'ghost', size: 'icon' }))}
								>
									<UserIcon className="h-[1.2rem] w-[1.2rem]" />
								</button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align={isReadOnlyUser() ? 'end' : 'center'} className="min-w-44">
								<DropdownMenuLabel>{pb.authStore.model?.email}</DropdownMenuLabel>
								<DropdownMenuSeparator />
								<DropdownMenuGroup>
									{isAdmin() && (
										<>
											<DropdownMenuItem asChild>
												<a href="/_/" target="_blank">
													<UsersIcon className="mr-2.5 h-4 w-4" />
													<span>Users</span>
												</a>
											</DropdownMenuItem>
											<DropdownMenuItem asChild>
												<a href="/_/#/collections?collectionId=2hz5ncl8tizk5nx" target="_blank">
													<ServerIcon className="mr-2.5 h-4 w-4" />
													<span>Systems</span>
												</a>
											</DropdownMenuItem>
											<DropdownMenuItem asChild>
												<a href="/_/#/logs" target="_blank">
													<LogsIcon className="mr-2.5 h-4 w-4" />
													<span>Logs</span>
												</a>
											</DropdownMenuItem>
											<DropdownMenuItem asChild>
												<a href="/_/#/settings/backups" target="_blank">
													<DatabaseBackupIcon className="mr-2.5 h-4 w-4" />
													<span>Backups</span>
												</a>
											</DropdownMenuItem>
											<DropdownMenuItem asChild>
												<a href="/_/#/settings/auth-providers" target="_blank">
													<LockKeyholeIcon className="mr-2.5 h-4 w-4" />
													<span>Auth providers</span>
												</a>
											</DropdownMenuItem>
											<DropdownMenuSeparator />
										</>
									)}
								</DropdownMenuGroup>
								<DropdownMenuItem onSelect={() => pb.authStore.clear()}>
									<LogOutIcon className="mr-2.5 h-4 w-4" />
									<span>Log out</span>
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
						<AddSystemButton className="ml-2" />
					</div>
				</div>
			</div>
			<div className="container mb-14 relative">
				<App />
				<Suspense>
					<CommandPalette />
				</Suspense>
				{copyContent && (
					<Suspense>
						<CopyToClipboardDialog content={copyContent} />
					</Suspense>
				)}
			</div>
		</>
	)
}

ReactDOM.createRoot(document.getElementById('app')!).render(
	// strict mode in dev mounts / unmounts components twice
	// and breaks the clipboard dialog
	//<StrictMode>
	<ThemeProvider>
		<Layout />
		<Toaster />
	</ThemeProvider>
	//</StrictMode>
)
