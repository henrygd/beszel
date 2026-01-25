import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { GlobeIcon, ServerIcon } from "lucide-react"
import { useEffect, lazy, memo, Suspense, useMemo, useState } from "react"
import { $router, Link } from "@/components/router"
import { Checkbox } from "@/components/ui/checkbox"
import { DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Switch } from "@/components/ui/switch"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { SearchIcon } from "lucide-react"
import { DockerIcon } from "@/components/ui/icons"
import { Input } from "@/components/ui/input"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { toast } from "@/components/ui/use-toast"
import { alertInfo } from "@/lib/alerts"
import { pb } from "@/lib/api"
import { $alerts, $systems } from "@/lib/stores"
import { cn, debounce } from "@/lib/utils"
import type { AlertInfo, AlertRecord, SystemRecord } from "@/types"

const Slider = lazy(() => import("@/components/ui/slider"))

const endpoint = "/api/beszel/user-alerts"

const alertDebounce = 100

const alertKeys = Object.keys(alertInfo) as (keyof typeof alertInfo)[]

const failedUpdateToast = (error: unknown) => {
	console.error(error)
	toast({
		title: t`Failed to update alert`,
		description: t`Please check logs for more details.`,
		variant: "destructive",
	})
}

/** Create or update alerts for a given name and systems */
async function upsertAlertsSync({ name, value, min, systems }: { name: string; value: number; min: number; systems: string[] }) {
	try {
		await pb.send<{ success: boolean }>(endpoint, {
			method: "POST",
			// overwrite is always true because we've done filtering client side
			body: { name, value, min, systems, overwrite: true },
			requestKey: null,
		})
	} catch (error) {
		failedUpdateToast(error)
	}
}

/** Delete alerts for a given name and systems */
async function deleteAlertsSync({ name, systems }: { name: string; systems: string[] }) {
	try {
		await pb.send<{ success: boolean }>(endpoint, {
			method: "DELETE",
			body: { name, systems },
			requestKey: null,
		})
	} catch (error) {
		failedUpdateToast(error)
	}
}

const upsertAlerts = debounce(upsertAlertsSync, alertDebounce)
const deleteAlerts = debounce(deleteAlertsSync, alertDebounce)

const fetchContainers = async (systemId: string) => {
	const { items } = await pb.collection("containers").getList(0, 500, {
		fields: "id,name,system",
		filter: pb.filter("system={:system}", { system: systemId }),
	})
	return items.sort((a: any, b: any) => a.name.localeCompare(b.name)) as any[]
}

