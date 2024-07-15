import { $alerts, pb } from '@/lib/stores'
import { useStore } from '@nanostores/react'
import {
	Dialog,
	DialogTrigger,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { BellIcon } from 'lucide-react'
import { cn, isAdmin } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { AlertRecord, SystemRecord } from '@/types'
import { useMemo, useState } from 'react'
import { toast } from './ui/use-toast'

export default function AlertsButton({ system }: { system: SystemRecord }) {
	const alerts = useStore($alerts)

	const active = useMemo(() => {
		return alerts.find((alert) => alert.system === system.id)
	}, [alerts, system])

	const systemAlerts = useMemo(() => {
		return alerts.filter((alert) => alert.system === system.id) as AlertRecord[]
	}, [alerts, system])

	return (
		<Dialog>
			<DialogTrigger asChild>
				<Button variant="ghost" size={'icon'} aria-label="Alerts" data-nolink>
					<BellIcon
						className={cn('h-[1.2em] w-[1.2em] pointer-events-none', {
							'fill-foreground': active,
						})}
					/>
				</Button>
			</DialogTrigger>
			<DialogContent>
				<DialogHeader>
					<DialogTitle className="mb-1">Alerts for {system.name}</DialogTitle>
					<DialogDescription>
						{isAdmin() && (
							<span>
								Please{' '}
								<a
									href="/_/#/settings/mail"
									className="font-medium text-primary opacity-80 hover:opacity-100 duration-100"
								>
									configure an SMTP server
								</a>{' '}
								to ensure alerts are delivered.{' '}
							</span>
						)}
						Webhook delivery and more alert options will be added in the future.
					</DialogDescription>
				</DialogHeader>
				<Alert system={system} alerts={systemAlerts} />
			</DialogContent>
		</Dialog>
	)
}

function Alert({ system, alerts }: { system: SystemRecord; alerts: AlertRecord[] }) {
	const [pendingChange, setPendingChange] = useState(false)

	const alert = useMemo(() => {
		return alerts.find((alert) => alert.name === 'status')
	}, [alerts])

	return (
		<label
			htmlFor="status"
			className="space-y-2 flex flex-row items-center justify-between rounded-lg border p-4 cursor-pointer"
		>
			<div className="grid gap-0.5 select-none">
				<p className="font-medium text-base">System status</p>
				<span
					id=":r3m:-form-item-description"
					className="block text-[0.8rem] text-foreground opacity-80"
				>
					Triggers when status switches between up and down.
				</span>
			</div>
			<Switch
				id="status"
				className={cn('transition-opacity', pendingChange && 'opacity-40')}
				checked={!!alert}
				value={!!alert ? 'on' : 'off'}
				onCheckedChange={async (active) => {
					if (pendingChange) {
						return
					}
					setPendingChange(true)
					try {
						if (!active && alert) {
							await pb.collection('alerts').delete(alert.id)
						} else if (active) {
							pb.collection('alerts').create({
								system: system.id,
								user: pb.authStore.model!.id,
								name: 'status',
							})
						}
					} catch (e) {
						toast({
							title: 'Failed to update alert',
							description: 'Please check logs for more details.',
							variant: 'destructive',
						})
					} finally {
						setPendingChange(false)
					}
				}}
			/>
		</label>
	)
}
