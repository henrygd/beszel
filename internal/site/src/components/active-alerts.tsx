import { alertInfo } from "@/lib/alerts"
import { $alerts, $allSystemsById } from "@/lib/stores"
import { useBrowserStorage } from "@/lib/utils"
import type { AlertRecord } from "@/types"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { useEffect, useMemo, useState } from "react"
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

function AlertLabel({ alert, systemName }: { alert: AlertRecord; systemName?: string }) {
	const info = alertInfo[alert.name as keyof typeof alertInfo]
	return (
		<>
			{systemName} <span className="opacity-60 font-normal">·</span> {info.name()}
		</>
	)
}

/** Banner showing the number of triggered alerts, with a sheet listing them. */
export const ActiveAlerts = ({ className }: { className?: string }) => {
	const alerts = useStore($alerts)
	const systems = useStore($allSystemsById)
	const [open, setOpen] = useState(false)
	// ids of the alerts that were active when the banner was last dismissed
	const [dismissedIds, setDismissedIds] = useBrowserStorage<string[]>("dismissedAlerts", [])

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

	// forget dismissed alerts once they resolve so they show again if they retrigger.
	// skipped while alerts are still loading so a reload doesn't clear the dismissal.
	useEffect(() => {
		if (Object.keys(alerts).length === 0) {
			return
		}
		const activeIds = new Set(activeAlerts.map((alert) => alert.id))
		if (dismissedIds.some((id) => !activeIds.has(id))) {
			setDismissedIds(dismissedIds.filter((id) => activeIds.has(id)))
		}
	}, [alertsKey])

	return useMemo(() => {
		const alertCount = activeAlerts.length
		// stay hidden after dismissing until an alert triggers that wasn't active at the time
		if (alertCount === 0 || activeAlerts.every((alert) => dismissedIds.includes(alert.id))) {
			return null
		}
		// name the alert directly in the banner when there is only one
		const [firstAlert] = activeAlerts
		return (
			<AlertBannerSheet
				open={open}
				onOpenChange={setOpen}
				onDismiss={() => setDismissedIds(activeAlerts.map((alert) => alert.id))}
				className={className}
				title={
					alertCount === 1 ? (
						<AlertLabel alert={firstAlert} systemName={systems[firstAlert.system]?.name} />
					) : (
						<Plural value={alertCount} one="# active alert" other="# active alerts" />
					)
				}
				description={
					alertCount === 1 ? (
						<AlertTriggeredDesc alert={firstAlert} />
					) : (
						<Plural value={systemCount} one="Across # system" other="Across # systems" />
					)
				}
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
							title={<AlertLabel alert={alert} systemName={system?.name} />}
							description={<AlertTriggeredDesc alert={alert} />}
						/>
					)
				})}
			</AlertBannerSheet>
		)
	}, [alertsKey, systemCount, systems, open, className, dismissedIds])
}
