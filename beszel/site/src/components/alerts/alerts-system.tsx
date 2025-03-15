import { t } from "@lingui/core/macro"
import { Trans, Plural } from "@lingui/react/macro"
import { $alerts, $systems, pb } from "@/lib/stores"
import { alertInfo, cn } from "@/lib/utils"
import { Switch } from "@/components/ui/switch"
import { AlertInfo, AlertRecord, SystemRecord } from "@/types"
import { lazy, Suspense, useMemo, useState } from "react"
import { toast } from "../ui/use-toast"
import { BatchService } from "pocketbase"
import { getSemaphore } from "@henrygd/semaphore"

interface AlertData {
	checked?: boolean
	val?: number
	min?: number
	updateAlert?: (checked: boolean, value: number, min: number) => void
	name: keyof typeof alertInfo
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
	const alert = systemAlerts.find((alert) => alert.name === data.name)

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
					name: data.name,
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

export const SystemAlertGlobal = ({ data, overwrite }: { data: AlertData; overwrite: boolean | "indeterminate" }) => {
	data.checked = false
	data.val = data.min = 0

	// set of system ids that have an alert for this name when the component is mounted
	const existingAlertsSystems = useMemo(() => {
		const map = new Set<string>()
		const alerts = $alerts.get()
		for (const alert of alerts) {
			if (alert.name === data.name) {
				map.add(alert.system)
			}
		}
		return map
	}, [])

	data.updateAlert = async (checked: boolean, value: number, min: number) => {
		const sem = getSemaphore("alerts")
		await sem.acquire()
		try {
			// if another update is waiting behind, don't start this one
			if (sem.size() > 1) {
				return
			}

			const recordData: Partial<AlertRecord> = {
				value,
				min,
				triggered: false,
			}

			const batch = batchWrapper("alerts", 25)
			const systems = $systems.get()
			const currentAlerts = $alerts.get()

			// map of current alerts with this name right now by system id
			const currentAlertsSystems = new Map<string, AlertRecord>()
			for (const alert of currentAlerts) {
				if (alert.name === data.name) {
					currentAlertsSystems.set(alert.system, alert)
				}
			}

			if (overwrite) {
				existingAlertsSystems.clear()
			}

			const processSystem = async (system: SystemRecord): Promise<void> => {
				const existingAlert = existingAlertsSystems.has(system.id)

				if (!overwrite && existingAlert) {
					return
				}

				const currentAlert = currentAlertsSystems.get(system.id)

				// delete existing alert if unchecked
				if (!checked && currentAlert) {
					return batch.remove(currentAlert.id)
				}
				if (checked && currentAlert) {
					// update existing alert if checked
					return batch.update(currentAlert.id, recordData)
				}
				if (checked) {
					// create new alert if checked and not existing
					return batch.create({
						system: system.id,
						user: pb.authStore.record!.id,
						name: data.name,
						...recordData,
					})
				}
			}

			// make sure current system is updated in the first batch
			await processSystem(data.system)
			for (const system of systems) {
				if (system.id === data.system.id) {
					continue
				}
				if (sem.size() > 1) {
					return
				}
				await processSystem(system)
			}
			await batch.send()
		} finally {
			sem.release()
		}
	}

	return <AlertContent data={data} />
}

/**
 * Creates a wrapper for performing batch operations on a specified collection.
 */
function batchWrapper(collection: string, batchSize: number) {
	let batch: BatchService | undefined
	let count = 0

	const create = async <T extends Record<string, any>>(options: T) => {
		batch ||= pb.createBatch()
		batch.collection(collection).create(options)
		if (++count >= batchSize) {
			await send()
		}
	}

	const update = async <T extends Record<string, any>>(id: string, data: T) => {
		batch ||= pb.createBatch()
		batch.collection(collection).update(id, data)
		if (++count >= batchSize) {
			await send()
		}
	}

	const remove = async (id: string) => {
		batch ||= pb.createBatch()
		batch.collection(collection).delete(id)
		if (++count >= batchSize) {
			await send()
		}
	}

	const send = async () => {
		if (count) {
			await batch?.send({ requestKey: null })
			batch = undefined
			count = 0
		}
	}

	return {
		update,
		remove,
		send,
		create,
	}
}

function AlertContent({ data }: { data: AlertData }) {
	const { name } = data

	const singleDescription = data.alert.singleDesc?.()

	const [checked, setChecked] = useState(data.checked || false)
	const [min, setMin] = useState(data.min || 10)
	const [value, setValue] = useState(data.val || (singleDescription ? 0 : 80))

	const Icon = alertInfo[name].icon

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
						<Icon className="h-4 w-4 opacity-85" /> {data.alert.name()}
					</p>
					{!checked && <span className="block text-sm text-muted-foreground">{data.alert.desc()}</span>}
				</div>
				<Switch
					id={`s${name}`}
					checked={checked}
					onCheckedChange={(newChecked) => {
						setChecked(newChecked)
						data.updateAlert?.(newChecked, value, min)
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
											{data.alert.unit}
										</strong>
									</Trans>
								</p>
								<div className="flex gap-3">
									<Slider
										aria-labelledby={`v${name}`}
										defaultValue={[value]}
										onValueCommit={(val) => {
											data.updateAlert?.(true, val[0], min)
										}}
										onValueChange={(val) => {
											setValue(val[0])
										}}
										min={1}
										max={alertInfo[name].max ?? 99}
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
									onValueCommit={(min) => {
										data.updateAlert?.(true, value, min[0])
									}}
									onValueChange={(val) => {
										setMin(val[0])
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
