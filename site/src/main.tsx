import './index.css'
import { render } from 'preact'
import { Link, Route, Switch } from 'wouter-preact'
import { Home } from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import LoginPage from './components/login.tsx'
import { pb } from './lib/stores.ts'
import { ServerDetail } from './components/routes/server.tsx'
import { ModeToggle } from './components/mode-toggle.tsx'
import { CommandPalette } from './components/command-dialog.tsx'

// import { ModeToggle } from './components/mode-toggle.tsx'

// const ls = localStorage.getItem('auth')
// console.log('ls', ls)
// @ts-ignore
pb.authStore.storageKey = 'pb_admin_auth'

console.log('pb.authStore', pb.authStore)

const App = () => <ThemeProvider>{pb.authStore.isValid ? <Main /> : <LoginPage />}</ThemeProvider>

const Main = () => (
	<div className="container mt-7 mb-14">
		<div class="flex mb-4 justify-end">
			<ModeToggle />
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

render(<App />, document.getElementById('app')!)
