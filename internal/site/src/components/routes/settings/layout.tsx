import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath, redirectPage } from "@nanostores/router"
import {
	AlertOctagonIcon,
	BellIcon,
	FileSlidersIcon,
	FingerprintIcon,
	HeartPulseIcon,
	SettingsIcon,
} from "lucide-react"
import { lazy, useEffect } from "react"
import { $router } from "@/components/router.tsx"
import { Card } from "@/components/ui/card.tsx"
import { toast } from "@/components/ui/use-toast.ts"
import { pb } from "@/lib/api"
import { $userSettings } from "@/lib/stores.ts"
import type { UserSettings } from "@/types"
import { SidebarNav } from "./sidebar-nav.tsx"

const generalSettingsImport = () => import("./general.tsx")
const notificationsSettingsImport = () => import("./notifications.tsx")
const configYamlSettingsImport = () => import("./config-yaml.tsx")
const fingerprintsSettingsImport = () => import("./tokens-fingerprints.tsx")
const alertsHistoryDataTableSettingsImport = () => import("./alerts-history-data-table.tsx")
const heartbeatSettingsImport = () => import("./heartbeat.tsx")

const GeneralSettings = lazy(generalSettingsImport)
const NotificationsSettings = lazy(notificationsSettingsImport)
const ConfigYamlSettings = lazy(configYamlSettingsImport)
const FingerprintsSettings = lazy(fingerprintsSettingsImport)
const AlertsHistoryDataTableSettings = lazy(alertsHistoryDataTableSettingsImport)
const HeartbeatSettings = lazy(heartbeatSettingsImport)

const validSettingsPages = new Set(["general", "notifications", "config", "tokens", "alert-history", "heartbeat"])

export async function saveSettings(newSettings: Partial<UserSettings>) {
	try {
		// get fresh copy of settings
		const req = await pb.collection("user_settings").getFirstListItem("", {
			fields: "id,settings",
		})
		// update user settings
		const updatedSettings = await pb.collection("user_settings").update(req.id, {
			settings: {
				...req.settings,
				...newSettings,
			},
		})
		$userSettings.set(updatedSettings.settings)
		toast({
			title: t`Settings saved`,
			description: t`Your user settings have been updated.`,
		})
	} catch (e) {
		// console.error('update settings', e)
		toast({
			title: t`Failed to save settings`,
			description: t`Check logs for more details.`,
			variant: "destructive",
		})
	}
}

export default function SettingsLayout() {
	const { t } = useLingui()

	const sidebarNavItems = [
		{
			title: t({ message: `General`, comment: "Context: General settings" }),
			href: getPagePath($router, "settings", { name: "general" }),
			icon: SettingsIcon,
		},
		{
			title: t`Notifications`,
			href: getPagePath($router, "settings", { name: "notifications" }),
			icon: BellIcon,
			preload: notificationsSettingsImport,
		},
		{
			title: t`Tokens & Fingerprints`,
			href: getPagePath($router, "settings", { name: "tokens" }),
			icon: FingerprintIcon,
			noReadOnly: true,
			preload: fingerprintsSettingsImport,
		},
		{
			title: t`Alert History`,
			href: getPagePath($router, "settings", { name: "alert-history" }),
			icon: AlertOctagonIcon,
			preload: alertsHistoryDataTableSettingsImport,
		},
		{
			title: t`Heartbeat`,
			href: getPagePath($router, "settings", { name: "heartbeat" }),
			icon: HeartPulseIcon,
			admin: true,
			preload: heartbeatSettingsImport,
		},
		{
			title: t`YAML Config`,
			href: getPagePath($router, "settings", { name: "config" }),
			icon: FileSlidersIcon,
			admin: true,
			preload: configYamlSettingsImport,
		},
	]

	const page = useStore($router)
	const pageParams = page?.params as { name?: unknown } | undefined
	const pageName = typeof pageParams?.name === "string" ? pageParams.name : undefined

	useEffect(() => {
		document.title = `${t`Settings`} / Beszel`
		if (!pageName || !validSettingsPages.has(pageName)) {
			redirectPage($router, "settings", { name: "general" })
		}
	}, [pageName, t])

	return (
		<div className="mb-14">
			<div className="mb-5">
				<h1 className="text-2xl font-semibold tracking-tight">
					<Trans>Settings</Trans>
				</h1>
				<p className="mt-1 text-sm text-muted-foreground">
					<Trans>Manage display and notification preferences.</Trans>
				</p>
			</div>
			<div className="flex flex-col gap-3.5 md:flex-row md:gap-8 lg:gap-10">
				<aside className="md:w-52 md:shrink-0">
					<SidebarNav items={sidebarNavItems} />
				</aside>
				<Card className="min-w-0 flex-1 p-5 sm:p-6">
					<SettingsContent name={pageName ?? "general"} />
				</Card>
			</div>
		</div>
	)
}

function SettingsContent({ name }: { name: string }) {
	const userSettings = useStore($userSettings)

	switch (name) {
		case "general":
			return <GeneralSettings userSettings={userSettings} />
		case "notifications":
			return <NotificationsSettings userSettings={userSettings} />
		case "config":
			return <ConfigYamlSettings />
		case "tokens":
			return <FingerprintsSettings />
		case "alert-history":
			return <AlertsHistoryDataTableSettings />
		case "heartbeat":
			return <HeartbeatSettings />
		default:
			return <GeneralSettings userSettings={userSettings} />
	}
}
