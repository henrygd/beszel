import "./index.css"
import { i18n } from "@lingui/core"
import { I18nProvider } from "@lingui/react"
import { useStore } from "@nanostores/react"
import { DirectionProvider } from "@radix-ui/react-direction"
// import { Suspense, lazy, useEffect, StrictMode } from "react"
import { lazy, memo, Suspense, useEffect } from "react"
import ReactDOM from "react-dom/client"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import Navbar from "@/components/navbar.tsx"
import { $router } from "@/components/router.tsx"
import Settings from "@/components/routes/settings/layout.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { Toaster } from "@/components/ui/toaster.tsx"
import { alertManager } from "@/lib/alerts"
import { pb, updateUserSettings } from "@/lib/api.ts"
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
const Monitors = lazy(() => import("@/components/routes/monitors.tsx"))
const SystemDetail = lazy(() => import("@/components/routes/system.tsx"))
const DomainDetail = lazy(() => import("@/components/routes/domain.tsx"))
const MonitorDetail = lazy(() => import("@/components/routes/monitor.tsx"))
const StatusPages = lazy(() => import("@/components/routes/status-pages.tsx"))
const PublicStatusPage = lazy(() => import("@/components/routes/public-status-page.tsx"))
const Incidents = lazy(() => import("@/components/routes/incidents.tsx"))
const Calendar = lazy(() => import("@/components/routes/calendar.tsx"))
const Monitoring = lazy(() => import("@/components/routes/monitoring.tsx"))
const CopyToClipboardDialog = lazy(() => import("@/components/copy-to-clipboard.tsx"))
const ActiveAlerts = lazy(() => import("@/components/active-alerts.tsx").then((m) => ({ default: m.ActiveAlerts })))

const App = memo(() => {
	const page = useStore($router)

	useEffect(() => {
		// change auth store on auth change
		const unsubscribeAuth = pb.authStore.onChange(() => {
			$authenticated.set(pb.authStore.isValid)
		})
		// get general info for authenticated users, such as public key and version
		if (pb.authStore.isValid) {
			pb.send<BeszelInfo>("/api/beszel/info", {}).then((data) => {
				$publicKey.set(data.key)
				// check for updates if enabled
				if (data.cu) {
					pb.send<UpdateInfo>("/api/beszel/update", {}).then($newVersion.set)
				}
			})
			// get user settings
			updateUserSettings()
		}
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
		return () => {
			unsubscribeAuth()
			alertManager.unsubscribe()
			systemsManager.unsubscribe()
		}
	}, [])

	if (!page) {
		return <h1 className="text-3xl text-center my-14">404</h1>
	} else if (page.route === "home") {
		return <Home />
	} else if (page.route === "system") {
		return <SystemDetail id={page.params.id} />
	} else if (page.route === "domain") {
		return <DomainDetail id={page.params.id} />
	} else if (page.route === "monitor") {
		return <MonitorDetail id={page.params.id} />
	} else if (page.route === "containers") {
		return <Containers />
	} else if (page.route === "smart") {
		return <Smart />
	} else if (page.route === "monitors") {
		return <Monitors />
	} else if (page.route === "settings") {
		return <Settings />
	} else if (page.route === "status_pages") {
		return <StatusPages />
	} else if (page.route === "public_status") {
		return <PublicStatusPage slug={page.params.slug} />
	} else if (page.route === "incidents") {
		return <Incidents />
	} else if (page.route === "calendar") {
		return <Calendar />
	} else if (page.route === "monitoring") {
		return <Monitoring />
	}
})

const Layout = () => {
	const authenticated = useStore($authenticated)
	const copyContent = useStore($copyContent)
	const direction = useStore($direction)
	const { layoutWidth } = useStore($userSettings, { keys: ["layoutWidth"] })
	const page = useStore($router)

	useEffect(() => {
		document.documentElement.dir = direction
	}, [direction])

	// Public status page doesn't require authentication
	const isPublicStatusPage = page?.route === "public_status"

	return (
		<DirectionProvider dir={direction}>
			{!authenticated && !isPublicStatusPage ? (
				<Suspense>
					<LoginPage />
				</Suspense>
			) : isPublicStatusPage ? (
				// Public status page renders without navbar/layout
				<Suspense>
					<App />
				</Suspense>
			) : (
				<div style={{ "--container": `${layoutWidth ?? defaultLayoutWidth}px` } as React.CSSProperties}>
					<div className="container">
						<Navbar />
					</div>
					<div className="container relative">
						<Suspense>
							<ActiveAlerts className="mb-4" />
						</Suspense>
						<App />
						{copyContent && (
							<Suspense>
								<CopyToClipboardDialog content={copyContent} />
							</Suspense>
						)}
					</div>
				</div>
			)}
		</DirectionProvider>
	)
}

// Create QueryClient instance
const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			staleTime: 60 * 1000,
			refetchOnWindowFocus: false,
		},
	},
})

const I18nApp = () => {
	useEffect(() => {
		// Activate a locale so I18nProvider can mount App and load the account settings.
		dynamicActivate(getLocale())
	}, [])

	return (
		<I18nProvider i18n={i18n}>
			<ThemeProvider>
				<QueryClientProvider client={queryClient}>
					<Layout />
					<Toaster />
				</QueryClientProvider>
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
