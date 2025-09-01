import { atom, map } from "nanostores"
import { AlertMap, ChartTimes, SystemRecord, UserSettings } from "@/types"
import { Unit } from "./enums"
import { pb } from "./api"

/** Store if user is authenticated */
export const $authenticated = atom(pb.authStore.isValid)

/** List of system records */
export const $systems = atom<SystemRecord[]>([])

/** Map of alert records by system id and alert name */
export const $alerts = map<AlertMap>({})

/** SSH public key */
export const $publicKey = atom("")

/** Chart time period */
export const $chartTime = atom<ChartTimes>("1h")

/** Whether to display average or max chart values */
export const $maxValues = atom(false)

// export const UserSettingsSchema = v.object({
// 	chartTime: v.picklist(["1h", "12h", "24h", "1w", "30d"]),
// 	emails: v.optional(v.array(v.pipe(v.string(), v.email())), [pb?.authStore?.record?.email ?? ""]),
// 	webhooks: v.optional(v.array(v.string())),
// 	colorWarn: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(100))),
// 	colorDanger: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(100))),
// 	unitTemp: v.optional(v.enum(Unit)),
// 	unitNet: v.optional(v.enum(Unit)),
// 	unitDisk: v.optional(v.enum(Unit)),
// })

/** User settings */
export const $userSettings = map<UserSettings>({
	chartTime: "1h",
	emails: [pb.authStore.record?.email || ""],
	unitNet: Unit.Bytes,
	unitTemp: Unit.Celsius,
})
// update chart time on change
$userSettings.subscribe((value) => $chartTime.set(value.chartTime))

/** Container chart filter */
export const $containerFilter = atom("")

/** Temperature chart filter */
export const $temperatureFilter = atom("")

/** Fallback copy to clipboard dialog content */
export const $copyContent = atom("")

/** Direction for localization */
export const $direction = atom<"ltr" | "rtl">("ltr")

/** Longest system name length. Used to set table column width. I know this
 *  is stupid but the table is virtualized and I know this will work.
 */
export const $longestSystemNameLen = atom(8)
