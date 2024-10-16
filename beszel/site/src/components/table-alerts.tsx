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
import { BellIcon, CpuIcon, HardDriveIcon, MemoryStickIcon, ServerIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { AlertRecord, SystemRecord } from '@/types'
import { lazy, Suspense, useMemo, useState } from 'react'
import { toast } from './ui/use-toast'
import { Link } from './router'
import { EthernetIcon, ThermometerIcon } from './ui/icons'

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
			<DialogContent className="max-h-full overflow-auto max-w-[35rem]">
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
						title="CPU usage"
						description="Triggers when CPU usage exceeds a threshold."
						Icon={CpuIcon}
					/>
					<AlertWithSlider
						system={system}
						alerts={systemAlerts}
						name="Memory"
						title="Memory usage"
						description="Triggers when memory usage exceeds a threshold."
						Icon={MemoryStickIcon}
					/>
					<AlertWithSlider
						system={system}
						alerts={systemAlerts}
						name="Disk"
						title="Disk usage"
						description="Triggers when root usage exceeds a threshold."
						Icon={HardDriveIcon}
					/>
					<AlertWithSlider
						system={system}
						alerts={systemAlerts}
						name="Bandwidth"
						title="Bandwidth"
						description="Triggers when combined up/down exceeds a threshold."
						unit=" MB/s"
						Icon={EthernetIcon}
					/>
					<AlertWithSlider
						system={system}
						alerts={systemAlerts}
						name="Temperature"
						title="Temperature"
						description="Triggers when any sensor exceeds a threshold."
						unit=" °C"
						Icon={ThermometerIcon}
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
			className="flex flex-row items-center justify-between gap-4 rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 p-4 cursor-pointer"
		>
			<div className="grid gap-1 select-none">
				<p className="font-semibold flex gap-3 items-center">
					<ServerIcon className="h-4 w-4 opacity-85" /> System status
				</p>
				<span className="block text-sm text-muted-foreground">
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
	unit = '%',
	max = 99,
	Icon,
}: {
	system: SystemRecord
	alerts: AlertRecord[]
	name: string
	title: string
	description: string
	unit?: string
	max?: number
	Icon: React.FC<React.SVGProps<SVGSVGElement>>
}) {
	const [pendingChange, setPendingChange] = useState(false)
	const [liveValue, setLiveValue] = useState(80)
	const [liveMinutes, setLiveMinutes] = useState(10)

	const key = name.replaceAll(' ', '-')

	const alert = useMemo(() => {
		const alert = alerts.find((alert) => alert.name === name)
		if (alert) {
			setLiveValue(alert.value)
			setLiveMinutes(alert.min || 1)
		}
		return alert
	}, [alerts])

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
			<label
				htmlFor={`v${key}`}
				className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4', {
					'pb-0': !!alert,
				})}
			>
				<div className="grid gap-1 select-none">
					<p className="font-semibold flex gap-3 items-center">
						<Icon className="h-4 w-4 opacity-85" /> {title}
					</p>
					{!alert && <span className="block text-sm text-muted-foreground">{description}</span>}
				</div>
				<Switch
					id={`v${key}`}
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
									min: liveMinutes,
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
				<div className="grid sm:grid-cols-2 mt-1.5 gap-5 px-4 pb-5 tabular-nums text-muted-foreground">
					<Suspense fallback={<div className="h-10" />}>
						<div>
							<label htmlFor={`v${key}`} className="text-sm block h-8">
								Average exceeds{' '}
								<strong className="text-foreground">
									{liveValue}
									{unit}
								</strong>
							</label>
							<div className="flex gap-3">
								<Slider
									id={`v${key}`}
									defaultValue={[liveValue]}
									onValueCommit={(val) => {
										pb.collection('alerts').update(alert.id, {
											value: val[0],
										})
									}}
									onValueChange={(val) => setLiveValue(val[0])}
									min={1}
									max={max}
								/>
							</div>
						</div>
						<div>
							<label htmlFor={`t${key}`} className="text-sm block h-8">
								For <strong className="text-foreground">{liveMinutes}</strong> minute
								{liveMinutes > 1 && 's'}
							</label>
							<div className="flex gap-3">
								<Slider
									id={`t${key}`}
									defaultValue={[liveMinutes]}
									onValueCommit={(val) => {
										pb.collection('alerts').update(alert.id, {
											min: val[0],
										})
									}}
									onValueChange={(val) => setLiveMinutes(val[0])}
									min={1}
									max={60}
								/>
							</div>
						</div>
					</Suspense>
				</div>
			)}
		</div>
	)
}
