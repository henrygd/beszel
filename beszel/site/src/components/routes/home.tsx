import { Suspense, lazy, memo, useEffect, useMemo } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card"
import { $alerts, $systems, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { GithubIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { getSystemNameFromId, updateRecordList, updateSystemList } from "@/lib/utils"
import { AlertRecord, SystemRecord } from "@/types"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { $router, Link } from "../router"
import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { getPagePath } from "@nanostores/router"
import { alertInfo } from "@/lib/alerts"

const SystemsTable = lazy(() => import("../systems-table/systems-table"))

export const Home = memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = t`Dashboard` + " / Beszel"
	}, [t])

	useEffect(() => {
		// make sure we have the latest list of systems
		updateSystemList()

		// subscribe to real time updates for systems / alerts
		pb.collection<SystemRecord>("systems").subscribe("*", (e) => {
			updateRecordList(e, $systems)
		})
		return () => {
			pb.collection("systems").unsubscribe("*")
		}
	}, [])

	return useMemo(
		() => (
			<>
				<ActiveAlerts />
				<Suspense>
					<SystemsTable />
				</Suspense>

				<div className="flex gap-1.5 justify-end items-center pe-3 sm:pe-6 mt-3.5 text-xs opacity-80">
					<a
						href="https://github.com/henrygd/beszel"
						target="_blank"
						className="flex items-center gap-0.5 text-muted-foreground hover:text-foreground duration-75"
					>
						<GithubIcon className="h-3 w-3" /> GitHub
					</a>
					<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
					<a
						href="https://github.com/henrygd/beszel/releases"
						target="_blank"
						className="text-muted-foreground hover:text-foreground duration-75"
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
										className="hover:-translate-y-px duration-200 bg-transparent border-foreground/10  hover:shadow-md shadow-black"
									>
										<info.icon className="h-4 w-4" />
										<AlertTitle>
											{getSystemNameFromId(alert.system)} {info.name().toLowerCase().replace("cpu", "CPU")}
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
											href={getPagePath($router, "system", { name: getSystemNameFromId(alert.system) })}
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
