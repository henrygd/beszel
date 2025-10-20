import "./index.css"
import { i18n } from "@lingui/core"
import { I18nProvider } from "@lingui/react"
import { useStore } from "@nanostores/react"
import { DirectionProvider } from "@radix-ui/react-direction"
// import { Suspense, lazy, useEffect, StrictMode } from "react"
import { lazy, memo, Suspense, useEffect } from "react"
import ReactDOM from "react-dom/client"
import { AppSidebar } from "./components/app-sidebar.tsx"
import { Breadcrumbs } from "./components/breadcrumbs.tsx"
import { $router } from "@/components/router.tsx"
import Settings from "@/components/routes/settings/layout.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { Toaster } from "@/components/ui/toaster.tsx"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "./components/ui/sidebar.tsx"
import { Separator } from "./components/ui/separator.tsx"
import { Breadcrumb, BreadcrumbList } from "./components/ui/breadcrumb.tsx"
import { alertManager } from "@/lib/alerts"
import { pb, updateUserSettings } from "@/lib/api.ts"
import { dynamicActivate, getLocale } from "@/lib/i18n"
import { $authenticated, $copyContent, $direction, $publicKey } from "@/lib/stores.ts"
import * as systemsManager from "@/lib/systemsManager.ts"

const LoginPage = lazy(() => import("@/components/login/login.tsx"))
const Home = lazy(() => import("@/components/routes/home.tsx"))
const Containers = lazy(() => import("@/components/routes/containers.tsx"))
const SystemDetail = lazy(() => import("@/components/routes/system.tsx"))
const GeneralPage = lazy(() => import("@/components/routes/general.tsx"))
const NotificationsPage = lazy(() => import("@/components/routes/notifications.tsx"))
const TokensPage = lazy(() => import("@/components/routes/tokens.tsx"))
const AlertHistoryPage = lazy(() => import("@/components/routes/alert-history.tsx"))
const YamlConfigPage = lazy(() => import("@/components/routes/yaml-config.tsx"))
const CopyToClipboardDialog = lazy(() => import("@/components/copy-to-clipboard.tsx"))

const App = memo(() => {
	const page = useStore($router)

	useEffect(() => {
		// change auth store on auth change
		pb.authStore.onChange(() => {
			$authenticated.set(pb.authStore.isValid)
		})
		// get version / public key
		pb.send("/api/beszel/getkey", {}).then((data) => {
			$publicKey.set(data.key)
		})
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
		return () => {
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
	} else if (page.route === "containers") {
		return <Containers />
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
			default:
				return <Settings />
		}
	} else if (page.route === "application") {
		// Handle individual application pages
		switch (page.params.name) {
			case "config":
				return <YamlConfigPage />
			default:
				return <h1 className="text-3xl text-center my-14">404</h1>
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

	// biome-ignore lint/correctness/useExhaustiveDependencies: only run on mount
	useEffect(() => {
		// refresh auth if not authenticated (required for trusted auth header)
		if (!authenticated) {
			pb.collection("users")
				.authRefresh()
				.then((res) => {
					pb.authStore.save(res.token, res.record)
					$authenticated.set(!!pb.authStore.isValid)
				})
		}
	}, [])

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

ReactDOM.createRoot(document.getElementById("app") as HTMLElement).render(
	// strict mode in dev mounts / unmounts components twice
	// and breaks the clipboard dialog
	//<StrictMode>
	<I18nApp />
	//</StrictMode>
)
