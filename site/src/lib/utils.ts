import { toast } from '@/components/ui/use-toast'
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { $alerts, $systems, pb } from './stores'
import { AlertRecord, ChartTimes, SystemRecord } from '@/types'
import { RecordModel, RecordSubscription } from 'pocketbase'
import { WritableAtom } from 'nanostores'
import { timeDay, timeHour } from 'd3-time'

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
		toast({
			duration,
			description: 'Failed to copy',
		})
	}
}

export const updateServerList = () => {
	pb.collection<SystemRecord>('systems')
		.getFullList({ sort: '+name' })
		.then((records) => {
			$systems.set(records)
		})
}

export const updateAlerts = () => {
	pb.collection('alerts')
		.getFullList<AlertRecord>({ fields: 'id,name,system' })
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

const dayFormatter = new Intl.DateTimeFormat(undefined, {
	day: 'numeric',
	month: 'long',
	// dateStyle: 'medium',
})
export const formatDay = (timestamp: string) => {
	// console.log('ts', timestamp)
	return dayFormatter.format(new Date(timestamp))
}

export const updateFavicon = (newIconUrl: string) =>
	((document.querySelector("link[rel='icon']") as HTMLLinkElement).href = newIconUrl)

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

export const chartTimeData = {
	'1h': {
		label: '1 hour',
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -1),
	},
	'12h': {
		label: '12 hours',
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -12),
	},
	'24h': {
		label: '24 hours',
		format: (timestamp: string) => hourWithMinutes(timestamp),
		getOffset: (endTime: Date) => timeHour.offset(endTime, -24),
	},
	'1w': {
		label: '1 week',
		format: (timestamp: string) => formatDay(timestamp),
		getOffset: (endTime: Date) => timeDay.offset(endTime, -7),
	},
	'30d': {
		label: '30 days',
		format: (timestamp: string) => formatDay(timestamp),
		getOffset: (endTime: Date) => timeDay.offset(endTime, -30),
	},
}
