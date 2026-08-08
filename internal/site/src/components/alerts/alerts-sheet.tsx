import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { ChevronDownIcon, GlobeIcon, ServerIcon } from "lucide-react"
import { lazy, memo, Suspense, useEffect, useMemo, useRef, useState } from "react"
import { $router, Link } from "@/components/router"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { toast } from "@/components/ui/use-toast"
import { alertInfo } from "@/lib/alerts"
import { pb } from "@/lib/api"
import { $alerts, $systems } from "@/lib/stores"
import { temperatureSensorNames, temperatureThresholdInput } from "@/lib/temperature-alerts"
import { cn, debounceByKey } from "@/lib/utils"
import type { AlertInfo, AlertRecord, AlertUnit, SystemRecord, SystemStatsRecord } from "@/types"

const Slider = lazy(() => import("@/components/ui/slider"))

const endpoint = "/api/beszel/user-alerts"

const alertDebounce = 400

/** Class names for the SheetContent that wraps AlertDialogContent */
export const alertsSheetClassName = "max-h-full overflow-auto w-160 !max-w-full p-4 sm:p-6"

const alertKeys = Object.keys(alertInfo) as (keyof typeof alertInfo)[]

/** Round to a multiple of step, trimming float noise like 0.30000000000000004 */
const roundToStep = (val: number, step: number) => Number((Math.round(val / step) * step).toFixed(6))

/** Number input step and minimum for a unit, which may be finer than the slider */
const getInputRange = ({ min, step, inputStep = step }: AlertUnit) => ({ step: inputStep, min: Math.min(min, inputStep) })

/** Index of the unit to display a stored value in: the largest unit whose input range and step fit it */
function getUnitIndex(units: AlertUnit[], value: number) {
	for (let i = units.length - 1; i > 0; i--) {
		const { min, step } = getInputRange(units[i])
		const val = value / units[i].factor
		if (val >= min && val <= units[i].max && Math.abs(roundToStep(val, step) - val) < 1e-6) {
			return i
		}
	}
	return 0
}

const failedUpdateToast = (error: unknown) => {
	console.error(error)
	toast({
		title: t`Failed to update alert`,
		description: t`Please check logs for more details.`,
		variant: "destructive",
	})
}

/** Create or update alerts for a given name and systems (debounced per alert name) */
const upsertAlerts = debounceByKey(
	async ({
		name,
		value,
		thresholds,
		min,
		systems,
	}: {
		name: string
		value: number
		thresholds?: Record<string, number>
		min: number
		systems: string[]
	}) => {
		try {
			await pb.send<{ success: boolean }>(endpoint, {
				method: "POST",
				// overwrite is always true because we've done filtering client side
				body: { name, value, thresholds, min, systems, overwrite: true },
				// scope auto-cancellation per alert so a request for one alert doesn't abort another's
				requestKey: `POST:${name}`,
			})
		} catch (error) {
			failedUpdateToast(error)
		}
	},
	alertDebounce
)

/** Delete alerts for a given name and systems (debounced per alert name) */
const deleteAlerts = debounceByKey(async ({ name, systems }: { name: string; systems: string[] }) => {
	try {
		await pb.send<{ success: boolean }>(endpoint, {
			method: "DELETE",
			body: { name, systems },
			requestKey: `DELETE:${name}`,
		})
	} catch (error) {
		failedUpdateToast(error)
	}
}, alertDebounce)