export const AlertDialogContent = memo(function AlertDialogContent({ system }: { system: SystemRecord }) {
	const alerts = useStore($alerts)
	const [overwriteExisting, setOverwriteExisting] = useState<boolean | "indeterminate">(false)
	const [currentTab, setCurrentTab] = useState("system")
	const [containers, setContainers] = useState<{ id: string; name: string }[]>([])
	const [containerSearch, setContainerSearch] = useState("")

	useEffect(() => {
		fetchContainers(system.id).then(setContainers)
	}, [system.id])

	const systemAlerts = alerts[system.id] ?? new Map()

	const getContainerAlertInfo = (name: string): AlertInfo => ({
		name: () => name,
		unit: "",
		icon: DockerIcon,
		desc: () => t`Triggers when container is unhealthy`,
		singleDesc: () => t`Unhealthy`,
		start: 1,
		invert: true,
	})

	const alertsWhenGlobalSelected = useMemo(() => {
		return currentTab === "global" ? structuredClone(alerts) : alerts
	}, [currentTab, alerts])

	const [bulkEnabled, setBulkEnabled] = useState(false)
	const [bulkMin, setBulkMin] = useState(10)

	useEffect(() => {
		if (containers.length > 0) {
			const enabledCount = containers.filter((c) => systemAlerts.has(`Container ${c.name}`)).length
			const allEnabled = enabledCount === containers.length
			// Only update global state if we have exactly one container OR if we just loaded multiple and they are all enabled
			if (containers.length === 1 || (!bulkEnabled && allEnabled)) {
				setBulkEnabled(allEnabled)
				const alert = systemAlerts.get(`Container ${containers[0].name}`)
				if (alert) {
					setBulkMin(alert.min)
				}
			}
		}
	}, [containers.length, systemAlerts])

	const filteredContainers = useMemo(() => {
		return containers.filter((c) => c.name.toLowerCase().includes(containerSearch.toLowerCase()))
	}, [containers, containerSearch])

	const updateAllContainersMin = useMemo(
		() =>
			debounce(async (val: number) => {
				for (const c of filteredContainers) {
					if (systemAlerts.has(`Container ${c.name}`)) {
						await upsertAlertsSync({
							name: `Container ${c.name}`,
							value: 1,
							min: val,
							systems: [system.id],
						})
					}
				}
			}, 200),
		[filteredContainers, system.id, systemAlerts]
	)

	return (
		<>
			<DialogHeader className="px-1">
				<DialogTitle className="text-xl">
					<Trans>Alerts</Trans>
				</DialogTitle>
				<DialogDescription>
					<Trans>
						See{" "}
						<Link href={getPagePath($router, "settings", { name: "notifications" })} className="link">
							notification settings
						</Link>{" "}
						to configure how you receive alerts.
					</Trans>
				</DialogDescription>
			</DialogHeader>
			<Tabs defaultValue="system" onValueChange={setCurrentTab}>
				<TabsList className="mb-1 -mt-0.5">
					<TabsTrigger value="system">
						<ServerIcon className="me-2 h-3.5 w-3.5" />
						<span className="truncate max-w-60">{system.name}</span>
					</TabsTrigger>
					<TabsTrigger value="global">
						<GlobeIcon className="me-1.5 h-3.5 w-3.5" />
						<Trans>All Systems</Trans>
					</TabsTrigger>
				</TabsList>
				<TabsContent value="system" className="mt-4 outline-none">
					<div className="grid gap-3">
						{alertKeys.map((name) => (
							<AlertContent
								key={name}
								alertKey={name}
								data={alertInfo[name as keyof typeof alertInfo]}
								alert={systemAlerts.get(name)}
								system={system}
							/>
						))}
						<Separator className="my-2" />
						{/* Container Alerts */}
						<div className="space-y-3">
							<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group p-4">
								<div className="flex flex-row items-center justify-between gap-4 select-none">
									<div className="grid gap-1">
										<p className="font-semibold flex gap-2.5 items-center">
											<DockerIcon className="h-4 w-4 opacity-85" />
											<Trans>All Containers</Trans>
										</p>
										<span className="block text-sm text-muted-foreground">
											{bulkEnabled ? (
												<Trans>
													Any container unhealthy for <strong className="text-foreground">{bulkMin}</strong>{" "}
													<Plural value={bulkMin} one="minute" other="minutes" />
												</Trans>
											) : (
												<Trans>Set unhealthy alert for all containers</Trans>
											)}
										</span>
									</div>
									<Switch
										checked={bulkEnabled}
										onCheckedChange={async (newChecked) => {
											setBulkEnabled(newChecked)
											if (newChecked) {
												for (const c of filteredContainers) {
													await upsertAlertsSync({
														name: `Container ${c.name}`,
														value: 1,
														min: bulkMin,
														systems: [system.id],
													})
												}
											} else {
												const alertNames = filteredContainers.map((c) => `Container ${c.name}`)
												for (const name of alertNames) {
													await deleteAlertsSync({ name, systems: [system.id] })
												}
											}
										}}
									/>
								</div>
								{bulkEnabled && (
									<div className="mt-4 pt-1 mb-1">
										<Suspense fallback={<div className="h-4 w-full" />}>
											<Slider
												value={[bulkMin]}
												onValueChange={(val) => {
													setBulkMin(val[0])
													updateAllContainersMin(val[0])
												}}
												min={1}
												max={60}
											/>
										</Suspense>
									</div>
								)}
							</div>

							<div className="relative">
								<SearchIcon className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground/60" />
								<Input
									placeholder={t`Search containers...`}
									className="pl-9 h-9 bg-muted/40 border-muted-foreground/10"
									value={containerSearch}
									onChange={(e) => setContainerSearch(e.target.value)}
								/>
							</div>

							<div className="grid gap-3 max-h-[350px] overflow-y-auto pr-1 scroll-sm">
								{filteredContainers.map((c) => {
									const alertKey = `Container ${c.name}`
									return (
										<AlertContent
											key={c.id}
											alertKey={alertKey}
											data={getContainerAlertInfo(c.name)}
											alert={systemAlerts.get(alertKey)}
											system={system}
										/>
									)
								})}
								{containers.length > 0 && filteredContainers.length === 0 && (
									<p className="text-center py-4 text-sm text-muted-foreground italic">
										<Trans>No containers found</Trans>
									</p>
								)}
							</div>
						</div>
					</div>
				</TabsContent>
				<TabsContent value="global">
					<label
						htmlFor="ovw"
						className="mb-3 flex gap-2 items-center justify-center cursor-pointer border rounded-sm py-3 px-4 border-destructive text-destructive font-semibold text-sm"
					>
						<Checkbox
							id="ovw"
							className="text-destructive border-destructive data-[state=checked]:bg-destructive"
							checked={overwriteExisting}
							onCheckedChange={setOverwriteExisting}
						/>
						<Trans>Overwrite existing alerts</Trans>
					</label>
					<div className="grid gap-3">
						{alertKeys.map((name) => (
							<AlertContent
								key={name}
								alertKey={name}
								system={system}
								alert={systemAlerts.get(name)}
								data={alertInfo[name as keyof typeof alertInfo]}
								global={true}
								overwriteExisting={!!overwriteExisting}
								initialAlertsState={alertsWhenGlobalSelected}
							/>
						))}
					</div>
				</TabsContent>
			</Tabs>
		</>
	)
})

