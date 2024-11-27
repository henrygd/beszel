import { pb } from "@/lib/stores"
import { alertInfo, cn } from "@/lib/utils"
import { Switch } from "@/components/ui/switch"
import { AlertInfo, AlertRecord, SystemRecord } from "@/types"
import { lazy, Suspense, useRef, useState } from "react"
import { toast } from "../ui/use-toast"
import { RecordOptions } from "pocketbase"
import { Trans, t, Plural } from "@lingui/macro"

interface AlertData {
	checked?: boolean
	val?: number
	min?: number
	updateAlert?: (checked: boolean, value: number, min: number) => void
	key: keyof typeof alertInfo
	alert: AlertInfo
	system: SystemRecord
}

const Slider = lazy(() => import("@/components/ui/slider"))

const failedUpdateToast = () =>
	toast({
		title: t`Failed to update alert`,
		description: t`Please check logs for more details.`,
		variant: "destructive",
	})

export function SystemAlert({
	system,
	systemAlerts,
	data,
}: {
	system: SystemRecord
	systemAlerts: AlertRecord[]
	data: AlertData
}) {
	const alert = systemAlerts.find((alert) => alert.name === data.key)

	data.updateAlert = async (checked: boolean, value: number, min: number) => {
		try {
			if (alert && !checked) {
				await pb.collection("alerts").delete(alert.id)
			} else if (alert && checked) {
				await pb.collection("alerts").update(alert.id, { value, min, triggered: false })
			} else if (checked) {
				pb.collection("alerts").create({
					system: system.id,
					user: pb.authStore.record!.id,
					name: data.key,
					value: value,
					min: min,
				})
			}
		} catch (e) {
			failedUpdateToast()
		}
	}

	if (alert) {
		data.checked = true
		data.val = alert.value
		data.min = alert.min || 1
	}

	return <AlertContent data={data} />
}

export function SystemAlertGlobal({
	data,
	overwrite,
	alerts,
	systems,
}: {
	data: AlertData
	overwrite: boolean | "indeterminate"
	alerts: AlertRecord[]
	systems: SystemRecord[]
}) {
	const systemsWithExistingAlerts = useRef<{ set: Set<string>; populatedSet: boolean }>({
		set: new Set(),
		populatedSet: false,
	})

	data.checked = false
	data.val = data.min = 0

	data.updateAlert = async (checked: boolean, value: number, min: number) => {
		const { set, populatedSet } = systemsWithExistingAlerts.current

		// if overwrite checked, make sure all alerts will be overwritten
		if (overwrite) {
			set.clear()
		}

		const recordData: Partial<AlertRecord> = {
			value,
			min,
			triggered: false,
		}

		// we can only send 50 in one batch
		let done = 0

		while (done < systems.length) {
			const batch = pb.createBatch()
			let batchSize = 0

			for (let i = done; i < Math.min(done + 50, systems.length); i++) {
				const system = systems[i]
				// if overwrite is false and system is in set (alert existed), skip
				if (!overwrite && set.has(system.id)) {
					continue
				}
				// find matching existing alert
				const existingAlert = alerts.find((alert) => alert.system === system.id && data.key === alert.name)
				// if first run, add system to set (alert already existed when global panel was opened)
				if (existingAlert && !populatedSet && !overwrite) {
					set.add(system.id)
					continue
				}
				batchSize++
				const requestOptions: RecordOptions = {
					requestKey: system.id,
				}

				// checked - make sure alert is created or updated
				if (checked) {
					if (existingAlert) {
						batch.collection("alerts").update(existingAlert.id, recordData, requestOptions)
					} else {
						batch.collection("alerts").create(
							{
								system: system.id,
								user: pb.authStore.record!.id,
								name: data.key,
								...recordData,
							},
							requestOptions
						)
					}
				} else if (existingAlert) {
					batch.collection("alerts").delete(existingAlert.id)
				}
			}
			try {
				batchSize && batch.send()
			} catch (e) {
				failedUpdateToast()
			} finally {
				done += 50
			}
		}
		systemsWithExistingAlerts.current.populatedSet = true
	}

	return <AlertContent data={data} />
}

function AlertContent({ data }: { data: AlertData }) {
	const { key } = data

	const hasSliders = !("single" in data.alert)

	const [checked, setChecked] = useState(data.checked || false)
	const [min, setMin] = useState(data.min || (hasSliders ? 10 : 0))
	const [value, setValue] = useState(data.val || (hasSliders ? 80 : 0))

	const showSliders = checked && hasSliders

	const newMin = useRef(min)
	const newValue = useRef(value)

	const Icon = alertInfo[key].icon

	const updateAlert = (c?: boolean) => data.updateAlert?.(c ?? checked, newValue.current, newMin.current)

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
			<label
				htmlFor={`s${key}`}
				className={cn("flex flex-row items-center justify-between gap-4 cursor-pointer p-4", {
					"pb-0": showSliders,
				})}
			>
				<div className="grid gap-1 select-none">
					<p className="font-semibold flex gap-3 items-center">
						<Icon className="h-4 w-4 opacity-85" /> {data.alert.name()}
					</p>
					{!showSliders && <span className="block text-sm text-muted-foreground">{data.alert.desc()}</span>}
				</div>
				<Switch
					id={`s${key}`}
					checked={checked}
					onCheckedChange={(checked) => {
						setChecked(checked)
						updateAlert(checked)
					}}
				/>
			</label>
			{showSliders && (
				<div className="grid sm:grid-cols-2 mt-1.5 gap-5 px-4 pb-5 tabular-nums text-muted-foreground">
					<Suspense fallback={<div className="h-10" />}>
						<div>
							<p id={`v${key}`} className="text-sm block h-8">
								<Trans>
									Average exceeds{" "}
									<strong className="text-foreground">
										{value}
										{data.alert.unit}
									</strong>
								</Trans>
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`v${key}`}
									defaultValue={[value]}
									onValueCommit={(val) => (newValue.current = val[0]) && updateAlert()}
									onValueChange={(val) => setValue(val[0])}
									min={1}
									max={alertInfo[key].max ?? 99}
								/>
							</div>
						</div>
						<div>
							<p id={`t${key}`} className="text-sm block h-8">
								<Trans>
									For <strong className="text-foreground">{min}</strong>{" "}
									<Plural value={min} one=" minute" other=" minutes" />
								</Trans>
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`v${key}`}
									defaultValue={[min]}
									onValueCommit={(val) => (newMin.current = val[0]) && updateAlert()}
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
