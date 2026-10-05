import { pb, saveUserSettings } from "@/lib/api"
import { alertInfo } from "@/lib/alerts"
import { $alerts, $allSystemsById, $userSettings } from "@/lib/stores"
import { formatDuration, formatShortDate, useBrowserStorage } from "@/lib/utils"
import type { AlertRecord, AlertsHistoryRecord } from "@/types"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { CircleCheckIcon } from "lucide-react"
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

function AlertLabel({ alert, systemName }: { alert: Pick<AlertRecord, "name">; systemName?: string }) {
	const info = alertInfo[alert.name as keyof typeof alertInfo]
	return (
		<>
			{systemName} <span className="opacity-60 font-normal">·</span> {info.name()}
		</>
	)
}

type ResolvedAlert = Pick<
	AlertsHistoryRecord,
	"id" | "alert_id" | "system" | "name" | "monitor_name" | "created" | "resolved"
>

function AlertResolvedDesc({ alert }: { alert: ResolvedAlert }) {
	const resolved = formatShortDate(alert.resolved ?? "")
	const duration = formatDuration(alert.created, alert.resolved)
	return (
		<>
			{alert.monitor_name && `${alert.monitor_name} · `}
			<Trans>
				Resolved {resolved} after {duration}
			</Trans>
		</>
	)
}

/**
 * Alerts resolved after `since`, newest first, one per alert. Refetched when the
 * active alerts change, since that is when new history records get resolved.
 */
function useResolvedAlerts(enabled: boolean, since: string | undefined, refreshKey: string) {
	const [resolved, setResolved] = useState<ResolvedAlert[]>([])
	useEffect(() => {
		if (!enabled || !since) {
			setResolved([])
			return
		}
		let cancelled = false
		// the history record is resolved right after the alert record, so give it a moment
		const timeout = setTimeout(async () => {
			try {
				const { items } = await pb.collection<ResolvedAlert>("alerts_history").getList(1, 100, {
					filter: pb.filter("resolved > {:since}", { since: new Date(since) }),
					sort: "-resolved",
					fields: "id,alert_id,system,name,monitor_name,created,resolved",
					requestKey: "resolvedAlerts",
				})
				if (!cancelled) {
					setResolved(items)
				}
			} catch (e) {
				if (!cancelled) {
					console.error("resolved alerts", e)
				}
			}
		}, 1000)
		return () => {
			cancelled = true
			clearTimeout(timeout)
		}
	}, [enabled, since, refreshKey])
	return resolved
}

