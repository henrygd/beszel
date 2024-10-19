import { useState } from 'react'
import { useStore } from '@nanostores/react'
import { $alerts } from '@/lib/stores'
import {
	Dialog,
	DialogTrigger,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { BellIcon, GlobeIcon, ServerIcon } from 'lucide-react'
import { alertInfo, cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { AlertRecord, SystemRecord } from '@/types'
import { Link } from '../router'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Checkbox } from '../ui/checkbox'
import { AlertStatusGlobal, AlertStatusSystem } from './alert-status'
import { AlertWithSliders, AlertWithSlidersGlobal } from './alert-sliders'

// todo: store systems without existing alerts on mount so the first change doesn't prevent it from running again
// currently sets to 80%, but then doesn't change bc it technically exists

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
					size="icon"
					aria-label="Alerts"
					data-nolink
					onClick={() => setOpened(true)}
				>
					<BellIcon
						className={cn('h-[1.2em] w-[1.2em] pointer-events-none', {
							'fill-muted-foreground': active,
							'stroke-muted-foreground': active,
						})}
					/>
				</Button>
			</DialogTrigger>
			<DialogContent className="max-h-full overflow-auto max-w-[35rem]">
				{opened && <TheContent data={{ system, alerts, systemAlerts }} />}
			</DialogContent>
		</Dialog>
	)
}

function TheContent({
	data: { system, alerts, systemAlerts },
}: {
	data: { system: SystemRecord; alerts: AlertRecord[]; systemAlerts: AlertRecord[] }
}) {
	const [overwriteExisting, setOverwriteExisting] = useState<boolean | 'indeterminate'>(false)

	interface ArrData {
		key: keyof typeof alertInfo
		alert: (typeof alertInfo)[keyof typeof alertInfo]
		system: SystemRecord
	}

	const data = (() => {
		const arr: ArrData[] = []
		for (const key in alertInfo) {
			const alert = alertInfo[key as keyof typeof alertInfo]
			arr.push({
				key: key as keyof typeof alertInfo,
				alert,
				system,
			})
		}
		return arr

		// (
		// 	<AlertWithSliders
		// 		key={key}
		// 		system={system}
		// 		alerts={systemAlerts}
		// 		name={key as keyof typeof alertInfo}
		// 		title={alert.name}
		// 		description={alert.desc}
		// 		unit={alert.unit}
		// 	/>
		// ))
	})()

	return (
		<>
			<DialogHeader>
				<DialogTitle className="text-xl">Alerts</DialogTitle>
				<DialogDescription>
					See{' '}
					<Link href="/settings/notifications" className="link">
						notification settings
					</Link>{' '}
					to configure how you receive alerts.
				</DialogDescription>
			</DialogHeader>
			<Tabs defaultValue="system">
				<TabsList className="mb-1 -mt-0.5">
					<TabsTrigger value="system">
						<ServerIcon className="mr-2 h-3.5 w-3.5" />
						{system.name}
					</TabsTrigger>
					<TabsTrigger value="global">
						<GlobeIcon className="mr-1.5 h-3.5 w-3.5" />
						All systems
					</TabsTrigger>
				</TabsList>
				<TabsContent value="system">
					<div className="grid gap-3">
						<AlertStatusSystem system={system} alerts={systemAlerts} />
						{data.map((d) => (
							<AlertWithSliders
								key={d.key}
								system={system}
								data={d}
								systemAlerts={systemAlerts}
								// title={alert.name}
								// description={alert.desc}
								// unit={alert.unit}
							/>
						))}
					</div>
				</TabsContent>
				<TabsContent value="global">
					<label
						htmlFor="ovw"
						className="mb-3 flex gap-2 items-center justify-center cursor-pointer border rounded-sm py-3 px-4 border-destructive text-destructive font-semibold text-sm"
					>
						<Checkbox
							id="ovw"
							className="text-destructive border-destructive data-[state=checked]:bg-destructive"
							checked={overwriteExisting}
							onCheckedChange={setOverwriteExisting}
						/>
						Overwrite existing alerts
					</label>
					<div className="grid gap-3">
						<AlertStatusGlobal alerts={alerts} overwrite={overwriteExisting} />
						{data.map((d) => (
							<AlertWithSlidersGlobal
								key={d.key}
								system={system}
								data={d}
								overwrite={overwriteExisting}
								alerts={alerts}
								// title={alert.name}
								// description={alert.desc}
								// unit={alert.unit}
							/>
						))}
					</div>
				</TabsContent>
			</Tabs>
		</>
	)
}
