import { Suspense, lazy, memo, useEffect, useMemo } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card"
import { $alerts, $systems, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { GithubIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { alertInfo, updateRecordList, updateSystemList } from "@/lib/utils"
import { AlertRecord, SystemRecord } from "@/types"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { $router, Link } from "../router"
import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { getPagePath } from "@nanostores/router"

const SystemsTable = lazy(() => import("../systems-table/systems-table"))

export const Home = memo(() => {
	const alerts = useStore($alerts)
	const systems = useStore($systems)
	const { t } = useLingui()

	/* key to prevent re-rendering of active alerts */
	const alertsKey: string[] = []

	const activeAlerts = useMemo(() => {
		const activeAlerts = alerts.filter((alert) => {
			const active = alert.triggered && alert.name in alertInfo
			if (!active) {
				return false
			}
			alert.sysname = systems.find((system) => system.id === alert.system)?.name
			alertsKey.push(alert.id)
			return true
		})
		return activeAlerts
	}, [systems, alerts])

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
		pb.collection<AlertRecord>("alerts").subscribe("*", (e) => {
			updateRecordList(e, $alerts)
		})
		return () => {
			pb.collection("systems").unsubscribe("*")
			// pb.collection('alerts').unsubscribe('*')
		}
	}, [])

	return useMemo(
		() => (
			<>
				{/* show active alerts */}
				{activeAlerts.length > 0 && <ActiveAlerts key={activeAlerts.length} activeAlerts={activeAlerts} />}
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
		[alertsKey.join("")]
	)
})

const ActiveAlerts = memo(({ activeAlerts }: { activeAlerts: AlertRecord[] }) => {
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
									className="hover:-translate-y-[1px] duration-200 bg-transparent border-foreground/10  hover:shadow-md shadow-black"
								>
									<info.icon className="h-4 w-4" />
									<AlertTitle>
										{alert.sysname} {info.name().toLowerCase().replace("cpu", "CPU")}
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
										href={getPagePath($router, "system", { name: alert.sysname! })}
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
})
