import './index.css'
import { render } from 'preact'
import { Link, Route, Switch } from 'wouter-preact'
import { Home } from './components/routes/home.tsx'
import { ThemeProvider } from './components/theme-provider.tsx'
import LoginPage from './components/login.tsx'
import { pb } from './lib/stores.ts'
import { ServerDetail } from './components/routes/server.tsx'

// import { ModeToggle } from './components/mode-toggle.tsx'

// const ls = localStorage.getItem('auth')
// console.log('ls', ls)
// @ts-ignore
pb.authStore.storageKey = 'pb_admin_auth'

console.log('pb.authStore', pb.authStore)

const App = () => <ThemeProvider>{pb.authStore.isValid ? <Main /> : <LoginPage />}</ThemeProvider>

const Main = () => (
	<div className="container">
		<nav class="flex gap-5 bg-white/10 p-4 rounded-md mb-3">
			<Link href="/">Home</Link>
			<Link href="/server/kagemusha">kagemusha</Link>
			<Link href="/server/rashomon">rashomon</Link>
			{/* <ModeToggle /> */}
		</nav>

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
	</div>
)

render(<App />, document.getElementById('app')!)
