import { t } from "@lingui/core/macro"
import { toast } from "@/components/ui/use-toast"
import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"
import { $alerts, $copyContent, $systems, $userSettings, pb } from "./stores"
import { AlertInfo, AlertRecord, ChartTimeData, ChartTimes, FingerprintRecord, SystemRecord, TemperatureUnit, TemperatureConversion, SpeedUnit, SpeedConversion } from "@/types"
import { RecordModel, RecordSubscription } from "pocketbase"
import { WritableAtom } from "nanostores"
import { timeDay, timeHour } from "d3-time"
import { useEffect, useState } from "react"
import { CpuIcon, HardDriveIcon, MemoryStickIcon, ServerIcon } from "lucide-react"
import { EthernetIcon, HourglassIcon, ThermometerIcon } from "@/components/ui/icons"
import { prependBasePath } from "@/components/router"

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

const verifyAuth = () => {
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

export const updateSystemList = (() => {
	let isFetchingSystems = false
	return async () => {
		if (isFetchingSystems) {
			return
		}
		isFetchingSystems = true
		try {
			const records = await pb
				.collection<SystemRecord>("systems")
				.getFullList({ sort: "+name", fields: "id,name,host,port,info,status" })

			if (records.length) {
				$systems.set(records)
			} else {
				verifyAuth()
			}
		} finally {
			isFetchingSystems = false
		}
	}
})()

/** Logs the user out by clearing the auth store and unsubscribing from realtime updates. */
export async function logOut() {
	sessionStorage.setItem("lo", "t")
	pb.authStore.clear()
	pb.realtime.unsubscribe()
}

export const updateAlerts = () => {
	pb.collection("alerts")
		.getFullList<AlertRecord>({ fields: "id,name,system,value,min,triggered", sort: "updated" })
		.then((records) => {
			$alerts.set(records)
		})
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

export const isAdmin = () => pb.authStore.record?.role === "admin"
export const isReadOnlyUser = () => pb.authStore.record?.role === "readonly"

/** Update systems / alerts list when records change  */
export function updateRecordList<T extends RecordModel>(e: RecordSubscription<T>, $store: WritableAtom<T[]>) {
	const curRecords = $store.get()
	const newRecords = []
	if (e.action === "delete") {
		for (const server of curRecords) {
			if (server.id !== e.record.id) {
				newRecords.push(server)
			}
		}
	} else {
		let found = 0
		for (const server of curRecords) {
			if (server.id === e.record.id) {
				found = newRecords.push(e.record)
			} else {
				newRecords.push(server)
			}
		}
		if (!found) {
			newRecords.push(e.record)
		}
	}
	$store.set(newRecords)
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

export function toFixedWithoutTrailingZeros(num: number, digits: number) {
	return parseFloat(num.toFixed(digits)).toString()
}

export function toFixedFloat(num: number, digits: number) {
	return parseFloat(num.toFixed(digits))
}

let decimalFormatters: Map<number, Intl.NumberFormat> = new Map()
/** Format number to x decimal places */
export function decimalString(num: number, digits = 2) {
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

/** Get value from local storage */
function getStorageValue(key: string, defaultValue: any) {
	const saved = localStorage?.getItem(key)
	return saved ? JSON.parse(saved) : defaultValue
}

/** Hook to sync value in local storage */
export function useLocalStorage<T>(key: string, defaultValue: T) {
	key = `besz-${key}`
	const [value, setValue] = useState(() => {
		return getStorageValue(key, defaultValue)
	})
	useEffect(() => {
		localStorage?.setItem(key, JSON.stringify(value))
	}, [key, value])

	return [value, setValue]
}

/** Convert temperature from Celsius to the specified unit */
export function convertTemperature(
	celsius: number,
	unit: TemperatureUnit = "celsius"
): TemperatureConversion {
	switch (unit) {
		case "fahrenheit":
			return { value: (celsius * 9) / 5 + 32, symbol: "°F" }
		default:
			return { value: celsius, symbol: "°C" }
	}
}

/** Convert network speed from MB/s to the specified unit  */
export function convertNetworkSpeed(
	mbps: number,
	unit: SpeedUnit = "mbps"
): SpeedConversion {
	switch (unit) {
		case "bps": {
			const bps = mbps * 8 * 1_000_000 // Convert MB/s to bits per second

			// Format large numbers appropriately
			if (bps >= 1_000_000_000) {
				return {
					value: bps / 1_000_000_000,
					symbol: " Gbps",
					display: `${decimalString(bps / 1_000_000_000, bps >= 10_000_000_000 ? 0 : 1)} Gbps`,
				}
			} else if (bps >= 1_000_000) {
				return {
					value: bps / 1_000_000,
					symbol: " Mbps",
					display: `${decimalString(bps / 1_000_000, bps >= 10_000_000 ? 0 : 1)} Mbps`,
				}
			} else if (bps >= 1_000) {
				return {
					value: bps / 1_000,
					symbol: " Kbps",
					display: `${decimalString(bps / 1_000, bps >= 10_000 ? 0 : 1)} Kbps`,
				}
			} else {
				return {
					value: bps,
					symbol: " bps",
					display: `${Math.round(bps)} bps`,
				}
			}
		}
		default:
			return {
				value: mbps,
				symbol: " MB/s",
				display: `${decimalString(mbps, mbps >= 100 ? 1 : 2)} MB/s`,
			}
	}
}

/** Convert disk speed from MB/s to the specified unit  */
export function convertDiskSpeed(
	mbps: number,
	unit: SpeedUnit = "mbps"
): SpeedConversion {
	switch (unit) {
		case "bps": {
			const bps = mbps * 8 * 1_000_000 // Convert MB/s to bits per second

			// Format large numbers appropriately
			if (bps >= 1_000_000_000) {
				return {
					value: bps / 1_000_000_000,
					symbol: " Gbps",
					display: `${decimalString(bps / 1_000_000_000, bps >= 10_000_000_000 ? 0 : 1)} Gbps`,
				}
			} else if (bps >= 1_000_000) {
				return {
					value: bps / 1_000_000,
					symbol: " Mbps",
					display: `${decimalString(bps / 1_000_000, bps >= 10_000_000 ? 0 : 1)} Mbps`,
				}
			} else if (bps >= 1_000) {
				return {
					value: bps / 1_000,
					symbol: " Kbps",
					display: `${decimalString(bps / 1_000, bps >= 10_000 ? 0 : 1)} Kbps`,
				}
			} else {
				return {
					value: bps,
					symbol: " bps",
					display: `${Math.round(bps)} bps`,
				}
			}
		}
		default:
			return {
				value: mbps,
				symbol: " MB/s",
				display: `${decimalString(mbps, mbps >= 100 ? 1 : 2)} MB/s`,
			}
	}
}

export async function updateUserSettings() {
	try {
		const req = await pb.collection("user_settings").getFirstListItem("", { fields: "settings" })
		$userSettings.set(req.settings)
		return
	} catch (e) {
		console.log("get settings", e)
	}
	// create user settings if error fetching existing
	try {
		const createdSettings = await pb.collection("user_settings").create({ user: pb.authStore.record!.id })
		$userSettings.set(createdSettings.settings)
	} catch (e) {
		console.log("create settings", e)
	}
}

/**
 * Get the value and unit of size (TB, GB, or MB) for a given size
 * @param n size in gigabytes or megabytes
 * @param isGigabytes boolean indicating if n represents gigabytes (true) or megabytes (false)
 * @returns an object containing the value and unit of size
 */
export const getSizeAndUnit = (n: number, isGigabytes = true) => {
	const sizeInGB = isGigabytes ? n : n / 1_000

	if (sizeInGB >= 1_000) {
		return { v: sizeInGB / 1_000, u: " TB" }
	} else if (sizeInGB >= 1) {
		return { v: sizeInGB, u: " GB" }
	}
	return { v: isGigabytes ? sizeInGB * 1_000 : n, u: " MB" }
}

export const chartMargin = { top: 12 }

export const alertInfo: Record<string, AlertInfo> = {
	Status: {
		name: () => t`Status`,
		unit: "",
		icon: ServerIcon,
		desc: () => t`Triggers when status switches between up and down`,
		/** "for x minutes" is appended to desc when only one value */
		singleDesc: () => t`System` + " " + t`Down`,
	},
	CPU: {
		name: () => t`CPU Usage`,
		unit: "%",
		icon: CpuIcon,
		desc: () => t`Triggers when CPU usage exceeds a threshold`,
	},
	Memory: {
		name: () => t`Memory Usage`,
		unit: "%",
		icon: MemoryStickIcon,
		desc: () => t`Triggers when memory usage exceeds a threshold`,
	},
	Disk: {
		name: () => t`Disk Usage`,
		unit: "%",
		icon: HardDriveIcon,
		desc: () => t`Triggers when usage of any disk exceeds a threshold`,
	},
	Bandwidth: {
		name: () => t`Bandwidth`,
		unit: " MB/s",
		icon: EthernetIcon,
		desc: () => t`Triggers when combined up/down exceeds a threshold`,
		max: 125,
	},
	Temperature: {
		name: () => t`Temperature`,
		unit: "°C",
		icon: ThermometerIcon,
		desc: () => t`Triggers when any sensor exceeds a threshold`,
	},
	LoadAvg5: {
		name: () => t`Load Average 5m`,
		unit: "",
		icon: HourglassIcon,
		max: 100,
		min: 0.1,
		start: 10,
		step: 0.1,
		desc: () => t`Triggers when 5 minute load average exceeds a threshold`,
	},
	LoadAvg15: {
		name: () => t`Load Average 15m`,
		unit: "",
		icon: HourglassIcon,
		min: 0.1,
		max: 100,
		start: 10,
		step: 0.1,
		desc: () => t`Triggers when 15 minute load average exceeds a threshold`,
	},
}

/**
 * Retuns value of system host, truncating full path if socket.
 * @example
 * // Assuming system.host is "/var/run/beszel.sock"
 * const hostname = getHostDisplayValue(system) // hostname will be "beszel.sock"
 */
export const getHostDisplayValue = (system: SystemRecord): string => system.host.slice(system.host.lastIndexOf("/") + 1)

/** Generate a random token for the agent */
export const generateToken = () => crypto?.randomUUID() ?? (performance.now() * Math.random()).toString(16)

/** Get the hub URL from the global BESZEL object */
export const getHubURL = () => BESZEL?.HUB_URL || window.location.origin

/** Map of system IDs to their corresponding tokens (used to avoid fetching in add-system dialog) */
export const tokenMap = new Map<SystemRecord["id"], FingerprintRecord["token"]>()
