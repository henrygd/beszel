import { t } from "@lingui/core/macro"
import PocketBase from "pocketbase"
import { basePath } from "@/components/router"
import { toast } from "@/components/ui/use-toast"
import type { ChartTimes, UserSettings } from "@/types"
import { $alerts, $allSystemsById, $allSystemsByName, $userSettings } from "./stores"
import { chartTimeData, THEME_STORAGE_KEY } from "./utils"

/** PocketBase JS Client */
export const pb = new PocketBase(basePath)

export const isAdmin = () => pb.authStore.record?.role === "admin"
export const isReadOnlyUser = () => pb.authStore.record?.role === "readonly"

export const verifyAuth = () => {
	pb.collection("users")
		.authRefresh()
		.catch(() => {
			logOut()
			toast({
				title: t`Failed to authenticate`,
				description: t`Please log in again`,
				variant: "destructive",
			})
		})
}

/** Logs the user out by clearing the auth store and unsubscribing from realtime updates. */
export function logOut() {
	$allSystemsByName.set({})
	$allSystemsById.set({})
	$alerts.set({})
	$userSettings.set({} as UserSettings)
	sessionStorage.setItem("lo", "t") // prevent auto login on logout
	pb.authStore.clear()
	pb.realtime.unsubscribe()
}

/** Fetch or create user settings in database */
export async function updateUserSettings() {
	try {
		const req = await pb.collection("user_settings").getFirstListItem("", { fields: "settings" })
		$userSettings.set(req.settings)
		backfillTheme(req.settings)
		return
	} catch (e) {
		console.error("get settings", e)
	}
	// create user settings if error fetching existing
	try {
		const createdSettings = await pb.collection("user_settings").create({ user: pb.authStore.record?.id })
		$userSettings.set(createdSettings.settings)
		backfillTheme(createdSettings.settings)
	} catch (e) {
		console.error("create settings", e)
	}
}

/** Save a partial update to user settings in database */
export async function saveUserSettings(newSettings: Partial<UserSettings>) {
	// get fresh copy of settings so concurrent changes aren't overwritten
	const req = await pb.collection("user_settings").getFirstListItem("", { fields: "id,settings" })
	const updatedSettings = await pb.collection("user_settings").update(req.id, {
		settings: {
			...req.settings,
			...newSettings,
		},
	})
	$userSettings.set(updatedSettings.settings)
}

/** Users who set a theme before it was stored in the database only have it in
 *  local storage, so seed the database from it once to avoid losing their choice. */
function backfillTheme(settings: UserSettings) {
	if (settings?.theme) {
		return
	}
	const localTheme = localStorage.getItem(THEME_STORAGE_KEY) as UserSettings["theme"]
	if (localTheme && localTheme !== "system") {
		saveUserSettings({ theme: localTheme }).catch((e) => console.error("backfill theme", e))
	}
}

export function getPbTimestamp(timeString: ChartTimes, d?: Date) {
	d ||= chartTimeData[timeString].getOffset(new Date())
	const year = d.getUTCFullYear()
	const month = String(d.getUTCMonth() + 1).padStart(2, "0")
	const day = String(d.getUTCDate()).padStart(2, "0")
	const hours = String(d.getUTCHours()).padStart(2, "0")
	const minutes = String(d.getUTCMinutes()).padStart(2, "0")
	const seconds = String(d.getUTCSeconds()).padStart(2, "0")

	return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}
