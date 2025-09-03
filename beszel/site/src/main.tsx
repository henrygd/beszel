import "./index.css"
// import { Suspense, lazy, useEffect, StrictMode } from "react"
import { Suspense, lazy, memo, useEffect } from "react"
import ReactDOM from "react-dom/client"
import { ThemeProvider } from "./components/theme-provider.tsx"
import { DirectionProvider } from "@radix-ui/react-direction"
import { $authenticated, $systems, $publicKey, $copyContent, $direction } from "./lib/stores.ts"
import { pb, updateSystemList, updateUserSettings } from "./lib/api.ts"
import { useStore } from "@nanostores/react"
import { Toaster } from "./components/ui/toaster.tsx"
import { $router } from "./components/router.tsx"
import { updateFavicon } from "@/lib/utils"
import { AppSidebar } from "./components/app-sidebar.tsx"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "./components/ui/sidebar.tsx"
import { Breadcrumbs } from "./components/breadcrumbs.tsx"
import { Separator } from "./components/ui/separator.tsx"
import { Breadcrumb, BreadcrumbList } from "./components/ui/breadcrumb.tsx"
import { I18nProvider } from "@lingui/react"
import { i18n } from "@lingui/core"
import { getLocale, dynamicActivate } from "./lib/i18n"
import { SystemStatus } from "./lib/enums"
import { alertManager } from "./lib/alerts"
import Settings from "./components/routes/settings/layout.tsx"

const LoginPage = lazy(() => import("@/components/login/login.tsx"))
const Home = lazy(() => import("@/components/routes/home.tsx"))
const SystemDetail = lazy(() => import("@/components/routes/system.tsx"))
const GeneralPage = lazy(() => import("@/components/routes/general.tsx"))
const NotificationsPage = lazy(() => import("@/components/routes/notifications.tsx"))
const TokensPage = lazy(() => import("@/components/routes/tokens.tsx"))
const AlertHistoryPage = lazy(() => import("@/components/routes/alert-history.tsx"))
const YamlConfigPage = lazy(() => import("@/components/routes/yaml-config.tsx"))
const CopyToClipboardDialog = lazy(() => import("@/components/copy-to-clipboard.tsx"))

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
		// need to get system list before alerts
		updateSystemList()
			// get alerts
			.then(alertManager.refresh)
			// subscribe to new alert updates
			.then(alertManager.subscribe)

		return () => {
			updateFavicon("favicon.svg")
			alertManager.unsubscribe()
		}
	}, [])

	// update favicon
	useEffect(() => {
		if (!systems.length || !authenticated) {
			updateFavicon("favicon.svg")
		} else {
			let up = false
			for (const system of systems) {
				if (system.status === SystemStatus.Down) {
					updateFavicon("favicon-red.svg")
					return
				}
				if (system.status === SystemStatus.Up) {
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
		// Handle individual settings pages
		switch (page.params.name) {
			case "general":
				return <GeneralPage />
			case "notifications":
				return <NotificationsPage />
			case "tokens":
				return <TokensPage />
			case "alert-history":
				return <AlertHistoryPage />
			case "config":
				return <YamlConfigPage />
			default:
				return <Settings />
		}
	}
})

const Layout = () => {
	const authenticated = useStore($authenticated)
	const copyContent = useStore($copyContent)
	const direction = useStore($direction)
	const page = useStore($router)

	useEffect(() => {
		document.documentElement.dir = direction
	}, [direction])

	const getContentContainerClass = () => {
		if (!page) return "w-full max-w-none"
		
		// All pages use full width for better space utilization
		return "w-full max-w-none"
	}

	return (
		<DirectionProvider dir={direction}>
			{!authenticated ? (
				<Suspense>
					<LoginPage />
				</Suspense>
			) : (
				<SidebarProvider>
					<AppSidebar />
					<SidebarInset>
						<header className="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12">
							<div className="flex items-center gap-2 px-4">
								<SidebarTrigger className="-ml-1" />
								<Separator
									orientation="vertical"
									className="mr-2 data-[orientation=vertical]:h-4"
								/>
								<Breadcrumb>
									<BreadcrumbList>
										<Breadcrumbs />
									</BreadcrumbList>
								</Breadcrumb>
							</div>
						</header>
						<div className="flex flex-1 flex-col gap-4 p-6">
							<div className={getContentContainerClass()}>
								<App />
							</div>
							{copyContent && (
								<Suspense>
									<CopyToClipboardDialog content={copyContent} />
								</Suspense>
							)}
						</div>
					</SidebarInset>
				</SidebarProvider>
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
