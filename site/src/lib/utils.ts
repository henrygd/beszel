import { toast } from '@/components/ui/use-toast'
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { $servers, pb } from './stores'
import { SystemRecord } from '@/types'

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
			$servers.set(records)
		})
}
