import './index.css'
import React, { Suspense, lazy, useEffect } from 'react'
import ReactDOM from 'react-dom/client'
import Home from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import { $authenticated, $router, $servers, navigate } from './lib/stores.ts'
import { ModeToggle } from './components/mode-toggle.tsx'
import { cn, updateFavicon, updateServerList } from './lib/utils.ts'
import { buttonVariants } from './components/ui/button.tsx'
import { Github } from 'lucide-react'
import { useStore } from '@nanostores/react'
import { Toaster } from './components/ui/toaster.tsx'
import { Logo } from './components/logo.tsx'
import {
	TooltipProvider,
	Tooltip,
	TooltipTrigger,
	TooltipContent,
} from '@/components/ui/tooltip.tsx'

const ServerDetail = lazy(() => import('./components/routes/server.tsx'))
const CommandPalette = lazy(() => import('./components/command-palette.tsx'))
const LoginPage = lazy(() => import('./components/login.tsx'))

const App = () => {
	const page = useStore($router)
	const authenticated = useStore($authenticated)
	const servers = useStore($servers)

	// get servers
	useEffect(updateServerList, [])

	// update favicon
	useEffect(() => {
		if (!authenticated || !servers.length) {
			updateFavicon('/favicon.svg')
		} else {
			let up = false
			for (const server of servers) {
				if (server.status === 'down') {
					updateFavicon('/favicon-red.svg')
					return
				} else if (server.status === 'up') {
					up = true
				}
			}
			updateFavicon(up ? '/favicon-green.svg' : '/favicon.svg')
		}
	}, [authenticated, servers])

	if (!page) {
		return <h1>404</h1>
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
		return <LoginPage />
	}

	return (
		<>
			<div className="container">
				<div className="flex items-center h-16 bg-card px-6 border bt-0 rounded-md my-5">
					<a
						href="/"
						aria-label="Home"
						className={'p-2 pl-0 -mb-1'}
						onClick={(e) => {
							e.preventDefault()
							navigate('/')
						}}
					>
						<Logo className="h-[1.2em] fill-foreground" />
					</a>

					<div className={'flex gap-1 ml-auto'}>
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
						<ModeToggle />
					</div>
				</div>
			</div>
			<div className="container mb-14 relative">
				<App />
				<CommandPalette />
				<Toaster />
			</div>
		</>
	)
}

ReactDOM.createRoot(document.getElementById('app')!).render(
	<React.StrictMode>
		<ThemeProvider>
			<Layout />
		</ThemeProvider>
	</React.StrictMode>
)
