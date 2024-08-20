import { toast } from '@/components/ui/use-toast'
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { $alerts, $systems, pb } from './stores'
import { AlertRecord, ChartTimeData, ChartTimes, SystemRecord } from '@/types'
import { RecordModel, RecordSubscription } from 'pocketbase'
import { WritableAtom } from 'nanostores'
import { timeDay, timeHour } from 'd3-time'
import { useEffect, useState } from 'react'
import useIsInViewport, { CallbackRef, HookOptions } from 'use-is-in-viewport'

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
		prompt(
			'Automatic copy requires a secure context (https, localhost, or *.localhost). Please copy manually:',
			content
		)
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
	// try {
	const records = await pb.collection<SystemRecord>('systems').getFullList({ sort: '+name' })
	if (records.length) {
		$systems.set(records)
	} else {
		verifyAuth()
	}
	// }
	// catch (e) {
	// 	console.log('verifying auth error', e)
	// 	verifyAuth()
	// }
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
		format: (timestamp: string) => formatShortDate(timestamp),
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

/** Hacky solution to set the correct width of the yAxis in recharts */
export function useYaxisWidth(chartRef: React.RefObject<HTMLDivElement>) {
	const [yAxisWidth, setYAxisWidth] = useState(180)
	useEffect(() => {
		let interval = setInterval(() => {
			// console.log('chartRef', chartRef.current)
			const yAxisElement = chartRef?.current?.querySelector('.yAxis')
			if (yAxisElement) {
				// console.log('yAxisElement', yAxisElement)
				clearInterval(interval)
				setYAxisWidth(yAxisElement.getBoundingClientRect().width + 22)
			}
		}, 16)
		return () => clearInterval(interval)
	}, [])
	return yAxisWidth
}

export function useClampedIsInViewport(options: HookOptions): [boolean | null, CallbackRef] {
	const [isInViewport, wrappedTargetRef] = useIsInViewport(options)
	const [wasInViewportAtleastOnce, setWasInViewportAtleastOnce] = useState(isInViewport)

	useEffect(() => {
		setWasInViewportAtleastOnce((prev) => {
			// this will clamp it to the first true
			// received from useIsInViewport
			if (!prev) {
				return isInViewport
			}
			return prev
		})
	}, [isInViewport])

	return [wasInViewportAtleastOnce, wrappedTargetRef]
}

export function toFixedWithoutTrailingZeros(num: number, digits: number) {
	return parseFloat(num.toFixed(digits)).toString()
}

export function toFixedFloat(num: number, digits: number) {
	return parseFloat(num.toFixed(digits))
}