export function AlertContent({
	alertKey,
	data: alertData,
	system,
	alert,
	global = false,
	overwriteExisting = false,
	initialAlertsState = {},
}: {
	alertKey: string
	data: AlertInfo
	system: SystemRecord
	alert?: AlertRecord
	global?: boolean
	overwriteExisting?: boolean
	initialAlertsState?: Record<string, Map<string, AlertRecord>>
}) {
	const { name } = alertData

	const singleDescription = alertData.singleDesc?.()

	const [checked, setChecked] = useState(global ? false : !!alert)
	const [min, setMin] = useState(alert?.min || 10)
	const [value, setValue] = useState(alert?.value ?? (singleDescription ? (alertData.start ?? 0) : (alertData.start ?? 80)))

	useEffect(() => {
		if (!global) {
			setChecked(!!alert)
		}
	}, [alert, global])

	useEffect(() => {
		if (alert?.min) {
			setMin(alert.min)
		}
	}, [alert?.min])

	useEffect(() => {
		if (alert?.value !== undefined) {
			setValue(alert.value)
		}
	}, [alert?.value])

	const Icon = alertData.icon

	/** Get system ids to update */
	function getSystemIds(): string[] {
		// if not global, update only the current system
		if (!global) {
			return [system.id]
		}
		// if global, update all systems when overwriteExisting is true
		// update only systems without an existing alert when overwriteExisting is false
		const allSystems = $systems.get()
		const systemIds: string[] = []
		for (const system of allSystems) {
			if (overwriteExisting || !initialAlertsState[system.id]?.has(alertKey)) {
				systemIds.push(system.id)
			}
		}
		return systemIds
	}

	function sendUpsert(min: number, value: number) {
		const systems = getSystemIds()
		systems.length &&
			upsertAlerts({
				name: alertKey,
				value,
				min,
				systems,
			})
	}

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
			<label
				htmlFor={`s${name}`}
				className={cn("flex flex-row items-center justify-between gap-4 cursor-pointer p-4", {
					"pb-0": checked,
				})}
			>
				<div className="grid gap-1 select-none">
					<p className="font-semibold flex gap-3 items-center">
						<Icon className="h-4 w-4 opacity-85" /> {alertData.name()}
					</p>
					{!checked && <span className="block text-sm text-muted-foreground">{alertData.desc()}</span>}
				</div>
				<Switch
					id={`s${name}`}
					checked={checked}
					onCheckedChange={(newChecked) => {
						setChecked(newChecked)
						if (newChecked) {
							// if alert checked, create or update alert
							sendUpsert(min, value)
						} else {
							// if unchecked, delete alert (unless global and overwriteExisting is false)
							deleteAlerts({ name: alertKey, systems: getSystemIds() })
							// when force deleting all alerts of a type, also remove them from initialAlertsState
							if (overwriteExisting) {
								for (const curAlerts of Object.values(initialAlertsState)) {
									curAlerts.delete(alertKey)
								}
							}
						}
					}}
				/>
			</label>
			{checked && (
				<div className="grid sm:grid-cols-2 mt-1.5 gap-5 px-4 pb-5 tabular-nums text-muted-foreground">
					<Suspense fallback={<div className="h-10" />}>
						{!singleDescription && (
							<div>
								<p id={`v${name}`} className="text-sm block h-8">
									{alertData.invert ? (
										<Trans>
											Average drops below{" "}
											<strong className="text-foreground">
												{value}
												{alertData.unit}
											</strong>
										</Trans>
									) : (
										<Trans>
											Average exceeds{" "}
											<strong className="text-foreground">
												{value}
												{alertData.unit}
											</strong>
										</Trans>
									)}
								</p>
								<div className="flex gap-3">
									<Slider
										aria-labelledby={`v${name}`}
										value={[value]}
										onValueChange={(val) => {
											setValue(val[0])
											sendUpsert(min, val[0])
										}}
										step={alertData.step ?? 1}
										min={alertData.min ?? 1}
										max={alertData.max ?? 99}
									/>
								</div>
							</div>
						)}
						<div className={cn(singleDescription && "col-span-full lowercase")}>
							<p id={`t${name}`} className="text-sm block h-8 first-letter:uppercase">
								{singleDescription && (
									<>
										{singleDescription}
										{` `}
									</>
								)}
								<Trans>
									For <strong className="text-foreground">{min}</strong>{" "}
									<Plural value={min} one="minute" other="minutes" />
								</Trans>
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`v${name}`}
									value={[min]}
									onValueChange={(val) => {
										setMin(val[0])
										sendUpsert(val[0], value)
									}}
									min={1}
									max={60}
								/>
							</div>
						</div>
					</Suspense>
				</div>
			)}
		</div>
	)
}