/** Banner showing the number of triggered alerts, with a sheet listing them. */
export const ActiveAlerts = ({ className }: { className?: string }) => {
	const alerts = useStore($alerts)
	const systems = useStore($allSystemsById)
	const [open, setOpen] = useState(false)
	// ids of the alerts that were active when the banner was last dismissed.
	// session storage because a retrigger while the page is closed keeps the same id.
	const [dismissedIds, setDismissedIds] = useBrowserStorage<string[]>("dismissedAlerts", [], sessionStorage)
	const { resolvedAlerts: resolvedMode, resolvedAlertsDismissed } = useStore($userSettings, {
		keys: ["resolvedAlerts", "resolvedAlertsDismissed"],
	})

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

	// with "keep until dismissed", resolved alerts stay listed until the user dismisses them
	const resolvedHistory = useResolvedAlerts(resolvedMode === "keep", resolvedAlertsDismissed, alertsKey)
	const resolvedAlerts = useMemo(() => {
		const activeIds = new Set(activeAlerts.map((alert) => alert.id))
		const seen = new Set<string>()
		const resolvedAlerts: ResolvedAlert[] = []
		for (const alert of resolvedHistory) {
			const key = `${alert.alert_id}${alert.monitor_name ?? ""}`
			// skip alerts that triggered again, older entries for the same alert, and removed systems
			if (activeIds.has(alert.alert_id) || seen.has(key) || !(alert.name in alertInfo) || !systems[alert.system]) {
				continue
			}
			seen.add(key)
			resolvedAlerts.push(alert)
		}
		return resolvedAlerts
	}, [resolvedHistory, alertsKey, systems])

	// forget dismissed alerts once they resolve so they show again if they retrigger.
	// skipped while alerts are still loading so a reload doesn't clear the dismissal,
	// and re-run once loaded in case they resolved while the page was closed.
	const alertsLoaded = Object.keys(alerts).length > 0
	useEffect(() => {
		if (!alertsLoaded) {
			return
		}
		const activeIds = new Set(activeAlerts.map((alert) => alert.id))
		if (dismissedIds.some((id) => !activeIds.has(id))) {
			setDismissedIds(dismissedIds.filter((id) => activeIds.has(id)))
		}
	}, [alertsKey, alertsLoaded])

	return useMemo(() => {
		const alertCount = activeAlerts.length
		const resolvedCount = resolvedAlerts.length
		// stay hidden after dismissing until an alert triggers that wasn't active at the time
		// (or, with "keep until dismissed", until another alert resolves)
		const showActive = alertCount > 0 && !activeAlerts.every((alert) => dismissedIds.includes(alert.id))
		if (!showActive && resolvedCount === 0) {
			return null
		}

		function dismiss() {
			setDismissedIds(activeAlerts.map((alert) => alert.id))
			// resolved alerts are listed newest first, so this hides all of them (uses server time)
			const newestResolved = resolvedAlerts[0]?.resolved
			if (newestResolved) {
				saveUserSettings({ resolvedAlertsDismissed: new Date(newestResolved).toISOString() }).catch(console.error)
			}
		}

		// name the alert directly in the banner when there is only one
		const [firstAlert] = activeAlerts
		const [firstResolved] = resolvedAlerts
		let title: React.ReactNode
		let description: React.ReactNode
		if (alertCount === 1) {
			title = <AlertLabel alert={firstAlert} systemName={systems[firstAlert.system]?.name} />
			description = <AlertTriggeredDesc alert={firstAlert} />
		} else if (alertCount > 1) {
			title = <Plural value={alertCount} one="# active alert" other="# active alerts" />
			description = <Plural value={systemCount} one="Across # system" other="Across # systems" />
		} else if (resolvedCount === 1) {
			title = <AlertLabel alert={firstResolved} systemName={systems[firstResolved.system]?.name} />
			description = <AlertResolvedDesc alert={firstResolved} />
		} else {
			title = <Plural value={resolvedCount} one="# resolved alert" other="# resolved alerts" />
		}
		if (alertCount > 0 && resolvedCount > 0) {
			description = <Plural value={resolvedCount} one="# resolved alert" other="# resolved alerts" />
		}

		return (
			<AlertBannerSheet
				open={open}
				onOpenChange={setOpen}
				onDismiss={dismiss}
				className={className}
				icon={alertCount === 0 ? CircleCheckIcon : undefined}
				title={title}
				description={description}
				buttonLabel={<Trans>View alerts</Trans>}
				sheetTitle={<Trans>Active Alerts</Trans>}
				sheetDescription={
					<>
						{alertCount > 0 && (
							<Plural
								value={alertCount}
								one="# alert is currently triggered"
								other="# alerts are currently triggered"
							/>
						)}
						{alertCount > 0 && resolvedCount > 0 && " "}
						{resolvedCount > 0 && (
							<Plural
								value={resolvedCount}
								one="# resolved alert is kept until dismissed"
								other="# resolved alerts are kept until dismissed"
							/>
						)}
					</>
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
				{resolvedAlerts.map((alert) => {
					const info = alertInfo[alert.name as keyof typeof alertInfo]
					const system = systems[alert.system]
					return (
						<AlertBannerSheetItem
							key={alert.id}
							resolved
							href={getPagePath($router, "system", { id: system?.id })}
							onClick={() => setOpen(false)}
							icon={info.icon}
							title={<AlertLabel alert={alert} systemName={system?.name} />}
							description={<AlertResolvedDesc alert={alert} />}
						/>
					)
				})}
			</AlertBannerSheet>
		)
	}, [alertsKey, resolvedAlerts, systemCount, systems, open, className, dismissedIds])
}
