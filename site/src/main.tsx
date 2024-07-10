import './index.css'
import React, { Suspense, lazy, useEffect } from 'react'
import ReactDOM from 'react-dom/client'
import Home from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
// import LoginPage from './components/login.tsx'
import { $authenticated, $router } from './lib/stores.ts'
import { ModeToggle } from './components/mode-toggle.tsx'
// import { CommandPalette } from './components/command-palette.tsx'
import { cn, updateServerList } from './lib/utils.ts'
import { buttonVariants } from './components/ui/button.tsx'
import { Github } from 'lucide-react'
import { useStore } from '@nanostores/react'
import { Toaster } from './components/ui/toaster.tsx'

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
		<div className="container mt-7 mb-14 relative">
			<div className="flex mb-4">
				{/* <a
				className={cn('', buttonVariants({ variant: 'ghost', size: 'icon' }))}
				href="/"
				title={'All servers'}
			>
				<HomeIcon className="h-[1.2rem] w-[1.2rem]" />
			</a> */}
				<div className={'flex gap-1 ml-auto'}>
					<a
						title={'Github'}
						href={'https://github.com/henrygd'}
						className={cn('', buttonVariants({ variant: 'ghost', size: 'icon' }))}
					>
						<Github className="h-[1.2rem] w-[1.2rem]" />
					</a>
					<ModeToggle />
				</div>
			</div>

			<App />
			<CommandPalette />
			<Toaster />
		</div>
	)
}

ReactDOM.createRoot(document.getElementById('app')!).render(
	<React.StrictMode>
		<ThemeProvider>
			<Layout />
		</ThemeProvider>
	</React.StrictMode>
)
