import './index.css'
import { render } from 'preact'
import { Route, Switch } from 'wouter-preact'
import { Home } from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import LoginPage from './components/login.tsx'
import { $authenticated, $servers, pb } from './lib/stores.ts'
import { ServerDetail } from './components/routes/server.tsx'
import { ModeToggle } from './components/mode-toggle.tsx'
import { CommandPalette } from './components/command-dialog.tsx'
import { cn } from './lib/utils.ts'
import { buttonVariants } from './components/ui/button.tsx'
import { Github } from 'lucide-react'
import { useStore } from '@nanostores/preact'
import { useEffect } from 'preact/hooks'
import { SystemRecord } from './types'

const App = () => {
	const authenticated = useStore($authenticated)

	return <ThemeProvider>{authenticated ? <Main /> : <LoginPage />}</ThemeProvider>
}

const Main = () => {
	// const servers = useStore($servers)

	useEffect(() => {
		console.log('fetching servers')
		pb.collection<SystemRecord>('systems')
			.getFullList({ sort: '+name' })
			.then((records) => {
				$servers.set(records)
			})
	}, [])

	return (
		<div className="container mt-7 mb-14">
			<div class="flex mb-4">
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
		</div>
	)
}
render(<App />, document.getElementById('app')!)
