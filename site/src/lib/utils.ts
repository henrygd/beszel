import { toast } from '@/components/ui/use-toast'
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { $alerts, $systems, pb } from './stores'
import { AlertRecord, SystemRecord } from '@/types'
import { RecordModel, RecordSubscription } from 'pocketbase'
import { WritableAtom } from 'nanostores'

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

const shortTimeFormatter = new Intl.DateTimeFormat(undefined, {
	// day: 'numeric',
	// month: 'numeric',
	// year: '2-digit',
	// hour12: false,
	hour: 'numeric',
	minute: 'numeric',
})
export const formatShortTime = (timestamp: string) => shortTimeFormatter.format(new Date(timestamp))

const shortDateFormatter = new Intl.DateTimeFormat(undefined, {
	day: 'numeric',
	month: 'short',
	// year: '2-digit',
	// hour12: false,
	hour: 'numeric',
	minute: 'numeric',
})
export const formatShortDate = (timestamp: string) => shortDateFormatter.format(new Date(timestamp))

export const updateFavicon = (newIconUrl: string) =>
	((document.querySelector("link[rel='icon']") as HTMLLinkElement).href = newIconUrl)

export const isAdmin = () => pb.authStore.model?.admin

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
