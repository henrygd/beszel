import { t } from "@lingui/core/macro"
import { toast } from "@/components/ui/use-toast"
import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"
import { $copyContent, $userSettings } from "./stores"
import type { ChartTimeData, FingerprintRecord, SemVer, SystemRecord } from "@/types"
import { timeDay, timeHour } from "d3-time"
import { useEffect, useState } from "react"
import { MeterState, Unit } from "./enums"
import { prependBasePath } from "@/components/router"

export const FAVICON_DEFAULT = "favicon.svg"
export const FAVICON_GREEN = "favicon-green.svg"
export const FAVICON_RED = "favicon-red.svg"

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs))
}

/** Adds event listener to node and returns function that removes the listener */
export function listen<T extends Event = Event>(node: Node, event: string, handler: (event: T) => void) {
	node.addEventListener(event, handler as EventListener)
	return () => node.removeEventListener(event, handler as EventListener)
}

export async function copyToClipboard(content: string) {
	const duration = 1500
	try {
		await navigator.clipboard.writeText(content)
		toast({
			duration,
			description: t`Copied to clipboard`,
		})
	} catch (e: any) {
		$copyContent.set(content)
	}
}

const hourWithMinutesFormatter = new Intl.DateTimeFormat(undefined, {
	hour: "numeric",
	minute: "numeric",
})
export const hourWithMinutes = (timestamp: string) => {
	return hourWithMinutesFormatter.format(new Date(timestamp))
}

const shortDateFormatter = new Intl.DateTimeFormat(undefined, {
	day: "numeric",
	month: "short",
	hour: "numeric",
	minute: "numeric",
})
export const formatShortDate = (timestamp: string) => {
	return shortDateFormatter.format(new Date(timestamp))
}

const dayFormatter = new Intl.DateTimeFormat(undefined, {
	day: "numeric",
	month: "short",
})
export const formatDay = (timestamp: string) => {
	return dayFormatter.format(new Date(timestamp))
}

export const updateFavicon = (newIcon: string) => {
	;(document.querySelector("link[rel='icon']") as HTMLLinkElement).href = prependBasePath(`/static/${newIcon}`)
}

export const chartTimeData: ChartTimeData = {
	"1h": {
		type: "1m",
		expectedInterval: 60_000,
		label: () => t`1 hour`,
		// ticks: 12,
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -1),
	},
	"12h": {
		type: "10m",
		expectedInterval: 60_000 * 10,
		label: () => t`12 hours`,
		ticks: 12,
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -12),
	},
	"24h": {
		type: "20m",
		expectedInterval: 60_000 * 20,
		label: () => t`24 hours`,
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -24),
	},
	"1w": {
		type: "120m",
		expectedInterval: 60_000 * 120,
		label: () => t`1 week`,
		ticks: 7,
		format: (timestamp: string) => formatDay(timestamp),
		getOffset: (endTime: Date) => timeDay.offset(endTime, -7),
	},
	"30d": {
		type: "480m",
		expectedInterval: 60_000 * 480,
		label: () => t`30 days`,
		ticks: 30,
		format: (timestamp: string) => formatDay(timestamp),
		getOffset: (endTime: Date) => timeDay.offset(endTime, -30),
	},
}

/** Sets the correct width of the y axis in recharts based on the longest label */
export function useYAxisWidth() {
	const [yAxisWidth, setYAxisWidth] = useState(0)
	let maxChars = 0
	let timeout: Timer
	function updateYAxisWidth(str: string) {
		if (str.length > maxChars) {
			maxChars = str.length
			const div = document.createElement("div")
			div.className = "text-xs tabular-nums tracking-tighter table sr-only"
			div.innerHTML = str
			clearTimeout(timeout)
			timeout = setTimeout(() => {
				document.body.appendChild(div)
				const width = div.offsetWidth + 24
				if (width > yAxisWidth) {
					setYAxisWidth(div.offsetWidth + 24)
				}
				document.body.removeChild(div)
			})
		}
		return str
	}
	return { yAxisWidth, updateYAxisWidth }
}

