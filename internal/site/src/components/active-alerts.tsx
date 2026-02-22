import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { type MouseEvent, useMemo, useState } from "react"
import { alertInfo } from "@/lib/alerts"
import { pb } from "@/lib/api"
import { $alerts, $allSystemsById } from "@/lib/stores"
import type { AlertRecord } from "@/types"
import { $router, Link } from "./router"
import { Alert, AlertDescription, AlertTitle } from "./ui/alert"
import { Button } from "./ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card"
import { toast } from "./ui/use-toast"

export const ActiveAlerts = () => {
	const alerts = useStore($alerts)
	const systems = useStore($allSystemsById)
	const [dismissing, setDismissing] = useState<Record<string, boolean>>({})

	const activeAlerts = useMemo(() => {
		const activeAlerts: AlertRecord[] = []

		for (const systemId of Object.keys(alerts)) {
			for (const alert of alerts[systemId].values()) {
				if (alert.triggered && alert.name in alertInfo) {
					activeAlerts.push(alert)
				}
			}
		}

		return activeAlerts
	}, [alerts])

	const dismissAlert = async (event: MouseEvent<HTMLButtonElement>, alert: AlertRecord) => {
		event.preventDefault()
		event.stopPropagation()
		if (dismissing[alert.id]) {
			return
		}

		setDismissing((current) => ({ ...current, [alert.id]: true }))
		try {
			await pb.collection<AlertRecord>("alerts").update(alert.id, { triggered: false })
		} catch (error) {
			console.error(error)
			toast({
				title: t`Failed to update alert`,
				description: t`Please check logs for more details.`,
				variant: "destructive",
			})
		} finally {
			setDismissing((current) => {
				const next = { ...current }
				delete next[alert.id]
				return next
			})
		}
	}

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
									{systems[alert.system]?.name} {info.name()}
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
								<Button
									type="button"
									variant="ghost"
									className="absolute right-2 top-2 z-10 h-auto rounded-sm !ps-1.5 px-1 py-0.5 xs:text-xs leading-none font-normal text-muted-foreground hover:bg-muted/40 hover:text-foreground"
									onClick={(event) => dismissAlert(event, alert)}
									disabled={!!dismissing[alert.id]}
								>
									<Trans>Dismiss</Trans>
								</Button>
								<Link
									href={getPagePath($router, "system", { id: systems[alert.system]?.id })}
									className="absolute inset-0 w-full h-full"
									aria-label="View system"
								></Link>
							</Alert>
						)
					})}
				</div>
			</CardContent>
		</Card>
	)
}
