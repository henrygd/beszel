import PocketBase from "pocketbase"
import { atom, map, WritableAtom } from "nanostores"
import { AlertRecord, ChartTimes, SystemRecord, UserSettings } from "@/types"
import { basePath } from "@/components/router"

/** PocketBase JS Client */
export const pb = new PocketBase(basePath)

/** Store if user is authenticated */
export const $authenticated = atom(pb.authStore.isValid)

/** List of system records */
export const $systems = atom([] as SystemRecord[])

/** List of alert records */
export const $alerts = atom([] as AlertRecord[])

/** SSH public key */
export const $publicKey = atom("")

/** Beszel hub version */
export const $hubVersion = atom("")

/** Chart time period */
export const $chartTime = atom("1h") as WritableAtom<ChartTimes>

/** User settings */
export const $userSettings = map<UserSettings>({
	chartTime: "1h",
	emails: [pb.authStore.record?.email || ""],
})
// update local storage on change
$userSettings.subscribe((value) => {
	// console.log('user settings changed', value)
	$chartTime.set(value.chartTime)
})

/** Container chart filter */
export const $containerFilter = atom("")

/** Fallback copy to clipboard dialog content */
export const $copyContent = atom("")

/** Direction for localization */
export const $direction = atom<"ltr" | "rtl">("ltr")
