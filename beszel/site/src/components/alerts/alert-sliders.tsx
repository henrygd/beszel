import { $systems, pb } from '@/lib/stores'
import { useStore } from '@nanostores/react'
import { alertInfo, alertQueue, cn, failedUpdateToast } from '@/lib/utils'
import { Switch } from '@/components/ui/switch'
import { AlertRecord, SystemRecord } from '@/types'
import { lazy, Suspense, useEffect, useRef, useState } from 'react'

interface AlertData {
	checked?: boolean
	val?: number
	min?: number
	updateAlert: (checked: boolean, value: number, min: number) => void
	key: keyof typeof alertInfo
	alert: (typeof alertInfo)[keyof typeof alertInfo]
	system: SystemRecord
	alerts: AlertRecord[]
}

const Slider = lazy(() => import('@/components/ui/slider'))

export function AlertWithSliders({
	system,
	systemAlerts: alerts,
	data,
}: {
	system: SystemRecord
	systemAlerts: AlertRecord[]
	data: AlertData
}) {
	const alert = alerts.find((alert) => alert.name === data.key)

	data.updateAlert = async (checked: boolean, value: number, min: number) => {
		try {
			if (alert && !checked) {
				await pb.collection('alerts').delete(alert.id)
			} else if (alert && checked) {
				await pb.collection('alerts').update(alert.id, { value, min, triggered: false })
			} else if (checked) {
				pb.collection('alerts').create({
					system: system.id,
					user: pb.authStore.model!.id,
					name: data.key,
					value: value,
					min: min,
				})
			}
		} catch (e) {
			failedUpdateToast()
		}
	}

	// const data: AlertData = {
	// 	title,
	// 	description,
	// 	unit,
	// 	key,
	// 	updateAlert,
	// }

	if (alert) {
		data.checked = true
		data.val = alert.value
		data.min = alert.min || 1
	}

	return <SliderStuff data={data} />
}

export function AlertWithSlidersGlobal({
	data,
	overwrite,
	alerts,
}: {
	data: AlertData
	overwrite: boolean | 'indeterminate'
	alerts: AlertRecord[]
}) {
	const systems = useStore($systems)

	data.checked = false
	data.val = 0
	data.min = 0

	data.updateAlert = (checked: boolean, value: number, min: number) => {
		const queue = alertQueue()

		const recordData: Partial<AlertRecord> = {
			value,
			min,
			triggered: false,
		}
		for (let system of systems) {
			const matchingAlert = alerts.find(
				(alert) => alert.system === system.id && data.key === alert.name
			)
			if (matchingAlert && !overwrite) {
				continue
			}
			// checked - make sure alert is created or updated
			if (checked) {
				if (matchingAlert) {
					console.log('updating', matchingAlert.id, recordData)
					queue.add(() => pb.collection('alerts').update(matchingAlert.id, recordData))
				} else {
					console.log('creating', recordData)
					queue.add(() =>
						pb.collection('alerts').create({
							system: system.id,
							user: pb.authStore.model!.id,
							name: data.key,
							...recordData,
						})
					)
				}
			} else if (matchingAlert) {
				console.log('deleting', matchingAlert.id)
				queue.add(() => pb.collection('alerts').delete(matchingAlert.id))
			}
		}
	}

	// const data = {
	// 	title,
	// 	description,
	// 	key,
	// 	unit,
	// 	updateAlert,
	// }

	return <SliderStuff data={data} />
}

function SliderStuff({ data }: { data: AlertData }) {
	const { key } = data

	const [checked, setChecked] = useState(data.checked || false)
	const [min, setMin] = useState(data.min || 10)
	const [value, setValue] = useState(data.val || 80)
	const [newValue, setNewValue] = useState(value)
	const [newMin, setNewMin] = useState(min)
	const mounted = useRef(false)

	const Icon = alertInfo[key].icon

	useEffect(() => {
		if (!mounted.current) {
			mounted.current = true
			return
		}
		console.log({ checked, newValue, newMin })
		data.updateAlert(checked, newValue, newMin)
	}, [checked, newValue, newMin])

	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
			<label
				htmlFor={`s${key}`}
				className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4', {
					'pb-0': checked,
				})}
			>
				<div className="grid gap-1 select-none">
					<p className="font-semibold flex gap-3 items-center capitalize">
						<Icon className="h-4 w-4 opacity-85" /> {data.alert.name}
					</p>
					{!checked && (
						<span className="block text-sm text-muted-foreground">{data.alert.desc}</span>
					)}
				</div>
				<Switch id={`s${key}`} checked={checked} onCheckedChange={setChecked} />
			</label>
			{checked && (
				<div className="grid sm:grid-cols-2 mt-1.5 gap-5 px-4 pb-5 tabular-nums text-muted-foreground">
					<Suspense fallback={<div className="h-10" />}>
						<div>
							<p id={`v${key}`} className="text-sm block h-8">
								Average exceeds{' '}
								<strong className="text-foreground">
									{value}
									{data.alert.unit}
								</strong>
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`v${key}`}
									defaultValue={[value]}
									onValueCommit={(val) => setNewValue(val[0])}
									onValueChange={(val) => setValue(val[0])}
									min={1}
									max={99}
								/>
							</div>
						</div>
						<div>
							<p id={`t${key}`} className="text-sm block h-8">
								For <strong className="text-foreground">{min}</strong> minute
								{min > 1 && 's'}
							</p>
							<div className="flex gap-3">
								<Slider
									aria-labelledby={`v${key}`}
									defaultValue={[min]}
									onValueCommit={(val) => setNewMin(val[0])}
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
