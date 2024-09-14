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
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { AlertRecord, SystemRecord } from '@/types'
import { lazy, Suspense, useMemo, useState } from 'react'
import { toast } from './ui/use-toast'
import { Link } from './router'

const Slider = lazy(() => import('./ui/slider'))

const failedUpdateToast = () =>
	toast({
		title: 'Failed to update alert',
		description: 'Please check logs for more details.',
		variant: 'destructive',
	})

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
			<DialogContent className="max-h-full overflow-auto">
				<DialogHeader>
					<DialogTitle className="text-xl">{system.name} alerts</DialogTitle>
					<DialogDescription className="mb-1">
						See{' '}
						<Link href="/settings/notifications" className="link">
							notification settings
						</Link>{' '}
						to configure how you receive alerts.
					</DialogDescription>
				</DialogHeader>
				<div className="grid gap-3">
					<AlertStatus system={system} alerts={systemAlerts} />
					<AlertWithSlider
						system={system}
						alerts={systemAlerts}
						name="CPU"
						title="CPU Usage"
						description="Triggers when CPU usage exceeds a threshold."
					/>
					<AlertWithSlider
						system={system}
						alerts={systemAlerts}
						name="Memory"
						title="Memory Usage"
						description="Triggers when memory usage exceeds a threshold."
					/>
					<AlertWithSlider
						system={system}
						alerts={systemAlerts}
						name="Disk"
						title="Disk Usage"
						description="Triggers when root usage exceeds a threshold."
					/>
				</div>
			</DialogContent>
		</Dialog>
	)
}

function AlertStatus({ system, alerts }: { system: SystemRecord; alerts: AlertRecord[] }) {
	const [pendingChange, setPendingChange] = useState(false)

	const alert = useMemo(() => {
		return alerts.find((alert) => alert.name === 'Status')
	}, [alerts])

	return (
		<label
			htmlFor="alert-status"
			className="flex flex-row items-center justify-between gap-4 rounded-lg border p-4 cursor-pointer"
		>
			<div className="grid gap-1 select-none">
				<p className="font-semibold">System Status</p>
				<span className="block text-sm text-foreground opacity-80">
					Triggers when status switches between up and down.
				</span>
			</div>
			<Switch
				id="alert-status"
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
								name: 'Status',
							})
						}
					} catch (e) {
						failedUpdateToast()
					} finally {
						setPendingChange(false)
					}
				}}
			/>
		</label>
	)
}

function AlertWithSlider({
	system,
	alerts,
	name,
	title,
	description,
}: {
	system: SystemRecord
	alerts: AlertRecord[]
	name: string
	title: string
	description: string
}) {
	const [pendingChange, setPendingChange] = useState(false)
	const [liveValue, setLiveValue] = useState(50)

	const alert = useMemo(() => {
		const alert = alerts.find((alert) => alert.name === name)
		if (alert) {
			setLiveValue(alert.value)
		}
		return alert
	}, [alerts])

	return (
		<div className="rounded-lg border">
			<label
				htmlFor={`alert-${name}`}
				className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4', {
					'pb-0': !!alert,
				})}
			>
				<div className="grid gap-1 select-none">
					<p className="font-semibold">{title}</p>
					<span className="block text-sm text-foreground opacity-80">{description}</span>
				</div>
				<Switch
					id={`alert-${name}`}
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
									name,
									value: liveValue,
								})
							}
						} catch (e) {
							failedUpdateToast()
						} finally {
							setPendingChange(false)
						}
					}}
				/>
			</label>
			{alert && (
				<div className="flex mt-2 mb-3 gap-3 px-4">
					<Suspense>
						<Slider
							defaultValue={[liveValue]}
							onValueCommit={(val) => {
								pb.collection('alerts').update(alert.id, {
									value: val[0],
								})
							}}
							onValueChange={(val) => {
								setLiveValue(val[0])
							}}
							min={10}
							max={99}
							// step={1}
						/>
					</Suspense>
					<span className="tabular-nums tracking-tighter text-[.92em]">{liveValue}%</span>
				</div>
			)}
		</div>
	)
}
