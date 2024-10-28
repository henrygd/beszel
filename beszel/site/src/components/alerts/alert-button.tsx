import { memo, useState } from 'react'
import { useStore } from '@nanostores/react'
import { $alerts, $systems } from '@/lib/stores'
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
import { SystemAlert, SystemAlertGlobal } from './alerts-system'
import { useTranslation } from 'react-i18next'

export default memo(function AlertsButton({ system }: { system: SystemRecord }) {
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
							'fill-primary': active,
						})}
					/>
				</Button>
			</DialogTrigger>
			<DialogContent className="max-h-full overflow-auto max-w-[35rem]">
				{opened && <TheContent data={{ system, alerts, systemAlerts }} />}
			</DialogContent>
		</Dialog>
	)
})

function TheContent({
	data: { system, alerts, systemAlerts },
}: {
	data: { system: SystemRecord; alerts: AlertRecord[]; systemAlerts: AlertRecord[] }
}) {
	const { t } = useTranslation()

	const [overwriteExisting, setOverwriteExisting] = useState<boolean | 'indeterminate'>(false)
	const systems = $systems.get()

	const data = Object.keys(alertInfo).map((key) => {
		const alert = alertInfo[key as keyof typeof alertInfo]
		return {
			key: key as keyof typeof alertInfo,
			alert,
			system,
		}
	})

	return (
		<>
			<DialogHeader>
				<DialogTitle className="text-xl">{t('alerts.title')}</DialogTitle>
				<DialogDescription>
					{t('alerts.subtitle_1')}{' '}
					<Link href="/settings/notifications" className="link">
						{t('alerts.notification_settings')}
					</Link>{' '}
					{t('alerts.subtitle_2')}
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
						{t('all_systems')}
					</TabsTrigger>
				</TabsList>
				<TabsContent value="system">
					<div className="grid gap-3">
						{data.map((d) => (
							<SystemAlert key={d.key} system={system} data={d} systemAlerts={systemAlerts} />
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
						{t('alerts.overwrite_existing_alerts')}
					</label>
					<div className="grid gap-3">
						{data.map((d) => (
							<SystemAlertGlobal
								key={d.key}
								data={d}
								overwrite={overwriteExisting}
								alerts={alerts}
								systems={systems}
							/>
						))}
					</div>
				</TabsContent>
			</Tabs>
		</>
	)
}