/** Format number to x decimal places, without trailing zeros */
export function toFixedFloat(num: number, digits: number) {
	return parseFloat((digits === 0 ? Math.ceil(num) : num).toFixed(digits))
}

let decimalFormatters: Map<number, Intl.NumberFormat> = new Map()
/** Format number to x decimal places, maintaining trailing zeros */
export function decimalString(num: number, digits = 2) {
	if (digits === 0) {
		return Math.ceil(num).toString()
	}
	let formatter = decimalFormatters.get(digits)
	if (!formatter) {
		formatter = new Intl.NumberFormat(undefined, {
			minimumFractionDigits: digits,
			maximumFractionDigits: digits,
		})
		decimalFormatters.set(digits, formatter)
	}
	return formatter.format(num)
}

/** Get value from local or session storage */
function getStorageValue(key: string, defaultValue: any, storageInterface: Storage = localStorage) {
	const saved = storageInterface?.getItem(key)
	return saved ? JSON.parse(saved) : defaultValue
}

/** Hook to sync value in local or session storage */
export function useBrowserStorage<T>(key: string, defaultValue: T, storageInterface: Storage = localStorage) {
	key = `besz-${key}`
	const [value, setValue] = useState(() => {
		return getStorageValue(key, defaultValue, storageInterface)
	})
	useEffect(() => {
		storageInterface?.setItem(key, JSON.stringify(value))
	}, [key, value])

	return [value, setValue]
}

/** Format temperature to user's preferred unit */
export function formatTemperature(celsius: number, unit?: Unit): { value: number; unit: string } {
	if (!unit) {
		unit = $userSettings.get().unitTemp || Unit.Celsius
	}
	// need loose equality check due to form data being strings
	if (unit == Unit.Fahrenheit) {
		return {
			value: celsius * 1.8 + 32,
			unit: "°F",
		}
	}
	return {
		value: celsius,
		unit: "°C",
	}
}

/** Format bytes to user's preferred unit */
export function formatBytes(
	size: number,
	perSecond = false,
	unit = Unit.Bytes,
	isMegabytes = false
): { value: number; unit: string } {
	// Convert MB to bytes if isMegabytes is true
	if (isMegabytes) size *= 1024 * 1024

	// need loose equality check due to form data being strings
	if (unit == Unit.Bits) {
		const bits = size * 8
		const suffix = perSecond ? "ps" : ""
		if (bits < 1000) return { value: bits, unit: `b${suffix}` }
		if (bits < 1_000_000) return { value: bits / 1_000, unit: `Kb${suffix}` }
		if (bits < 1_000_000_000)
			return {
				value: bits / 1_000_000,
				unit: `Mb${suffix}`,
			}
		if (bits < 1_000_000_000_000)
			return {
				value: bits / 1_000_000_000,
				unit: `Gb${suffix}`,
			}
		return {
			value: bits / 1_000_000_000_000,
			unit: `Tb${suffix}`,
		}
	}
	// bytes
	const suffix = perSecond ? "/s" : ""
	if (size < 100) return { value: size, unit: `B${suffix}` }
	if (size < 1000 * 1024) return { value: size / 1024, unit: `KB${suffix}` }
	if (size < 1000 * 1024 ** 2)
		return {
			value: size / 1024 ** 2,
			unit: `MB${suffix}`,
		}
	if (size < 1000 * 1024 ** 3)
		return {
			value: size / 1024 ** 3,
			unit: `GB${suffix}`,
		}
	return {
		value: size / 1024 ** 4,
		unit: `TB${suffix}`,
	}
}

export const chartMargin = { top: 12 }

/**
 * Retuns value of system host, truncating full path if socket.
 * @example
 * // Assuming system.host is "/var/run/beszel.sock"
 * const hostname = getHostDisplayValue(system) // hostname will be "beszel.sock"
 */
export const getHostDisplayValue = (system: SystemRecord): string => system.host.slice(system.host.lastIndexOf("/") + 1)

