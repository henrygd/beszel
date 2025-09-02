import { t } from "@lingui/core/macro"
import { Trans, Plural } from "@lingui/react/macro"
import { $alerts, $systems } from "@/lib/stores"
import { cn, debounce } from "@/lib/utils"
import { alertInfo } from "@/lib/alerts"
import { Switch } from "@/components/ui/switch"
import { AlertInfo, AlertRecord, SystemRecord } from "@/types"
import { lazy, memo, Suspense, useMemo, useState, useEffect } from "react"
import { toast } from "@/components/ui/use-toast"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { Checkbox } from "@/components/ui/checkbox"
import { DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ServerIcon, GlobeIcon } from "lucide-react"
import { $router, Link } from "@/components/router"
import { DialogHeader } from "@/components/ui/dialog"
import { pb } from "@/lib/api"

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
	async ({ name, value, min, systems, repeat_interval, max_repeats, filesystem }: { 
		name: string; 
		value: number; 
		min: number; 
		systems: string[];
		repeat_interval?: number;
		max_repeats?: number;
		filesystem?: string;
	}) => {
		try {
			await pb.send<{ success: boolean }>(endpoint, {
				method: "POST",
				// overwrite is always true because we've done filtering client side
				body: { name, value, min, systems, overwrite: true, repeat_interval, max_repeats, filesystem },
			})
		} catch (error) {
			failedUpdateToast(error)
		}
	},
	alertDebounce
)

