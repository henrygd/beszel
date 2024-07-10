import './index.css'
import React, { useEffect } from 'react'
import ReactDOM from 'react-dom/client'
import { Route, Switch } from 'wouter'
import { Home } from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import LoginPage from './components/login.tsx'
import { $authenticated } from './lib/stores.ts'
import { ServerDetail } from './components/routes/server.tsx'
import { ModeToggle } from './components/mode-toggle.tsx'
import { CommandPalette } from './components/command-palette.tsx'
import { cn, updateServerList } from './lib/utils.ts'
import { buttonVariants } from './components/ui/button.tsx'
import { Github } from 'lucide-react'
import { useStore } from '@nanostores/react'
import { Toaster } from './components/ui/toaster.tsx'

const App = () => {
	const authenticated = useStore($authenticated)

	return <ThemeProvider>{authenticated ? <Main /> : <LoginPage />}</ThemeProvider>
}

const Main = () => {
	// get servers
	useEffect(updateServerList, [])

	return (
		<div className="container mt-7 mb-14">
			<div className="flex mb-4">
				{/* <Link
				className={cn('', buttonVariants({ variant: 'ghost', size: 'icon' }))}
				href="/"
				title={'All servers'}
			>
				<HomeIcon className="h-[1.2rem] w-[1.2rem]" />
			</Link> */}
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

			{/* 
      Routes below are matched exclusively -
      the first matched route gets rendered
    */}
			<Switch>
				<Route path="/" component={Home} />

				<Route path="/server/:name" component={ServerDetail}></Route>

				{/* Default route in a switch */}
				<Route>404: No such page!</Route>
			</Switch>
			<CommandPalette />
			<Toaster />
		</div>
	)
}

ReactDOM.createRoot(document.getElementById('app')!).render(
	<React.StrictMode>
		<App />
	</React.StrictMode>
)
