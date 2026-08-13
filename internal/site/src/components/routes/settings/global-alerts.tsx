import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ChevronDownIcon } from "lucide-react"
import { lazy, memo, Suspense, useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { toast } from "@/components/ui/use-toast"
import { alertInfo } from "@/lib/alerts"
import { pb } from "@/lib/api"
import { $globalAlerts, $systems } from "@/lib/stores"
import { cn, debounce } from "@/lib/utils"
import type { AlertInfo, GlobalAlertRecord } from "@/types"

const Slider = lazy(() => import("@/components/ui/slider"))

const alertKeys = Object.keys(alertInfo) as (keyof typeof alertInfo)[]

const alertDebounce = 400

const failedUpdateToast = (error: unknown) => {
	console.error(error)
	toast({
		title: t`Failed to update alert`,
		description: t`Please check logs for more details.`,
		variant: "destructive",
	})
}

export default function GlobalAlertsSettings() {
	const globalAlerts = useStore($globalAlerts)

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>Global Alerts</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						Alert defaults applied to all systems automatically. Changes sync immediately to all non-excluded
						systems. New systems added later will also receive these alerts.
					</Trans>
				</p>
			</div>
			<Separator className="my-4" />
			<div className="grid gap-3">
				{alertKeys.map((name) => (
					<GlobalAlertRow key={name} alertKey={name} data={alertInfo[name]} alert={globalAlerts.get(name)} />
				))}
			</div>
		</div>
	)
}

const GlobalAlertRow = memo(function GlobalAlertRow({
	alertKey,
	data: alertData,
	alert,
}: {
	alertKey: string
	data: AlertInfo
	alert?: GlobalAlertRecord
}) {
	const systems = useStore($systems)
	const singleDescription = alertData.singleDesc?.()
	const { name } = alertData

	const [checked, setChecked] = useState(!!alert)
	const [min, setMin] = useState(alert?.min || 10)
	const [value, setValue] = useState(alert?.value || (singleDescription ? 0 : (alertData.start ?? 80)))
	const [excludedSystems, setExcludedSystems] = useState<string[]>(alert?.excluded_systems ?? [])

	// Sync local state when the server record changes (realtime updates from another session)
	useEffect(() => {
		setChecked(!!alert)
		if (alert) {
			setMin(alert.min)
			setValue(alert.value)
			setExcludedSystems(alert.excluded_systems)
		}
	}, [alert])

	const Icon = alertData.icon

	// Per-row debounce so adjusting one alert doesn't cancel another alert's pending save
	const debouncedUpsert = useMemo(
		() =>
			debounce(
				async ({ name, value, min, excluded_systems }: { name: string; value: number; min: number; excluded_systems: string[] }) => {
					try {
						const existing = $globalAlerts.get().get(name)
						if (existing) {
							await pb.collection("global_alerts").update(existing.id, { value, min, excluded_systems })
						} else {
							await pb.collection("global_alerts").create({ name, value, min, excluded_systems })
						}
					} catch (error) {
						failedUpdateToast(error)
					}
				},
				alertDebounce
			),
		// eslint-disable-next-line react-hooks/exhaustive-deps
		[] // intentionally empty — one debounce instance per row mount
	)

	function sendUpsert(newMin: number, newValue: number, newExcluded: string[] = excludedSystems) {
		debouncedUpsert({ name: alertKey, value: newValue, min: newMin, excluded_systems: newExcluded })
	}

	async function handleToggle(newChecked: boolean) {
		setChecked(newChecked)
		if (newChecked) {
			sendUpsert(min, value, excludedSystems)
		} else {
			try {
				const existing = $globalAlerts.get().get(alertKey)
				if (existing) {
					await pb.collection("global_alerts").delete(existing.id)
				}
			} catch (error) {
				failedUpdateToast(error)
				setChecked(true)
			}
		}
	}

	function toggleExclusion(systemId: string) {
		const newExcluded = excludedSystems.includes(systemId)
			? excludedSystems.filter((id) => id !== systemId)
			: [...excludedSystems, systemId]
		setExcludedSystems(newExcluded)
		sendUpsert(min, value, newExcluded)
	}

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100">
			<label
				htmlFor={`gs${name}`}
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
				<Switch id={`gs${name}`} checked={checked} onCheckedChange={handleToggle} />
			</label>
			{checked && (
				<div className="grid sm:grid-cols-2 mt-1.5 gap-5 px-4 pb-5 tabular-nums text-muted-foreground">
					<Suspense fallback={<div className="h-10" />}>
						{!singleDescription && (
							<div>
								<p id={`gv${name}`} className="text-sm block h-6">
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
								<div className="flex gap-3 items-center">
									<Slider
										aria-labelledby={`gv${name}`}
										value={[value]}
										onValueCommit={(val) => sendUpsert(min, val[0])}
										onValueChange={(val) => setValue(val[0])}
										step={alertData.step ?? 1}
										min={alertData.min ?? 1}
										max={alertData.max ?? 99}
									/>
									<Input
										type="number"
										value={value}
										onChange={(e) => {
											let val = parseFloat(e.target.value)
											if (!Number.isNaN(val)) {
												if (alertData.max != null) val = Math.min(val, alertData.max)
												if (alertData.min != null) val = Math.max(val, alertData.min)
												setValue(val)
												sendUpsert(min, val)
											}
										}}
										step={alertData.step ?? 1}
										min={alertData.min ?? 1}
										max={alertData.max ?? 99}
										className="w-16 h-8 text-center px-1"
									/>
								</div>
							</div>
						)}
						<div className={cn(singleDescription && "col-span-full lowercase")}>
							<p id={`gt${name}`} className="text-sm block h-6 first-letter:uppercase">
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
									aria-labelledby={`gt${name}`}
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
					</Suspense>
					{systems.length > 0 && (
						<div className="sm:col-span-2">
							<p className="text-sm mb-2">
								<Trans>
									Excluded systems{" "}
									{excludedSystems.length > 0 && (
										<strong className="text-foreground">({excludedSystems.length})</strong>
									)}
								</Trans>
							</p>
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" size="sm" className="text-xs gap-1.5 h-8">
										{excludedSystems.length === 0 ? (
											<Trans>Exclude systems</Trans>
										) : (
											<Trans>
												{excludedSystems.length} excluded
											</Trans>
										)}
										<ChevronDownIcon className="h-3.5 w-3.5" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="start" className="max-h-64 overflow-auto min-w-44">
									{systems.map((system) => (
										<DropdownMenuCheckboxItem
											key={system.id}
											checked={excludedSystems.includes(system.id)}
											onCheckedChange={() => toggleExclusion(system.id)}
											onSelect={(e) => e.preventDefault()}
										>
											{system.name}
										</DropdownMenuCheckboxItem>
									))}
								</DropdownMenuContent>
							</DropdownMenu>
						</div>
					)}
				</div>
			)}
		</div>
	)
})
