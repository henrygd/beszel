import { pb, $systems } from '@/lib/stores'
import { alertQueue, cn, failedUpdateToast } from '@/lib/utils'
import { SystemRecord, AlertRecord } from '@/types'
import { useStore } from '@nanostores/react'
import { Switch } from '@/components/ui/switch'
import { ServerIcon } from 'lucide-react'
import { useState } from 'react'

export const AlertStatusSystem = ({
	system,
	alerts,
}: {
	system: SystemRecord
	alerts: AlertRecord[]
}) => {
	const [pendingChange, setPendingChange] = useState(false)
	const alert = alerts.find((alert) => alert.name === 'Status')

	const handleChange = async (active: boolean) => {
		if (pendingChange) return
		setPendingChange(true)
		try {
			if (!active && alert) {
				await pb.collection('alerts').delete(alert.id)
			} else if (active) {
				await pb.collection('alerts').create({
					system: system.id,
					user: pb.authStore.model!.id,
					name: 'Status',
				})
			}
		} catch (e) {
			failedUpdateToast()
		} finally {
			setPendingChange(false)
		}
	}

	return (
		<AlertStatus>
			<Switch
				id="alert-status"
				className={cn('transition-opacity', pendingChange && 'opacity-40')}
				checked={!!alert}
				value={!!alert ? 'on' : 'off'}
				onCheckedChange={handleChange}
			/>
		</AlertStatus>
	)
}

export const AlertStatusGlobal = ({
	alerts,
	overwrite,
}: {
	alerts: AlertRecord[]
	overwrite: boolean | 'indeterminate'
}) => {
	const systems = useStore($systems)

	const handleChange = async (checked: boolean) => {
		const name = 'Status'
		const queue = alertQueue()

		for (let system of systems) {
			const matchingAlert = alerts.find(
				(alert) => alert.system === system.id && name === alert.name
			)

			if (matchingAlert && !overwrite) {
				continue
			}

			if (checked && !matchingAlert) {
				queue.add(() =>
					pb.collection('alerts').create({
						system: system.id,
						user: pb.authStore.model!.id,
						name,
					})
				)
			} else if (!checked && matchingAlert) {
				queue.add(() => pb.collection('alerts').delete(matchingAlert.id))
			}
		}
	}

	return (
		<AlertStatus>
			<Switch id="alert-status" className="transition-opacity" onCheckedChange={handleChange} />
		</AlertStatus>
	)
}

const AlertStatus = ({ children }: { children: React.ReactNode }) => (
	<label
		htmlFor="alert-status"
		className="flex flex-row items-center justify-between gap-4 rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 p-4 cursor-pointer"
	>
		<div className="grid gap-1 select-none">
			<p className="font-semibold flex gap-3 items-center">
				<ServerIcon className="h-4 w-4 opacity-85" /> System Status
			</p>
			<span className="block text-sm text-muted-foreground">
				Triggers when status switches between up and down.
			</span>
		</div>
		{children}
	</label>
)
