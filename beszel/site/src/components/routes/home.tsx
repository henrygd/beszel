import { Suspense, memo, useEffect, useMemo } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card"
import { $alerts, $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { getSystemNameFromId } from "@/lib/utils"
import { pb, updateRecordList, updateSystemList } from "@/lib/api"
import { AlertRecord, SystemRecord } from "@/types"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { $router, Link } from "../router"
import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { getPagePath } from "@nanostores/router"
import { alertInfo } from "@/lib/alerts"
import SystemsTable from "@/components/systems-table/systems-table"

export default memo(function () {
	const { t } = useLingui()

	useEffect(() => {
		document.title = t`Dashboard` + " / Beszel"
	}, [t])

	return useMemo(
		() => (
			<>
				<ActiveAlerts />
				<Suspense>
					<SystemsTable />
				</Suspense>

			</>
		),
		[]
	)
})

const ActiveAlerts = () => {
	const alerts = useStore($alerts)
	const systems = useStore($allSystemsById)

	const { activeAlerts, alertsKey } = useMemo(() => {
		const activeAlerts: AlertRecord[] = []
		// key to prevent re-rendering if alerts change but active alerts didn't
		const alertsKey: string[] = []

		for (const systemId of Object.keys(alerts)) {
			for (const alert of alerts[systemId].values()) {
				if (alert.triggered && alert.name in alertInfo) {
					activeAlerts.push(alert)
					alertsKey.push(`${alert.system}${alert.value}${alert.min}`)
				}
			}
		}

		return { activeAlerts, alertsKey }
	}, [alerts])

	return useMemo(() => {
		if (activeAlerts.length === 0) {
			return null
		}
		return (
			<Card className="mb-4">
				<CardHeader className="pb-4 px-2 sm:px-6 max-sm:pt-5 max-sm:pb-1">
					<div className="px-2 sm:px-1">
						<CardTitle>
							<Trans>Active Alerts</Trans>
						</CardTitle>
					</div>
				</CardHeader>
				<CardContent className="max-sm:p-2">
					{activeAlerts.length > 0 && (
						<div className="grid sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-3">
							{activeAlerts.map((alert) => {
								const info = alertInfo[alert.name as keyof typeof alertInfo]
								return (
									<Alert
										key={alert.id}
										className="hover:-translate-y-px duration-200 bg-transparent border-foreground/10 hover:shadow-md shadow-black/5"
									>
										<info.icon className="h-4 w-4" />
										<AlertTitle>
											{systems[alert.system]?.name} {info.name().toLowerCase().replace("cpu", "CPU")}
										</AlertTitle>
										<AlertDescription>
											{alert.name === "Status" ? (
												<Trans>Connection is down</Trans>
											) : (
												<Trans>
													Exceeds {alert.value}
													{info.unit} in last <Plural value={alert.min} one="# minute" other="# minutes" />
												</Trans>
											)}
										</AlertDescription>
										<Link
											href={getPagePath($router, "system", { name: systems[alert.system]?.name })}
											className="absolute inset-0 w-full h-full"
											aria-label="View system"
										></Link>
									</Alert>
								)
							})}
						</div>
					)}
				</CardContent>
			</Card>
		)
	}, [alertsKey.join("")])
}
