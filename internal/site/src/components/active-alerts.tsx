import { alertInfo } from "@/lib/alerts"
import { $alerts, $allSystemsById } from "@/lib/stores"
import type { AlertRecord } from "@/types"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { useMemo, useState } from "react"
import { AlertBannerSheet, AlertBannerSheetItem } from "./alert-banner-sheet"
import { $router } from "./router"

function AlertTriggeredDesc({ alert }: { alert: AlertRecord }) {
	const info = alertInfo[alert.name as keyof typeof alertInfo]
	if (info.triggeredDesc) {
		return info.triggeredDesc()
	}
	if (alert.name === "NetworkMonitorLoss") {
		return <Trans>One or more monitors exceed {alert.value}% loss</Trans>
	}
	if (alert.name === "Status") {
		return <Trans>Connection is down</Trans>
	}
	if (info.invert) {
		return (
			<Trans>
				Below {alert.value}
				{info.unit} in last <Plural value={alert.min} one="# minute" other="# minutes" />
			</Trans>
		)
	}
	return (
		<Trans>
			Exceeds {alert.value}
			{info.unit} in last <Plural value={alert.min} one="# minute" other="# minutes" />
		</Trans>
	)
}

/** Banner showing the number of triggered alerts, with a sheet listing them. */
export const ActiveAlerts = ({ className }: { className?: string }) => {
	const alerts = useStore($alerts)
	const systems = useStore($allSystemsById)
	const [open, setOpen] = useState(false)

	const { activeAlerts, systemCount, alertsKey } = useMemo(() => {
		const activeAlerts: AlertRecord[] = []
		const systemIds = new Set<string>()
		// key to prevent re-rendering if alerts change but active alerts didn't
		const alertsKey: string[] = []

		for (const systemId of Object.keys(alerts)) {
			for (const alert of alerts[systemId].values()) {
				if (alert.triggered && alert.name in alertInfo) {
					activeAlerts.push(alert)
					systemIds.add(alert.system)
					alertsKey.push(`${alert.id}${alert.value}${alert.min}`)
				}
			}
		}

		return { activeAlerts, systemCount: systemIds.size, alertsKey: alertsKey.join("") }
	}, [alerts])

	return useMemo(() => {
		const alertCount = activeAlerts.length
		if (alertCount === 0) {
			return null
		}
		return (
			<AlertBannerSheet
				open={open}
				onOpenChange={setOpen}
				className={className}
				title={<Plural value={alertCount} one="# active alert" other="# active alerts" />}
				description={<Plural value={systemCount} one="Across # system" other="Across # systems" />}
				buttonLabel={<Trans>View alerts</Trans>}
				sheetTitle={<Trans>Active Alerts</Trans>}
				sheetDescription={
					<Plural value={alertCount} one="# alert is currently triggered" other="# alerts are currently triggered" />
				}
			>
				{activeAlerts.map((alert) => {
					const info = alertInfo[alert.name as keyof typeof alertInfo]
					const system = systems[alert.system]
					return (
						<AlertBannerSheetItem
							key={alert.id}
							href={getPagePath($router, "system", { id: system?.id })}
							onClick={() => setOpen(false)}
							icon={info.icon}
							title={
								<>
									{system?.name} <span className="text-muted-foreground font-normal">·</span> {info.name()}
								</>
							}
							description={<AlertTriggeredDesc alert={alert} />}
						/>
					)
				})}
			</AlertBannerSheet>
		)
	}, [alertsKey, systemCount, systems, open, className])
}
