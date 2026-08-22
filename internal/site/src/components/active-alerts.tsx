import { alertInfo } from "@/lib/alerts"
import { $alerts, $allSystemsById } from "@/lib/stores"
import type { AlertRecord } from "@/types"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { $router, Link } from "./router"
import { Alert, AlertTitle, AlertDescription } from "./ui/alert"
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card"

export const ActiveAlerts = () => {
	const alerts = useStore($alerts)
	const systems = useStore($allSystemsById)

	const activeAlerts: AlertRecord[] = []
	for (const systemAlerts of Object.values(alerts)) {
		for (const alert of systemAlerts.values()) {
			if (alert.triggered && alert.name in alertInfo && systems[alert.system]) activeAlerts.push(alert)
		}
	}

	if (activeAlerts.length === 0) return null

	return (
		<Card className="border-amber-500/25">
			<CardHeader className="px-4 pb-3 pt-5 sm:px-6">
				<p className="font-mono text-[0.68rem] font-semibold uppercase tracking-[0.2em] text-amber-500">
					<Trans>Attention required</Trans>
				</p>
				<CardTitle className="mt-1">
					<Trans>Active Alerts</Trans>
				</CardTitle>
			</CardHeader>
			<CardContent className="px-3 pb-3 sm:px-6 sm:pb-6">
				<div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
					{activeAlerts.map((alert) => {
						const info = alertInfo[alert.name as keyof typeof alertInfo]
						const system = systems[alert.system]
						return (
							<Alert
								key={alert.id}
								className="border-amber-500/25 bg-amber-500/[0.055] transition hover:border-amber-500/45"
							>
								<info.icon className="h-4 w-4 text-amber-500" />
								<AlertTitle>
									{system.name} · {info.name()}
								</AlertTitle>
								<AlertDescription>
									{alert.name === "Status" ? (
										<Trans>Connection is down</Trans>
									) : info.invert ? (
										<Trans>
											Below {alert.value}
											{info.unit} in last <Plural value={alert.min} one="# minute" other="# minutes" />
										</Trans>
									) : (
										<Trans>
											Exceeds {alert.value}
											{info.unit} in last <Plural value={alert.min} one="# minute" other="# minutes" />
										</Trans>
									)}
								</AlertDescription>
								<Link
									href={getPagePath($router, "system", { id: system.id })}
									className="absolute inset-0"
									aria-label={`View ${system.name}`}
								/>
							</Alert>
						)
					})}
				</div>
			</CardContent>
		</Card>
	)
}
