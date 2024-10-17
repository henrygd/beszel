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
import { BellIcon, ServerIcon } from 'lucide-react'
import { alertInfo, cn } from '@/lib/utils'
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
	const [opened, setOpened] = useState(false)

	const systemAlerts = alerts.filter((alert) => alert.system === system.id) as AlertRecord[]

	const active = systemAlerts.length > 0

	return (
		<Dialog>
			<DialogTrigger asChild>
				<Button
					variant="ghost"
					size={'icon'}
					aria-label="Alerts"
					data-nolink
					onClick={() => setOpened(true)}
				>
					<BellIcon
						className={cn('h-[1.2em] w-[1.2em] pointer-events-none', {
							'fill-foreground': active,
						})}
					/>
				</Button>
			</DialogTrigger>
			<DialogContent
				className="max-h-full overflow-auto max-w-[35rem]"
				// onCloseAutoFocus={() => setOpened(false)}
			>
				{opened && (
					<>
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
							{Object.keys(alertInfo).map((key) => {
								const alert = alertInfo[key as keyof typeof alertInfo]
								return (
									<AlertWithSlider
										key={key}
										system={system}
										alerts={systemAlerts}
										name={key}
										title={alert.name}
										description={alert.desc}
										unit={alert.unit}
										Icon={alert.icon}
									/>
								)
							})}
						</div>
					</>
				)}
			</DialogContent>
		</Dialog>
	)
}

function AlertStatus({ system, alerts }: { system: SystemRecord; alerts: AlertRecord[] }) {
	const [pendingChange, setPendingChange] = useState(false)

	const alert = alerts.find((alert) => alert.name === 'Status')

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
	const [value, setValue] = useState(80)
	const [min, setMin] = useState(10)

	const key = name.replaceAll(' ', '-')

	const alert = useMemo(() => {
		const alert = alerts.find((alert) => alert.name === name)
		if (alert) {
			setValue(alert.value)
			setMin(alert.min || 1)
		}
		return alert
	}, [alerts])

	const updateAlert = (obj: Partial<AlertRecord>) => {
		obj.triggered = false
		alert && pb.collection('alerts').update(alert.id, obj)
	}

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
			<label
				htmlFor={`s${key}`}
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
					id={`s${key}`}
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
									value: value,
									min: min,
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
							<p id={`v${key}`} className="text-sm block h-8">
								Average exceeds{' '}
								<strong className="text-foreground">
									{value}
									{unit}
								</strong>
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`v${key}`}
									defaultValue={[value]}
									onValueCommit={(val) => updateAlert({ value: val[0] })}
									onValueChange={(val) => setValue(val[0])}
									min={1}
									max={max}
								/>
							</div>
						</div>
						<div>
							<p id={`t${key}`} className="text-sm block h-8">
								For <strong className="text-foreground">{min}</strong> minute
								{min > 1 && 's'}
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`v${key}`}
									defaultValue={[min]}
									onValueCommit={(val) => updateAlert({ min: val[0] })}
									onValueChange={(val) => setMin(val[0])}
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
