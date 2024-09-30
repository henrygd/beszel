import { toast } from '@/components/ui/use-toast'
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { $alerts, $copyContent, $systems, $userSettings, pb } from './stores'
import { AlertRecord, ChartTimeData, ChartTimes, SystemRecord } from '@/types'
import { RecordModel, RecordSubscription } from 'pocketbase'
import { WritableAtom } from 'nanostores'
import { timeDay, timeHour } from 'd3-time'
import { useEffect, useState } from 'react'

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs))
}

export async function copyToClipboard(content: string) {
	const duration = 1500
	try {
		await navigator.clipboard.writeText(content)
		toast({
			duration,
			description: 'Copied to clipboard',
		})
	} catch (e: any) {
		$copyContent.set(content)
	}
}

const verifyAuth = () => {
	pb.collection('users')
		.authRefresh()
		.catch(() => {
			pb.authStore.clear()
			toast({
				title: 'Failed to authenticate',
				description: 'Please log in again',
				variant: 'destructive',
			})
		})
}

export const updateSystemList = async () => {
	const records = await pb
		.collection<SystemRecord>('systems')
		.getFullList({ sort: '+name', fields: 'id,name,host,info,status' })
	if (records.length) {
		$systems.set(records)
	} else {
		verifyAuth()
	}
}

export const updateAlerts = () => {
	pb.collection('alerts')
		.getFullList<AlertRecord>({ fields: 'id,name,system,value' })
		.then((records) => {
			$alerts.set(records)
		})
}

const hourWithMinutesFormatter = new Intl.DateTimeFormat(undefined, {
	hour: 'numeric',
	minute: 'numeric',
})
export const hourWithMinutes = (timestamp: string) => {
	return hourWithMinutesFormatter.format(new Date(timestamp))
}

const shortDateFormatter = new Intl.DateTimeFormat(undefined, {
	day: 'numeric',
	month: 'short',
	hour: 'numeric',
	minute: 'numeric',
})
export const formatShortDate = (timestamp: string) => {
	// console.log('ts', timestamp)
	return shortDateFormatter.format(new Date(timestamp))
}

// const dayTimeFormatter = new Intl.DateTimeFormat(undefined, {
// 	// day: 'numeric',
// 	// month: 'short',
// 	hour: 'numeric',
// 	weekday: 'short',
// 	minute: 'numeric',
// 	// dateStyle: 'short',
// })
// export const formatDayTime = (timestamp: string) => {
// 	// console.log('ts', timestamp)
// 	return dayTimeFormatter.format(new Date(timestamp))
// }

const dayFormatter = new Intl.DateTimeFormat(undefined, {
	day: 'numeric',
	month: 'short',
	// dateStyle: 'medium',
})
export const formatDay = (timestamp: string) => {
	// console.log('ts', timestamp)
	return dayFormatter.format(new Date(timestamp))
}

export const updateFavicon = (newIcon: string) =>
	((document.querySelector("link[rel='icon']") as HTMLLinkElement).href = `/static/${newIcon}`)

export const isAdmin = () => pb.authStore.model?.role === 'admin'
export const isReadOnlyUser = () => pb.authStore.model?.role === 'readonly'
// export const isDefaultUser = () => pb.authStore.model?.role === 'user'

/** Update systems / alerts list when records change  */
export function updateRecordList<T extends RecordModel>(
	e: RecordSubscription<T>,
	$store: WritableAtom<T[]>
) {
	const curRecords = $store.get()
	const newRecords = []
	// console.log('e', e)
	if (e.action === 'delete') {
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

export function getPbTimestamp(timeString: ChartTimes) {
	const d = chartTimeData[timeString].getOffset(new Date())
	const year = d.getUTCFullYear()
	const month = String(d.getUTCMonth() + 1).padStart(2, '0')
	const day = String(d.getUTCDate()).padStart(2, '0')
	const hours = String(d.getUTCHours()).padStart(2, '0')
	const minutes = String(d.getUTCMinutes()).padStart(2, '0')
	const seconds = String(d.getUTCSeconds()).padStart(2, '0')

	return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

export const chartTimeData: ChartTimeData = {
	'1h': {
		type: '1m',
		expectedInterval: 60_000,
		label: '1 hour',
		// ticks: 12,
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -1),
	},
	'12h': {
		type: '10m',
		expectedInterval: 60_000 * 10,
		label: '12 hours',
		ticks: 12,
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -12),
	},
	'24h': {
		type: '20m',
		expectedInterval: 60_000 * 20,
		label: '24 hours',
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -24),
	},
	'1w': {
		type: '120m',
		expectedInterval: 60_000 * 120,
		label: '1 week',
		ticks: 7,
		format: (timestamp: string) => formatDay(timestamp),
		getOffset: (endTime: Date) => timeDay.offset(endTime, -7),
	},
	'30d': {
		type: '480m',
		expectedInterval: 60_000 * 480,
		label: '30 days',
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
			const div = document.createElement('div')
			div.className = 'text-xs tabular-nums tracking-tighter table sr-only'
			div.innerHTML = str
			clearTimeout(timeout)
			timeout = setTimeout(() => {
				document.body.appendChild(div)
				setYAxisWidth(div.offsetWidth + 24)
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

let twoDecimalFormatter: Intl.NumberFormat
/** Format number to two decimal places */
export function twoDecimalString(num: number) {
	if (!twoDecimalFormatter) {
		twoDecimalFormatter = new Intl.NumberFormat(undefined, {
			minimumFractionDigits: 2,
			maximumFractionDigits: 2,
		})
	}
	// Return a function that formats numbers using the saved formatter
	return twoDecimalFormatter.format(num)
}

/** Get value from local storage */
function getStorageValue(key: string, defaultValue: any) {
	const saved = localStorage?.getItem(key)
	return saved ? JSON.parse(saved) : defaultValue
}

/** Hook to sync value in local storage */
export const useLocalStorage = (key: string, defaultValue: any) => {
	key = `besz-${key}`
	const [value, setValue] = useState(() => {
		return getStorageValue(key, defaultValue)
	})
	useEffect(() => {
		localStorage?.setItem(key, JSON.stringify(value))
	}, [key, value])

	return [value, setValue]
}

export async function updateUserSettings() {
	try {
		const req = await pb.collection('user_settings').getFirstListItem('', { fields: 'settings' })
		$userSettings.set(req.settings)
		return
	} catch (e) {
		console.log('get settings', e)
	}
	// create user settings if error fetching existing
	try {
		const createdSettings = await pb
			.collection('user_settings')
			.create({ user: pb.authStore.model!.id })
		$userSettings.set(createdSettings.settings)
	} catch (e) {
		console.log('create settings', e)
	}
}

/**
 * Get the unit of size (TB or GB) for a given size in gigabytes
 * @param n size in gigabytes
 * @returns unit of size (TB or GB)
 */
export const getSizeUnit = (n: number) => (n >= 1_000 ? ' TB' : ' GB')

/**
 * Get the value of number in gigabytes if less than 1000, otherwise in terabytes
 * @param n size in gigabytes
 * @returns value in GB if less than 1000, otherwise value in TB
 */
export const getSizeVal = (n: number) => (n >= 1_000 ? n / 1_000 : n)