export const AlertDialogContent = memo(function AlertDialogContent({ system }: { system: SystemRecord }) {
	const alerts = useStore($alerts)
	const systems = useStore($systems)
	const [overwriteExisting, setOverwriteExisting] = useState<boolean | "indeterminate">(false)
	const [currentTab, setCurrentTab] = useState("system")
	// copyKey is used to force remount AlertContent components with
	// new alert data after copying alerts from another system
	const [copyKey, setCopyKey] = useState(0)

	const systemAlerts = alerts[system.id] ?? new Map()

	// Systems that have at least one alert configured (excluding the current system)
	const systemsWithAlerts = useMemo(
		() => systems.filter((s) => s.id !== system.id && alerts[s.id]?.size),
		[systems, alerts, system.id]
	)

	async function copyAlertsFromSystem(sourceSystemId: string) {
		const sourceAlerts = $alerts.get()[sourceSystemId]
		if (!sourceAlerts?.size) return
		try {
			const currentTargetAlerts = $alerts.get()[system.id] ?? new Map()
			// Alert names present on target but absent from source should be deleted
			const namesToDelete = Array.from(currentTargetAlerts.keys()).filter((name) => !sourceAlerts.has(name))
			await Promise.all([
				...Array.from(sourceAlerts.values()).map(({ name, value, thresholds, min }) =>
					pb.send<{ success: boolean }>(endpoint, {
						method: "POST",
						body: { name, value, thresholds, min, systems: [system.id], overwrite: true },
						requestKey: name,
					})
				),
				...namesToDelete.map((name) =>
					pb.send<{ success: boolean }>(endpoint, {
						method: "DELETE",
						body: { name, systems: [system.id] },
						requestKey: name,
					})
				),
			])
			// Optimistically update the store so components re-mount with correct data
			// before the realtime subscription event arrives.
			const newSystemAlerts = new Map<string, AlertRecord>()
			for (const alert of sourceAlerts.values()) {
				newSystemAlerts.set(alert.name, { ...alert, system: system.id, triggered: false })
			}
			$alerts.setKey(system.id, newSystemAlerts)
			setCopyKey((k) => k + 1)
		} catch (error) {
			failedUpdateToast(error)
		}
	}

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
				<div className="flex items-center justify-between mb-1 -mt-0.5">
					<TabsList>
						<TabsTrigger value="system">
							<ServerIcon className="me-2 h-3.5 w-3.5" />
							<span className="truncate max-w-60">{system.name}</span>
						</TabsTrigger>
						<TabsTrigger value="global">
							<GlobeIcon className="me-1.5 h-3.5 w-3.5" />
							<Trans>All Systems</Trans>
						</TabsTrigger>
					</TabsList>
					{systemsWithAlerts.length > 0 && currentTab === "system" && (
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="ghost" size="sm" className="text-muted-foreground text-xs gap-1.5">
									<Trans context="Copy alerts from another system">Copy from</Trans>
									<ChevronDownIcon className="h-3.5 w-3.5" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" className="max-h-100 overflow-auto">
								{systemsWithAlerts.map((s) => (
									<DropdownMenuItem key={s.id} className="min-w-44" onSelect={() => copyAlertsFromSystem(s.id)}>
										{s.name}
									</DropdownMenuItem>
								))}
							</DropdownMenuContent>
						</DropdownMenu>
					)}
				</div>
				<TabsContent value="system">
					<div key={copyKey} className="grid gap-3">
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
	/** Alerts that fire on first observation have no duration to configure */
	const noDuration = alertData.noDuration === true
	/** Binary alerts have no threshold to configure */
	const noThreshold = !!singleDescription || alertData.noThreshold === true
	/** Whether enabling the alert reveals anything to configure */
	const hasControls = !(noThreshold && noDuration)

	const [checked, setChecked] = useState(global ? false : !!alert)
	const [min, setMin] = useState(alert?.min || (noDuration ? 0 : 10))
	const { units } = alertData
	const storedValue = alert?.value ?? (noThreshold ? 0 : (alertData.start ?? 80))
	const [unitIndex, setUnitIndex] = useState(() => (units ? getUnitIndex(units, storedValue) : 0))
	/** Selected threshold unit, if the alert has selectable units */
	const unit = units?.[unitIndex]
	const factor = unit?.factor ?? 1
	const unitLabel = unit?.unit ?? alertData.unit
	const { min: valueMin, max: valueMax, step = 1 } = unit ?? alertData
	const { min: inputMin, step: inputStep } = unit ? getInputRange(unit) : { min: valueMin, step }
	/** Threshold value in the selected unit */
	const [value, setValue] = useState(() => Number((storedValue / factor).toFixed(6)))
	const [temperatureMode, setTemperatureMode] = useState<"any" | "sensors">(
		Object.keys(alert?.thresholds ?? {}).length ? "sensors" : "any"
	)
	const [sensorThresholds, setSensorThresholds] = useState<Record<string, number>>(alert?.thresholds ?? {})
	const sensorThresholdsRef = useRef(sensorThresholds)
	const [temperatureSensors, setTemperatureSensors] = useState(() => Object.keys(alert?.thresholds ?? {}).sort())
	const [loadingTemperatureSensors, setLoadingTemperatureSensors] = useState(false)
	const isTemperatureAlert = alertKey === "Temperature"
	const canConfigureTemperatureSensors = isTemperatureAlert && !global

	const Icon = alertData.icon

	function updateSensorThresholds(
		update: Record<string, number> | ((currentThresholds: Record<string, number>) => Record<string, number>)
	) {
		const nextThresholds = typeof update === "function" ? update(sensorThresholdsRef.current) : update
		sensorThresholdsRef.current = nextThresholds
		setSensorThresholds(nextThresholds)
		return nextThresholds
	}

	useEffect(() => {
		if (!canConfigureTemperatureSensors || !checked) return
		let cancelled = false
		setLoadingTemperatureSensors(true)
		pb.collection<SystemStatsRecord>("system_stats")
			.getList(1, 1, {
				filter: pb.filter("system={:system} && type={:type}", { system: system.id, type: "1m" }),
				fields: "stats",
				sort: "-created",
			})
			.then(({ items }) => {
				if (cancelled) return
				setTemperatureSensors(temperatureSensorNames(sensorThresholds, items[0]?.stats?.t))
			})
			.catch((error) => console.error("Failed to load temperature sensors", error))
			.finally(() => !cancelled && setLoadingTemperatureSensors(false))
		return () => {
			cancelled = true
		}
	}, [canConfigureTemperatureSensors, checked, system.id])

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

	function sendUpsert(min: number, value: number, thresholds?: Record<string, number>, valueFactor = factor) {
		const systems = getSystemIds()
		systems.length &&
			upsertAlerts(alertKey, {
				name: alertKey,
				value: Number((value * valueFactor).toFixed(6)),
				thresholds:
					thresholds ??
					(isTemperatureAlert
						? canConfigureTemperatureSensors && temperatureMode === "sensors"
							? sensorThresholdsRef.current
							: {}
						: undefined),
				min,
				systems,
			})
	}

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
			<label
				htmlFor={`s${name}`}
				className={cn("flex flex-row items-center justify-between gap-4 cursor-pointer p-4", {
					"pb-0": checked && hasControls,
				})}
			>
				<div className="grid gap-1 select-none">
					<p className="font-semibold flex gap-3 items-center">
						<Icon className="h-4 w-4 opacity-85" /> {alertData.name()}
					</p>
					{(!checked || !hasControls) && (
						<span className="block text-sm text-muted-foreground">{alertData.desc()}</span>
					)}
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
							deleteAlerts(alertKey, { name: alertKey, systems: getSystemIds() })
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
			{checked && hasControls && (
				<div className="grid sm:grid-cols-2 mt-1.5 gap-5 px-4 pb-5 tabular-nums text-muted-foreground">
					<Suspense fallback={<div className="h-10" />}>
						{canConfigureTemperatureSensors && (
							<div className="col-span-full">
								<p className="text-sm block h-6">
									<Trans>Temperature source</Trans>
								</p>
								<Select
									value={temperatureMode}
									onValueChange={(mode: "any" | "sensors") => {
										setTemperatureMode(mode)
										if (mode === "any") {
											sendUpsert(min, value, {})
											return
										}
										const currentThresholds = sensorThresholdsRef.current
										const nextThresholds = Object.keys(currentThresholds).length
											? currentThresholds
											: Object.fromEntries(temperatureSensors.map((sensor) => [sensor, value]))
										updateSensorThresholds(nextThresholds)
										if (Object.keys(nextThresholds).length) sendUpsert(min, value, nextThresholds)
									}}
								>
									<SelectTrigger className="h-9" aria-label={t`Temperature source`}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="any">
											<Trans>Any sensor</Trans>
										</SelectItem>
										<SelectItem value="sensors" disabled={!temperatureSensors.length}>
											<Trans>Individual sensors</Trans>
										</SelectItem>
									</SelectContent>
								</Select>
							</div>
						)}
						{!noThreshold && (!canConfigureTemperatureSensors || temperatureMode === "any") && (
							<div className={cn(noDuration && "col-span-full")}>
								<p id={`v${name}`} className="text-sm block h-6">
									{alertKey === "NetworkMonitorCert" ? (
										<Trans>
											Expires within{" "}
											<strong className="text-foreground">
												<Plural value={value} one="# day" other="# days" />
											</strong>
										</Trans>
									) : alertData.invert ? (
										<Trans>
											Average drops below{" "}
											<strong className="text-foreground">
												{value}
												{unitLabel}
											</strong>
										</Trans>
									) : (
										<Trans>
											Average exceeds{" "}
											<strong className="text-foreground">
												{value}
												{unitLabel}
											</strong>
										</Trans>
									)}
								</p>
								<div className="flex gap-3 items-center">
									<Slider
										aria-labelledby={`v${name}`}
										value={[value]}
										onValueCommit={(val) => sendUpsert(min, val[0])}
										onValueChange={(val) => setValue(val[0])}
										step={step}
										min={valueMin ?? 1}
										max={valueMax ?? 99}
									/>
									<Input
										type="number"
										value={value}
										onChange={(e) => {
											let val = parseFloat(e.target.value)
											if (!Number.isNaN(val)) {
												// keep values on the step grid so the unit can be inferred on reload
												if (unit) val = roundToStep(val, inputStep)
												// certificate thresholds are whole days
												else if (alertKey === "NetworkMonitorCert") val = Math.round(val)
												if (valueMax != null) val = Math.min(val, valueMax)
												if (inputMin != null) val = Math.max(val, inputMin)
												setValue(val)
												sendUpsert(min, val)
											}
										}}
										step={inputStep}
										min={inputMin ?? 1}
										max={valueMax ?? 99}
										className="w-16 h-8 text-center px-1"
									/>
									{units && (
										<Select
											value={String(unitIndex)}
											onValueChange={(index) => {
												const next = units[Number(index)]
												const nextInput = getInputRange(next)
												// convert the current threshold to the new unit, clamped to its range
												const val = roundToStep((value * factor) / next.factor, nextInput.step)
												const clamped = Math.min(Math.max(val, nextInput.min), next.max)
												setUnitIndex(Number(index))
												setValue(clamped)
												sendUpsert(min, clamped, undefined, next.factor)
											}}
										>
											<SelectTrigger className="w-auto shrink-0 h-8 gap-1.5 px-2.5">
												<SelectValue />
											</SelectTrigger>
											<SelectContent>
												{units.map(({ unit }, i) => (
													<SelectItem key={unit} value={String(i)}>
														{unit.trim()}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
									)}
								</div>
							</div>
						)}
						{canConfigureTemperatureSensors && temperatureMode === "sensors" && (
							<div className="col-span-full grid gap-3">
								{loadingTemperatureSensors && !temperatureSensors.length ? (
									<p className="text-sm">
										<Trans>Loading temperature sensors...</Trans>
									</p>
								) : (
									temperatureSensors.map((sensor, index) => {
										const enabled = Object.hasOwn(sensorThresholds, sensor)
										const sensorValue = sensorThresholds[sensor] ?? value
										const sensorId = `temperature-sensor-${index}`
										const updateSensor = (nextValue: number) => {
											const nextThresholds = updateSensorThresholds((currentThresholds) => ({
												...currentThresholds,
												[sensor]: nextValue,
											}))
											sendUpsert(min, value, nextThresholds)
										}
										return (
											<div key={sensor} className="grid gap-2 rounded-md border border-muted-foreground/15 p-3">
												<div className="flex items-center justify-between gap-3">
													<label htmlFor={sensorId} className="min-w-0 truncate text-sm font-medium text-foreground">
														{sensor}
													</label>
													<Checkbox
														id={sensorId}
														checked={enabled}
														onCheckedChange={(nextChecked) => {
															if (nextChecked) {
																updateSensor(sensorValue)
																return
															}
															const nextThresholds = updateSensorThresholds((currentThresholds) => {
																const nextThresholds = { ...currentThresholds }
																delete nextThresholds[sensor]
																return nextThresholds
															})
															if (!Object.keys(nextThresholds).length) setTemperatureMode("any")
															sendUpsert(min, value, nextThresholds)
														}}
													/>
												</div>
												<div className="flex gap-3 items-center">
													<Slider
														aria-label={sensor}
														disabled={!enabled}
														value={[sensorValue]}
														onValueChange={(val) =>
															updateSensorThresholds((currentThresholds) => ({
																...currentThresholds,
																[sensor]: val[0],
															}))
														}
														onValueCommit={(val) => updateSensor(val[0])}
														step={alertData.step ?? 1}
														min={alertData.min ?? 1}
														max={alertData.max ?? 99}
													/>
													<Input
														type="number"
														disabled={!enabled}
														aria-label={sensor}
														min={alertData.min ?? 1}
														max={alertData.max ?? 99}
														step={alertData.step ?? 1}
														value={sensorValue}
														onChange={(e) => {
															const nextValue = temperatureThresholdInput(e.target.value, alertData.min, alertData.max)
															if (nextValue !== undefined) {
																updateSensor(nextValue)
															}
														}}
														className="w-16 h-8 text-center px-1"
													/>
													<span className="sr-only">°C</span>
												</div>
											</div>
										)
									})
								)}
							</div>
						)}
						{!noDuration && (
							<div
								className={cn(
									(singleDescription || (canConfigureTemperatureSensors && temperatureMode === "sensors")) &&
										"col-span-full",
									singleDescription && "lowercase"
								)}
							>
								<p id={`t${name}`} className="text-sm block h-6 first-letter:uppercase">
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
								<div className="flex gap-3 items-center">
									<Slider
										aria-labelledby={`t${name}`}
										value={[min]}
										onValueCommit={(val) => sendUpsert(val[0], value)}
										onValueChange={(val) => setMin(val[0])}
										min={1}
										max={60}
									/>
									<Input
										type="number"
										value={min}
										onChange={(e) => {
											let val = parseInt(e.target.value, 10)
											if (!Number.isNaN(val)) {
												val = Math.max(1, Math.min(val, 60))
												setMin(val)
												sendUpsert(val, value)
											}
										}}
										min={1}
										max={60}
										className="w-16 h-8 text-center px-1"
									/>
								</div>
							</div>
						)}
					</Suspense>
					{checked && alertData.note && (
						<span className="block col-span-full text-sm text-muted-foreground -mt-3">{alertData.note()}</span>
					)}
				</div>
			)}
		</div>
	)
}
