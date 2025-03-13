import "./index.css"
// import { Suspense, lazy, useEffect, StrictMode } from "react"
import { Suspense, lazy, memo, useEffect } from "react"
import ReactDOM from "react-dom/client"
import { Home } from "./components/routes/home.tsx"
import { ThemeProvider } from "./components/theme-provider.tsx"
import { DirectionProvider } from "@radix-ui/react-direction"
import { $authenticated, $systems, pb, $publicKey, $copyContent, $direction } from "./lib/stores.ts"
import { updateUserSettings, updateAlerts, updateFavicon, updateSystemList } from "./lib/utils.ts"
import { useStore } from "@nanostores/react"
import { Toaster } from "./components/ui/toaster.tsx"
import { $router } from "./components/router.tsx"
import SystemDetail from "./components/routes/system.tsx"
import Navbar from "./components/navbar.tsx"
import { I18nProvider } from "@lingui/react"
import { i18n } from "@lingui/core"
import "@/lib/i18n.ts"

// const ServerDetail = lazy(() => import('./components/routes/system.tsx'))
const LoginPage = lazy(() => import("./components/login/login.tsx"))
const CopyToClipboardDialog = lazy(() => import("./components/copy-to-clipboard.tsx"))
const Settings = lazy(() => import("./components/routes/settings/layout.tsx"))

const App = memo(() => {
	const page = useStore($router)
	const authenticated = useStore($authenticated)
	const systems = useStore($systems)

	useEffect(() => {
		// change auth store on auth change
		pb.authStore.onChange(() => {
			$authenticated.set(pb.authStore.isValid)
		})
		// get version / public key
		pb.send("/api/beszel/getkey", {}).then((data) => {
			$publicKey.set(data.key)
		})
		// get servers / alerts / settings
		updateUserSettings()
		// get alerts after system list is loaded
		updateSystemList().then(updateAlerts)

		return () => updateFavicon("favicon.svg")
	}, [])

	// update favicon
	useEffect(() => {
		if (!systems.length || !authenticated) {
			updateFavicon("favicon.svg")
		} else {
			let up = false
			for (const system of systems) {
				if (system.status === "down") {
					updateFavicon("favicon-red.svg")
					return
				} else if (system.status === "up") {
					up = true
				}
			}
			updateFavicon(up ? "favicon-green.svg" : "favicon.svg")
		}
	}, [systems])

	if (!page) {
		return <h1 className="text-3xl text-center my-14">404</h1>
	} else if (page.route === "home") {
		return <Home />
	} else if (page.route === "system") {
		return <SystemDetail name={page.params.name} />
	} else if (page.route === "settings") {
		return (
			<Suspense>
				<Settings />
			</Suspense>
		)
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
					<div className="container mb-14 relative">
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

ReactDOM.createRoot(document.getElementById("app")!).render(
	// strict mode in dev mounts / unmounts components twice
	// and breaks the clipboard dialog
	//<StrictMode>
	<I18nProvider i18n={i18n}>
		<ThemeProvider>
			<Layout />
			<Toaster />
		</ThemeProvider>
	</I18nProvider>
	//</StrictMode>
)
