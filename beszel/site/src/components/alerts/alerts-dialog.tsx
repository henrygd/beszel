import { t } from "@lingui/core/macro"
import { Trans, Plural } from "@lingui/react/macro"
import { $alerts, $systems, pb } from "@/lib/stores"
import { alertInfo, cn, debounce } from "@/lib/utils"
import { Switch } from "@/components/ui/switch"
import { AlertInfo, AlertRecord, SystemRecord } from "@/types"
import { lazy, memo, Suspense, useMemo, useState } from "react"
import { toast } from "@/components/ui/use-toast"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { Checkbox } from "@/components/ui/checkbox"
import { DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { ServerIcon, GlobeIcon } from "lucide-react"
import { $router, Link } from "@/components/router"
import { DialogHeader } from "@/components/ui/dialog"

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
const upsertAlerts = debounce(
	async ({ name, value, min, systems }: { name: string; value: number; min: number; systems: string[] }) => {
		try {
			await pb.send<{ success: boolean }>(endpoint, {
				method: "POST",
				// overwrite is always true because we've done filtering client side
				body: { name, value, min, systems, overwrite: true },
			})
		} catch (error) {
			failedUpdateToast(error)
		}
	},
	alertDebounce
)

/** Delete alerts for a given name and systems */
const deleteAlerts = debounce(async ({ name, systems }: { name: string; systems: string[] }) => {
	try {
		await pb.send<{ success: boolean }>(endpoint, {
			method: "DELETE",
			body: { name, systems },
		})
	} catch (error) {
		failedUpdateToast(error)
	}
}, alertDebounce)

export const AlertDialogContent = memo(function AlertDialogContent({ system }: { system: SystemRecord }) {
	const alerts = useStore($alerts)
	const [overwriteExisting, setOverwriteExisting] = useState<boolean | "indeterminate">(false)
	const [currentTab, setCurrentTab] = useState("system")

	const systemAlerts = alerts[system.id] ?? new Map()

	// We need to keep a copy of alerts when we switch to global tab. If we always compare to
	// current alerts, it will only be updated when first checked, then won't be updated because
	// after that it exists.
	const alertsWhenGlobalSelected = useMemo(() => {
		return currentTab === "global" ? structuredClone(alerts) : alerts
	}, [currentTab])

	return (
		<>
			<DialogHeader>
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
						{system.name}
					</TabsTrigger>
					<TabsTrigger value="global">
						<GlobeIcon className="me-1.5 h-3.5 w-3.5" />
						<Trans>All Systems</Trans>
					</TabsTrigger>
				</TabsList>
				<TabsContent value="system">
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
	const [value, setValue] = useState(alert?.value || (singleDescription ? 0 : alertData.start ?? 80))

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
									<Trans>
										Average exceeds{" "}
										<strong className="text-foreground">
											{value}
											{alertData.unit}
										</strong>
									</Trans>
								</p>
								<div className="flex gap-3">
									<Slider
										aria-labelledby={`v${name}`}
										defaultValue={[value]}
										onValueCommit={(val) => sendUpsert(min, val[0])}
										onValueChange={(val) => setValue(val[0])}
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
									defaultValue={[min]}
									onValueCommit={(minVal) => sendUpsert(minVal[0], value)}
									onValueChange={(val) => setMin(val[0])}
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
