import { alertInfo } from "@/lib/alerts"
import { $alerts, $allSystemsById } from "@/lib/stores"
import type { AlertRecord } from "@/types"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { useMemo } from "react"
import { $router, Link } from "./router"
import { Alert, AlertTitle, AlertDescription } from "./ui/alert"
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card"

export const ActiveAlerts = () => {
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

	// biome-ignore lint/correctness/useExhaustiveDependencies: alertsKey is inclusive
	return useMemo(() => {
		if (activeAlerts.length === 0) {
			return null
		}
		return (
			<Card>
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
											href={getPagePath($router, "system", { id: systems[alert.system]?.id })}
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