// export function formatUptimeString(uptimeSeconds: number): string {
// 	if (!uptimeSeconds || isNaN(uptimeSeconds)) return ""
// 	if (uptimeSeconds < 3600) {
// 		const minutes = Math.trunc(uptimeSeconds / 60)
// 		return plural({ minutes }, { one: "# minute", other: "# minutes" })
// 	} else if (uptimeSeconds < 172800) {
// 		const hours = Math.trunc(uptimeSeconds / 3600)
// 		console.log(hours)
// 		return plural({ hours }, { one: "# hour", other: "# hours" })
// 	} else {
// 		const days = Math.trunc(uptimeSeconds / 86400)
// 		return plural({ days }, { one: "# day", other: "# days" })
// 	}
// }

/** Generate a random token for the agent */
export const generateToken = () => {
	try {
		return crypto?.randomUUID()
	} catch (e) {
		return Array.from({ length: 2 }, () => (performance.now() * Math.random()).toString(16).replace(".", "-")).join("-")
	}
}

/** Get the hub URL from the global BESZEL object */
export const getHubURL = () => BESZEL?.HUB_URL || window.location.origin

/** Map of system IDs to their corresponding tokens (used to avoid fetching in add-system dialog) */
export const tokenMap = new Map<SystemRecord["id"], FingerprintRecord["token"]>()

/** Calculate duration between two dates and format as human-readable string */
export function formatDuration(
	createdDate: string | null | undefined,
	resolvedDate: string | null | undefined
): string {
	const created = createdDate ? new Date(createdDate) : null
	const resolved = resolvedDate ? new Date(resolvedDate) : null

	if (!created || !resolved) return ""

	const diffMs = resolved.getTime() - created.getTime()
	if (diffMs < 0) return ""

	const totalSeconds = Math.floor(diffMs / 1000)
	let hours = Math.floor(totalSeconds / 3600)
	let minutes = Math.floor((totalSeconds % 3600) / 60)
	let seconds = totalSeconds % 60

	// if seconds are close to 60, round up to next minute
	// if minutes are close to 60, round up to next hour
	if (seconds >= 58) {
		minutes += 1
		seconds = 0
	}
	if (minutes >= 60) {
		hours += 1
		minutes = 0
	}

	// For durations over 1 hour, omit seconds for cleaner display
	if (hours > 0) {
		return [hours ? `${hours}h` : null, minutes ? `${minutes}m` : null].filter(Boolean).join(" ")
	}

	return [hours ? `${hours}h` : null, minutes ? `${minutes}m` : null, seconds ? `${seconds}s` : null]
		.filter(Boolean)
		.join(" ")
}

export const parseSemVer = (semVer = ""): SemVer => {
	// if (semVer.startsWith("v")) {
	// 	semVer = semVer.slice(1)
	// }
	if (semVer.includes("-")) {
		semVer = semVer.slice(0, semVer.indexOf("-"))
	}
	const parts = semVer.split(".").map(Number)
	return { major: parts?.[0] ?? 0, minor: parts?.[1] ?? 0, patch: parts?.[2] ?? 0 }
}

/** Get meter state from 0-100 value. Used for color coding meters. */
export function getMeterState(value: number): MeterState {
	const { colorWarn = 65, colorCrit = 90 } = $userSettings.get()
	return value >= colorCrit ? MeterState.Crit : value >= colorWarn ? MeterState.Warn : MeterState.Good
}

export function debounce<T extends (...args: any[]) => any>(func: T, wait: number): (...args: Parameters<T>) => void {
	let timeout: ReturnType<typeof setTimeout>
	return (...args: Parameters<T>) => {
		clearTimeout(timeout)
		timeout = setTimeout(() => func(...args), wait)
	}
}

// Cache for runOnce
const runOnceCache = new WeakMap<Function, { done: boolean; result: unknown }>()
/** Run a function only once */
export function runOnce<T extends (...args: any[]) => any>(fn: T): T {
	return ((...args: Parameters<T>) => {
		let state = runOnceCache.get(fn)
		if (!state) {
			state = { done: false, result: undefined }
			runOnceCache.set(fn, state)
		}
		if (!state.done) {
			state.result = fn(...args)
			state.done = true
		}
		return state.result
	}) as T
}
