import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { GithubIcon } from "lucide-react"
import { memo, Suspense, useEffect, useMemo } from "react"
import { $router, Link } from "@/components/router"
import SystemsTable from "@/components/systems-table/systems-table"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { alertInfo } from "@/lib/alerts"
import { $alerts, $allSystemsById } from "@/lib/stores"
import type { AlertRecord } from "@/types"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`Dashboard`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
				<ActiveAlerts />
				<Suspense>
					<SystemsTable />
				</Suspense>

				<div className="flex gap-1.5 justify-end items-center pe-3 sm:pe-6 mt-3.5 mb-4 text-xs opacity-80">
					<a
						href="https://github.com/henrygd/beszel"
						target="_blank"
						className="flex items-center gap-0.5 text-muted-foreground hover:text-foreground duration-75"
						rel="noopener"
					>
						<GithubIcon className="h-3 w-3" /> GitHub
					</a>
					<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
					<a
						href="https://github.com/henrygd/beszel/releases"
						target="_blank"
						className="text-muted-foreground hover:text-foreground duration-75"
						rel="noopener"
					>
						Beszel {globalThis.BESZEL.HUB_VERSION}
					</a>
				</div>
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

	// biome-ignore lint/correctness/useExhaustiveDependencies: alertsKey is inclusive
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
											{systems[alert.system]?.name} {info.name().toLowerCase().replace("cpu", "CPU")}{alert.filesystem && alert.name === "Disk" ? ` (${alert.filesystem})` : ""}
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
