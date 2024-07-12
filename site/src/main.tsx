import './index.css'
import React, { Suspense, lazy, useEffect } from 'react'
import ReactDOM from 'react-dom/client'
import Home from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import { $authenticated, $router, navigate } from './lib/stores.ts'
import { ModeToggle } from './components/mode-toggle.tsx'
import { cn, updateServerList } from './lib/utils.ts'
import { buttonVariants } from './components/ui/button.tsx'
import { Github } from 'lucide-react'
import { useStore } from '@nanostores/react'
import { Toaster } from './components/ui/toaster.tsx'
import { Can, Logo } from './components/logo.tsx'
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

	// get servers
	useEffect(updateServerList, [])

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
				<div className="flex items-center py-3.5 bg-card px-6 border bt-0 rounded-md my-5">
					<TooltipProvider delayDuration={300}>
						<Tooltip>
							<TooltipTrigger asChild>
								<a
									href="/"
									aria-label="Home"
									onClick={(e) => {
										e.preventDefault()
										navigate('/')
									}}
								>
									<Logo className="h-5 fill-foreground" />
								</a>
							</TooltipTrigger>
							<TooltipContent>
								<p>Home</p>
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
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
