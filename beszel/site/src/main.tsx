import "./index.css"
// import { Suspense, lazy, useEffect, StrictMode } from "react"
import { Suspense, lazy, memo, useEffect } from "react"
import ReactDOM from "react-dom/client"
import { ThemeProvider } from "./components/theme-provider.tsx"
import { DirectionProvider } from "@radix-ui/react-direction"
import { $authenticated, $publicKey, $copyContent, $direction } from "./lib/stores.ts"
import { pb, updateUserSettings, updateCookieToken } from "./lib/api.ts"
import * as systemsManager from "./lib/systemsManager.ts"
import { useStore } from "@nanostores/react"
import { Toaster } from "./components/ui/toaster.tsx"
import { $router } from "./components/router.tsx"
import Navbar from "./components/navbar.tsx"
import { I18nProvider } from "@lingui/react"
import { i18n } from "@lingui/core"
import { getLocale, dynamicActivate } from "./lib/i18n"
import { alertManager } from "./lib/alerts"
import Settings from "./components/routes/settings/layout.tsx"

const LoginPage = lazy(() => import("@/components/login/login.tsx"))
const Home = lazy(() => import("@/components/routes/home.tsx"))
const SystemDetail = lazy(() => import("@/components/routes/system.tsx"))
const CopyToClipboardDialog = lazy(() => import("@/components/copy-to-clipboard.tsx"))

const App = memo(() => {
	const page = useStore($router)

	useEffect(() => {
		// change auth store on auth change
		updateCookieToken()
		pb.authStore.onChange(() => {
			$authenticated.set(pb.authStore.isValid)
			updateCookieToken()
		})
		// get version / public key
		pb.send("/api/beszel/getkey", {}).then((data) => {
			$publicKey.set(data.key)
		})
		// get user settings
		updateUserSettings()
		const startingSystems = globalThis.BESZEL.SYSTEMS
		for (const system of startingSystems) {
			// if (typeof system.info === "string") {
			system.info = JSON.parse(system.info as unknown as string)
			// }
		}
		// need to get system list before alerts
		systemsManager.init()
		systemsManager
			// get current systems list
			.refresh(startingSystems)
			// subscribe to new system updates
			.then(systemsManager.subscribe)
			// get current alerts
			.then(alertManager.refresh)
			// subscribe to new alert updates
			.then(alertManager.subscribe)
		return () => {
			// updateFavicon("favicon.svg")
			alertManager.unsubscribe()
			systemsManager.unsubscribe()
			globalThis.BESZEL.SYSTEMS = []
		}
	}, [])

	if (!page) {
		return <h1 className="text-3xl text-center my-14">404</h1>
	} else if (page.route === "home") {
		return <Home />
	} else if (page.route === "system") {
		return <SystemDetail name={page.params.name} />
	} else if (page.route === "settings") {
		return <Settings />
	}
})

const Layout = () => {
	const authenticated = useStore($authenticated)
	const copyContent = useStore($copyContent)
	const direction = useStore($direction)

	useEffect(() => {
		document.documentElement.dir = direction
	}, [direction])

	return (
		<DirectionProvider dir={direction}>
			{!authenticated ? (
				<Suspense>
					<LoginPage />
				</Suspense>
			) : (
				<>
					<div className="container">
						<Navbar />
					</div>
					<div className="container relative">
						<App />
						{copyContent && (
							<Suspense>
								<CopyToClipboardDialog content={copyContent} />
							</Suspense>
						)}
					</div>
				</>
			)}
		</DirectionProvider>
	)
}

const I18nApp = () => {
	useEffect(() => {
		dynamicActivate(getLocale())
	}, [])

	return (
		<I18nProvider i18n={i18n}>
			<ThemeProvider>
				<Layout />
				<Toaster />
			</ThemeProvider>
		</I18nProvider>
	)
}

ReactDOM.createRoot(document.getElementById("app")!).render(
	// strict mode in dev mounts / unmounts components twice
	// and breaks the clipboard dialog
	//<StrictMode>
	<I18nApp />
	//</StrictMode>
)
