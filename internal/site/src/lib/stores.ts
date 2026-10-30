import { atom, computed, map, onMount, type ReadableAtom } from "nanostores"
import type { AlertMap, ChartTimes, SeriesLevel, SystemRecord, UpdateInfo, UserSettings } from "@/types"
import { pb } from "./api"
import { DEFAULT_SERIES_LEVEL, isSeriesLevel } from "./container-series"
import { Unit } from "./enums"

/** Default layout width. Used as fallback when user setting is unset. */
export const defaultLayoutWidth = 1600

/** Store if user is authenticated */
export const $authenticated = atom(pb.authStore.isValid)

/** Map of system records by name */
export const $allSystemsByName = map<Record<string, SystemRecord>>({})
/** Map of system records by id */
export const $allSystemsById = map<Record<string, SystemRecord>>({})
/** Map of up systems by id */
export const $upSystems = map<Record<string, SystemRecord>>({})
/** Map of down systems by id */
export const $downSystems = map<Record<string, SystemRecord>>({})
/** Map of paused systems by id */
export const $pausedSystems = map<Record<string, SystemRecord>>({})
/** List of all system records */
export const $systems: ReadableAtom<SystemRecord[]> = computed($allSystemsById, Object.values)
/** Whether the initial systems list has been fetched */
export const $systemsLoaded = atom(false)

/** Map of alert records by system id and alert name */
export const $alerts = map<AlertMap>({})

/** SSH public key */
export const $publicKey = atom("")

/** New version info if an update is available, otherwise undefined */
export const $newVersion = atom<UpdateInfo | undefined>()

/** Chart time period used when user settings don't provide one */
export const defaultChartTime: ChartTimes = "1h"

/** Chart time period */
export const $chartTime = atom<ChartTimes>(defaultChartTime)

/** Whether to display average or max chart values */
export const $maxValues = atom(false)

const logTimestampsKey = "besz-log-ts"

/** Whether to show timestamps in Docker and systemd logs */
export const $showLogTimestamps = atom<boolean>(JSON.parse(localStorage.getItem(logTimestampsKey) ?? "true"))

export function toggleLogTimestamps() {
	const next = !$showLogTimestamps.get()
	$showLogTimestamps.set(next)
	localStorage.setItem(logTimestampsKey, JSON.stringify(next))
}

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
	chartTime: defaultChartTime,
	emails: [pb.authStore.record?.email || ""],
	unitNet: Unit.Bytes,
	unitTemp: Unit.Celsius,
})

/** Chart time period stored in user settings, or the default if it's missing */
export function getUserChartTime(settings: UserSettings = $userSettings.get()): ChartTimes {
	return settings.chartTime || defaultChartTime
}

/**
 * Apply settings loaded from the database, including the default chart time.
 * Other settings writes don't touch $chartTime so they can't reset the active chart range.
 */
export function hydrateUserSettings(settings: UserSettings) {
	$userSettings.set(settings)
	$chartTime.set(getUserChartTime(settings))
}

/** Container chart filter */
export const $containerFilter = atom("")

const seriesLevelKey = "besz-series-level"

/** Stored series level, or the default when nothing valid is stored */
function storedSeriesLevel(): SeriesLevel {
	try {
		const saved = JSON.parse(localStorage.getItem(seriesLevelKey) ?? "null")
		return isSeriesLevel(saved) ? saved : DEFAULT_SERIES_LEVEL
	} catch {
		// anything can end up under that key; a bad value must not take down the page
		return DEFAULT_SERIES_LEVEL
	}
}

/**
 * Level the container charts group their series by: containers, services or stacks.
 *
 * Kept in localStorage like the grouping toggle of the container table, not in the user
 * settings: it is a per-browser view preference and would mean a write to the database on every
 * click. The stored value is read when the first component subscribes, so importing this module
 * costs nothing and does not depend on web storage being present.
 */
export const $containerSeriesLevel = atom<SeriesLevel>(DEFAULT_SERIES_LEVEL)

onMount($containerSeriesLevel, () => {
	const stored = storedSeriesLevel()
	if (stored !== $containerSeriesLevel.get()) {
		$containerSeriesLevel.set(stored)
	}
})

/**
 * Switch the series level and remember it.
 *
 * The filter is cleared along with it: its terms are matched against series keys, and those mean
 * something else on every level, so a filter typed for container names would blank out every
 * stack series.
 */
export function setContainerSeriesLevel(level: SeriesLevel) {
	if ($containerSeriesLevel.get() === level) {
		return
	}
	$containerSeriesLevel.set(level)
	$containerFilter.set("")
	try {
		localStorage.setItem(seriesLevelKey, JSON.stringify(level))
	} catch {
		// private mode or blocked storage: the level still applies for this session
	}
}

/** Temperature chart filter */
export const $temperatureFilter = atom("")

/** Filter for network monitor charts (compare page and per-system monitor charts) */
export const $monitorFilter = atom("")

/** Fan-speed chart filter */
export const $fanFilter = atom("")

/** Fallback copy to clipboard dialog content */
export const $copyContent = atom("")

/** Direction for localization */
export const $direction = atom<"ltr" | "rtl">("ltr")
