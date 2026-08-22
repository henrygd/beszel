import "./index.css"
import { i18n } from "@lingui/core"
import { I18nProvider } from "@lingui/react"
import { useStore } from "@nanostores/react"
import { DirectionProvider } from "@radix-ui/react-direction"
import { AlertTriangleIcon, LoaderCircleIcon } from "lucide-react"
import { Trans } from "@lingui/react/macro"
// import { Suspense, lazy, useEffect, StrictMode } from "react"
import { lazy, memo, Suspense, useEffect } from "react"
import ReactDOM from "react-dom/client"
import Navbar from "@/components/navbar.tsx"
import { $router } from "@/components/router.tsx"
import Settings from "@/components/routes/settings/layout.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { Toaster } from "@/components/ui/toaster.tsx"
import { Button } from "@/components/ui/button.tsx"
import { alertManager } from "@/lib/alerts"
import { isAdmin, pb, updateUserSettings } from "@/lib/api.ts"
import { dynamicActivate, getLocale } from "@/lib/i18n"
import {
	$authenticated,
	$copyContent,
	$direction,
	$newVersion,
	$publicKey,
	$userSettings,
	defaultLayoutWidth,
} from "@/lib/stores.ts"
import * as systemsManager from "@/lib/systemsManager.ts"
import type { BeszelInfo, UpdateInfo } from "./types"

const LoginPage = lazy(() => import("@/components/login/login.tsx"))
const Home = lazy(() => import("@/components/routes/home.tsx"))
const Containers = lazy(() => import("@/components/routes/containers.tsx"))
const Smart = lazy(() => import("@/components/routes/smart.tsx"))
const SystemDetail = lazy(() => import("@/components/routes/system.tsx"))
const CopyToClipboardDialog = lazy(() => import("@/components/copy-to-clipboard.tsx"))

const App = memo(() => {
	const page = useStore($router)

	useEffect(() => {
		// change auth store on auth change
		const unsubscribeAuth = pb.authStore.onChange(() => {
			$authenticated.set(pb.authStore.isValid)
		})
		// get general info for authenticated users, such as public key and version
		pb.send<BeszelInfo>("/api/beszel/info", {})
			.then((data) => {
				$publicKey.set(data.key)
				// check for updates if enabled
				if (data.cu && isAdmin()) {
					pb.send<UpdateInfo>("/api/beszel/update", {})
						.then($newVersion.set)
						.catch(() => undefined)
				}
			})
			.catch(() => undefined)
		// get user settings
		updateUserSettings()
		// need to get system list before alerts
		systemsManager.init()
		systemsManager
			// get current systems list
			.refresh()
			// subscribe to new system updates
			.then(systemsManager.subscribe)
			// get current alerts
			.then(alertManager.refresh)
			// subscribe to new alert updates
			.then(alertManager.subscribe)
			.catch(() => undefined)
		return () => {
			unsubscribeAuth()
			alertManager.unsubscribe()
			systemsManager.unsubscribe()
		}
	}, [])

	if (!page) {
		return (
			<div className="grid min-h-[55vh] place-items-center text-center">
				<div className="max-w-md">
					<AlertTriangleIcon className="mx-auto size-9 text-amber-500" />
					<p className="mt-4 font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">404 · Unknown route</p>
					<h1 className="mt-2 text-2xl font-semibold tracking-tight">This console view does not exist.</h1>
					<Button className="mt-6" onClick={() => $router.open(globalThis.BESZEL?.BASE_PATH || "/")}>
						Return to fleet
					</Button>
				</div>
			</div>
		)
	} else if (page.route === "home") {
		return <Home />
	} else if (page.route === "system") {
		return <SystemDetail id={page.params.id} />
	} else if (page.route === "containers") {
		return <Containers />
	} else if (page.route === "smart") {
		return <Smart />
	} else if (page.route === "settings") {
		return <Settings />
	}
})

const Layout = () => {
	const authenticated = useStore($authenticated)
	const copyContent = useStore($copyContent)
	const direction = useStore($direction)
	const { layoutWidth } = useStore($userSettings, { keys: ["layoutWidth"] })

	useEffect(() => {
		document.documentElement.dir = direction
	}, [direction])

	return (
		<DirectionProvider dir={direction}>
			{!authenticated ? (
				<Suspense fallback={<AppLoading />}>
					<LoginPage />
				</Suspense>
			) : (
				<div style={{ "--container": `${layoutWidth ?? defaultLayoutWidth}px` } as React.CSSProperties}>
					<a className="skip-link" href="#main-content">
						<Trans>Skip to content</Trans>
					</a>
					<div className="container">
						<Navbar />
					</div>
					<main id="main-content" className="container relative">
						<Suspense fallback={<AppLoading />}>
							<App />
						</Suspense>
						{copyContent && (
							<Suspense>
								<CopyToClipboardDialog content={copyContent} />
							</Suspense>
						)}
					</main>
				</div>
			)}
		</DirectionProvider>
	)
}

const AppLoading = () => (
	<output className="grid min-h-52 place-items-center">
		<LoaderCircleIcon className="size-6 animate-spin text-primary" />
		<span className="sr-only">Loading</span>
	</output>
)

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

ReactDOM.createRoot(document.getElementById("app") as HTMLElement).render(
	// strict mode in dev mounts / unmounts components twice
	// and breaks the clipboard dialog
	//<StrictMode>
	<I18nApp />
	//</StrictMode>
)