/** Delete alerts for a given name and systems */
const deleteAlerts = debounce(async ({ name, systems, filesystem }: { name: string; systems: string[]; filesystem?: string }) => {
	try {
		await pb.send<{ success: boolean }>(endpoint, {
			method: "DELETE",
			body: { name, systems, filesystem },
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
						<span className="truncate max-w-60">{system.name}</span>
					</TabsTrigger>
					<TabsTrigger value="global">
						<GlobeIcon className="me-1.5 h-3.5 w-3.5" />
						<Trans>All Systems</Trans>
					</TabsTrigger>
				</TabsList>
				<TabsContent value="system">
					<div className="grid gap-3">
						{alertKeys.map((name) => {
							if (name === 'Disk') {
								return <DiskAlertSection
									key={name}
									alertKey={name}
									data={alertInfo[name as keyof typeof alertInfo]}
									systemAlerts={systemAlerts}
									system={system}
								/>
							}
							return (
								<AlertContent
									key={name}
									alertKey={name}
									data={alertInfo[name as keyof typeof alertInfo]}
									alert={systemAlerts.get(name)}
									system={system}
								/>
							)
						})}
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
						{alertKeys.map((name) => {
							if (name === 'Disk') {
								return <DiskAlertSection
									key={name}
									alertKey={name}
									data={alertInfo[name as keyof typeof alertInfo]}
									systemAlerts={systemAlerts}
									system={system}
									global={true}
									overwriteExisting={!!overwriteExisting}
									initialAlertsState={alertsWhenGlobalSelected}
								/>
							}
							return (
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
							)
						})}
					</div>
				</TabsContent>
			</Tabs>
		</>
	)
})

function DiskAlertSection({
	alertKey,
	data: alertData,
	systemAlerts,
	system,
	global = false,
	overwriteExisting = false,
	initialAlertsState = {},
}: {
	alertKey: string
	data: AlertInfo
	systemAlerts: Map<string, AlertRecord>
	system: SystemRecord
	global?: boolean
	overwriteExisting?: boolean
	initialAlertsState?: Record<string, Map<string, AlertRecord>>
}) {
	const [extraFilesystems, setExtraFilesystems] = useState<string[]>([])
	
	// Fetch recent system stats to get actual extra filesystems
	useEffect(() => {
		const fetchExtraFilesystems = async () => {
			try {
				// Fetch recent system stats to get efs data
				const records = await pb.collection('system_stats').getList(1, 1, {
					filter: `system = '${system.id}' && type = '1m'`,
					sort: '-created',
				})
				
				if (records.items.length > 0) {
					const stats = records.items[0].stats as any
					if (stats.efs) {
						setExtraFilesystems(Object.keys(stats.efs))
					}
				}
			} catch (error) {
				console.warn('Could not fetch system stats for filesystem detection:', error)
			}
		}
		
		fetchExtraFilesystems()
	}, [system.id])

	// Get available filesystems: always root + extra filesystems from actual system data  
	const filesystems = useMemo(() => {
		const result = ['root'] // Always include root filesystem
		
		// Add actual extra filesystems from system stats
		result.push(...extraFilesystems)
		
		// Also add any filesystems from existing alerts that might not be in current stats
		for (const [, alertRecord] of systemAlerts) {
			if (alertRecord.name === 'Disk' && alertRecord.filesystem && !result.includes(alertRecord.filesystem)) {
				result.push(alertRecord.filesystem)
			}
		}
		
		return result
	}, [extraFilesystems, systemAlerts])

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
			<div className="p-4 pb-3">
				<div className="grid gap-1 select-none mb-3">
					<p className="font-semibold flex gap-3 items-center">
						<alertData.icon className="h-4 w-4 opacity-85" /> {alertData.name()}
					</p>
					<span className="block text-sm text-muted-foreground">{alertData.desc()}</span>
				</div>
				
				<div className="grid gap-3">
					{filesystems.map(filesystem => {
						// Look for filesystem-specific alert or legacy disk alert for root
						let fsAlert: AlertRecord | undefined
						for (const [, alertRecord] of systemAlerts) {
							if (alertRecord.name === alertKey) {
								if (alertRecord.filesystem === filesystem || 
									(filesystem === 'root' && !alertRecord.filesystem)) {
									fsAlert = alertRecord
									break
								}
							}
						}
						
						return (
							<AlertContent
								key={`${alertKey}-${filesystem}`}
								alertKey={alertKey}
								data={{...alertData, name: () => `${alertData.name()} (${filesystem})`}}
								alert={fsAlert}
								system={system}
								global={global}
								overwriteExisting={overwriteExisting}
								initialAlertsState={initialAlertsState}
								filesystem={filesystem}
							/>
						)
					})}
				</div>
			</div>
		</div>
	)
}

export function AlertContent({
	alertKey,
	data: alertData,
	system,
	alert,
	global = false,
	overwriteExisting = false,
	initialAlertsState = {},
	filesystem,
}: {
	alertKey: string
	data: AlertInfo
	system: SystemRecord
	alert?: AlertRecord
	global?: boolean
	overwriteExisting?: boolean
	initialAlertsState?: Record<string, Map<string, AlertRecord>>
	filesystem?: string
}) {
	const { name } = alertData

	const singleDescription = alertData.singleDesc?.()

	const [checked, setChecked] = useState(global ? false : !!alert)
	const [min, setMin] = useState(alert?.min || 10)
	
	// For bandwidth alerts with units, we need to parse the stored value
	const initValue = alert?.value || (singleDescription ? 0 : alertData.start ?? 80)
	const [selectedUnit, setSelectedUnit] = useState(() => {
		if (alertData.hasUnits && alertData.units) {
			// Find the best unit for the stored value
			const sortedUnits = [...alertData.units].sort((a, b) => b.multiplier - a.multiplier)
			for (const unit of sortedUnits) {
				if (initValue >= unit.multiplier) {
					return unit.label
				}
			}
			return alertData.units[0].label
		}
		return ""
	})
	const [value, setValue] = useState(() => {
		if (alertData.hasUnits && alertData.units) {
			const unit = alertData.units.find(u => u.label === selectedUnit)
			return unit ? initValue / unit.multiplier : initValue
		}
		return initValue
	})
	
	const [repeatInterval, setRepeatInterval] = useState(alert?.repeat_interval || 0)
	const [maxRepeats, setMaxRepeats] = useState(alert?.max_repeats || 0)

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

	function sendUpsert(min: number, value: number, repeat_interval?: number, max_repeats?: number, unit?: string) {
		// Convert value to base unit if this alert has units
		let finalValue = value
		if (alertData.hasUnits && alertData.units) {
			const selectedUnitData = alertData.units.find(u => u.label === (unit || selectedUnit))
			if (selectedUnitData) {
				finalValue = value * selectedUnitData.multiplier
			}
		}
		
		const systems = getSystemIds()
		systems.length &&
			upsertAlerts({
				name: alertKey,
				value: finalValue,
				min,
				systems,
				repeat_interval: repeat_interval ?? repeatInterval,
				max_repeats: max_repeats ?? maxRepeats,
				filesystem,
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
							deleteAlerts({ name: alertKey, systems: getSystemIds(), filesystem })
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
											{alertData.hasUnits ? selectedUnit : alertData.unit}
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
									{alertData.hasUnits && alertData.units && (
										<Select value={selectedUnit} onValueChange={(newUnit) => {
											// Convert current value to new unit
											const currentUnitData = alertData.units!.find(u => u.label === selectedUnit)
											const newUnitData = alertData.units!.find(u => u.label === newUnit)
											if (currentUnitData && newUnitData) {
												const baseValue = value * currentUnitData.multiplier
												const newValue = baseValue / newUnitData.multiplier
												setValue(newValue)
												setSelectedUnit(newUnit)
												sendUpsert(min, newValue, undefined, undefined, newUnit)
											}
										}}>
											<SelectTrigger className="w-20 h-8">
												<SelectValue />
											</SelectTrigger>
											<SelectContent>
												{alertData.units.map((unit) => (
													<SelectItem key={unit.label} value={unit.label}>
														{unit.label}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
									)}
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
						<div className="col-span-full">
							<p id={`r${name}`} className="text-sm block h-8">
								<Trans>
									{repeatInterval === 0 ? (
										<span>No repeat notifications</span>
									) : (
										<span>
											Repeat every <strong className="text-foreground">{repeatInterval}</strong>{" "}
											<Plural value={repeatInterval} one="minute" other="minutes" />
										</span>
									)}
								</Trans>
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`r${name}`}
									defaultValue={[repeatInterval]}
									onValueCommit={(val) => sendUpsert(min, value, val[0])}
									onValueChange={(val) => setRepeatInterval(val[0])}
									min={0}
									max={1440}
									step={5}
								/>
							</div>
						</div>
						{repeatInterval > 0 && (
							<div className="col-span-full">
								<p id={`mr${name}`} className="text-sm block h-8">
									<Trans>
										{maxRepeats === 0 ? (
											<span>Unlimited repeat notifications</span>
										) : (
											<span>
												Maximum <strong className="text-foreground">{maxRepeats}</strong> repeats
											</span>
										)}
									</Trans>
								</p>
								<div className="flex gap-3">
									<Slider
										aria-labelledby={`mr${name}`}
										defaultValue={[maxRepeats]}
										onValueCommit={(val) => sendUpsert(min, value, repeatInterval, val[0])}
										onValueChange={(val) => setMaxRepeats(val[0])}
										min={0}
										max={20}
									/>
								</div>
							</div>
						)}
					</Suspense>
				</div>
			)}
		</div>
	)
}
