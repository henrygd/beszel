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
import { updateUserSettings, updateAlerts, updateFavicon, updateSystemList } from './lib/utils.ts'
import { useStore } from '@nanostores/react'
import { Toaster } from './components/ui/toaster.tsx'
import { $router } from './components/router.tsx'
import SystemDetail from './components/routes/system.tsx'

import './lib/i18n.ts'
import { useTranslation } from 'react-i18next'
import Navbar from './components/navbar.tsx'

// const ServerDetail = lazy(() => import('./components/routes/system.tsx'))
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
		updateUserSettings()
		// get alerts after system list is loaded
		updateSystemList().then(updateAlerts)

		return () => updateFavicon('favicon.svg')
	}, [])

	// update favicon
	useEffect(() => {
		if (!systems.length || !authenticated) {
			updateFavicon('favicon.svg')
		} else {
			let up = false
			for (const system of systems) {
				if (system.status === 'down') {
					updateFavicon('favicon-red.svg')
					return
				} else if (system.status === 'up') {
					up = true
				}
			}
			updateFavicon(up ? 'favicon-green.svg' : 'favicon.svg')
		}
	}, [systems])

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
	const { t } = useTranslation()

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
			<div className="container">{Navbar(t)}</div>
			<div className="container mb-14 relative">
				<App />
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
