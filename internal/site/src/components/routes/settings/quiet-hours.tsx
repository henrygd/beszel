import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import {
	MoreHorizontalIcon,
	PlusIcon,
	Trash2Icon,
	ServerIcon,
	ClockIcon,
	CalendarIcon,
	ActivityIcon,
	PenSquareIcon,
} from "lucide-react"
import { useEffect, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useToast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import { $systems } from "@/lib/stores"
import { formatShortDate } from "@/lib/utils"
import type { QuietHoursRecord, SystemRecord } from "@/types"

export function QuietHours() {
	const [data, setData] = useState<QuietHoursRecord[]>([])
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editingRecord, setEditingRecord] = useState<QuietHoursRecord | null>(null)
	const { toast } = useToast()
	const systems = useStore($systems)

	useEffect(() => {
		let unsubscribe: (() => void) | undefined
		const pbOptions = {
			expand: "system",
			fields: "id,user,system,type,start,end,expand.system.name",
		}
		// Initial load
		pb.collection<QuietHoursRecord>("quiet_hours")
			.getList(0, 200, {
				...pbOptions,
				sort: "system",
			})
			.then(({ items }) => setData(items))

		// Subscribe to changes
		;(async () => {
			unsubscribe = await pb.collection("quiet_hours").subscribe(
				"*",
				(e) => {
					if (e.action === "create") {
						setData((current) => [e.record as QuietHoursRecord, ...current])
					}
					if (e.action === "update") {
						setData((current) => current.map((r) => (r.id === e.record.id ? (e.record as QuietHoursRecord) : r)))
					}
					if (e.action === "delete") {
						setData((current) => current.filter((r) => r.id !== e.record.id))
					}
				},
				pbOptions
			)
		})()
		// Unsubscribe on unmount
		return () => unsubscribe?.()
	}, [])

	const handleDelete = async (id: string) => {
		try {
			await pb.collection("quiet_hours").delete(id)
		} catch (e: unknown) {
			toast({
				variant: "destructive",
				title: t`Error`,
				description: (e as Error).message || "Failed to delete quiet hours.",
			})
		}
	}

	const openEditDialog = (record: QuietHoursRecord) => {
		setEditingRecord(record)
		setDialogOpen(true)
	}

	const closeDialog = () => {
		setDialogOpen(false)
		setEditingRecord(null)
	}

	const formatDateTime = (record: QuietHoursRecord) => {
		if (record.type === "daily") {
			// For daily windows, show only time
			const startTime = new Date(record.start).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })
			const endTime = new Date(record.end).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })
			return `${startTime} - ${endTime}`
		}
		// For one-time windows, show full date and time
		const start = formatShortDate(record.start)
		const end = formatShortDate(record.end)
		return `${start} - ${end}`
	}

	const getWindowState = (record: QuietHoursRecord): "active" | "past" | "future" => {
		const now = new Date()

		if (record.type === "daily") {
			// For daily windows, check if current time is within the window
			const startDate = new Date(record.start)
			const endDate = new Date(record.end)

			// Get current time in local timezone
			const currentMinutes = now.getHours() * 60 + now.getMinutes()
			const startMinutes = startDate.getUTCHours() * 60 + startDate.getUTCMinutes()
			const endMinutes = endDate.getUTCHours() * 60 + endDate.getUTCMinutes()

			// Convert UTC to local time offset
			const offset = now.getTimezoneOffset()
			const localStartMinutes = (startMinutes - offset + 1440) % 1440
			const localEndMinutes = (endMinutes - offset + 1440) % 1440

			// Handle cases where window spans midnight
			if (localStartMinutes <= localEndMinutes) {
				return currentMinutes >= localStartMinutes && currentMinutes < localEndMinutes ? "active" : "future"
			} else {
				return currentMinutes >= localStartMinutes || currentMinutes < localEndMinutes ? "active" : "future"
			}
		} else {
			// For one-time windows
			const startDate = new Date(record.start)
			const endDate = new Date(record.end)

			if (now >= startDate && now < endDate) {
				return "active"
			} else if (now >= endDate) {
				return "past"
			} else {
				return "future"
			}
		}
	}

	return (
		<>
			<div className="grid grid-cols-1 sm:flex items-center justify-between gap-4 mb-3">
				<div>
					<h3 className="mb-1 text-lg font-medium">
						<Trans>Quiet hours</Trans>
					</h3>
					<p className="text-sm text-muted-foreground leading-relaxed">
						<Trans>
							Schedule quiet hours where notifications will not be sent, such as during maintenance periods.
						</Trans>
					</p>
				</div>
				<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
					<DialogTrigger asChild>
						<Button variant="outline" className="h-10 shrink-0" onClick={() => setEditingRecord(null)}>
							<PlusIcon className="size-4" />
							<span className="ms-1">
								<Trans>Add Quiet Hours</Trans>
							</span>
						</Button>
					</DialogTrigger>
					<QuietHoursDialog editingRecord={editingRecord} systems={systems} onClose={closeDialog} toast={toast} />
				</Dialog>
			</div>
			{data.length > 0 && (
				<div className="rounded-md border overflow-x-auto whitespace-nowrap">
					<Table>
						<TableHeader>
							<TableRow className="border-border/50">
								<TableHead className="px-4">
									<span className="flex items-center gap-2">
										<ServerIcon className="size-4" />
										<Trans>System</Trans>
									</span>
								</TableHead>
								<TableHead className="px-4">
									<span className="flex items-center gap-2">
										<ClockIcon className="size-4" />
										<Trans>Type</Trans>
									</span>
								</TableHead>
								<TableHead className="px-4">
									<span className="flex items-center gap-2">
										<ActivityIcon className="size-4" />
										<Trans>State</Trans>
									</span>
								</TableHead>
								<TableHead className="px-4">
									<span className="flex items-center gap-2">
										<CalendarIcon className="size-4" />
										<Trans>Schedule</Trans>
									</span>
								</TableHead>
								<TableHead className="px-4 text-right sr-only">
									<Trans>Actions</Trans>
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{data.map((record) => (
								<TableRow key={record.id}>
									<TableCell className="px-4 py-3">
										{record.system ? record.expand?.system?.name || record.system : <Trans>All Systems</Trans>}
									</TableCell>
									<TableCell className="px-4 py-3">
										{record.type === "daily" ? <Trans>Daily</Trans> : <Trans>One-time</Trans>}
									</TableCell>
									<TableCell className="px-4 py-3">
										{(() => {
											const state = getWindowState(record)
											const stateConfig = {
												active: { label: <Trans>Active</Trans>, variant: "success" as const },
												past: { label: <Trans>Past</Trans>, variant: "danger" as const },
												future: { label: <Trans>Future</Trans>, variant: "default" as const },
											}
											const config = stateConfig[state]
											return <Badge variant={config.variant}>{config.label}</Badge>
										})()}
									</TableCell>
									<TableCell className="px-4 py-3">{formatDateTime(record)}</TableCell>
									<TableCell className="px-4 py-3 text-right">
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button variant="ghost" size="icon" className="size-8">
													<span className="sr-only">
														<Trans>Open menu</Trans>
													</span>
													<MoreHorizontalIcon className="size-4" />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end">
												<DropdownMenuItem onClick={() => openEditDialog(record)}>
													<PenSquareIcon className="me-2.5 size-4" />
													<Trans>Edit</Trans>
												</DropdownMenuItem>
												<DropdownMenuItem onClick={() => handleDelete(record.id)}>
													<Trash2Icon className="me-2.5 size-4" />
													<Trans>Delete</Trans>
												</DropdownMenuItem>
											</DropdownMenuContent>
										</DropdownMenu>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				</div>
			)}
		</>
	)
}

// Helper function to format Date as datetime-local string (YYYY-MM-DDTHH:mm) in local time
function formatDateTimeLocal(date: Date): string {
	const year = date.getFullYear()
	const month = String(date.getMonth() + 1).padStart(2, "0")
	const day = String(date.getDate()).padStart(2, "0")
	const hours = String(date.getHours()).padStart(2, "0")
	const minutes = String(date.getMinutes()).padStart(2, "0")
	return `${year}-${month}-${day}T${hours}:${minutes}`
}

function QuietHoursDialog({
	editingRecord,
	systems,
	onClose,
	toast,
}: {
	editingRecord: QuietHoursRecord | null
	systems: SystemRecord[]
	onClose: () => void
	toast: any
}) {
	const [selectedSystem, setSelectedSystem] = useState(editingRecord?.system || "")
	const [isGlobal, setIsGlobal] = useState(!editingRecord?.system)
	const [windowType, setWindowType] = useState<"one-time" | "daily">(editingRecord?.type || "one-time")
	const [startDateTime, setStartDateTime] = useState("")
	const [endDateTime, setEndDateTime] = useState("")
	const [startTime, setStartTime] = useState("")
	const [endTime, setEndTime] = useState("")

	useEffect(() => {
		if (editingRecord) {
			setSelectedSystem(editingRecord.system || "")
			setIsGlobal(!editingRecord.system)
			setWindowType(editingRecord.type)
			if (editingRecord.type === "daily") {
				// Extract time from datetime
				const start = new Date(editingRecord.start)
				const end = editingRecord.end ? new Date(editingRecord.end) : null
				setStartTime(start.toTimeString().slice(0, 5))
				setEndTime(end ? end.toTimeString().slice(0, 5) : "")
			} else {
				// For one-time, format as datetime-local (local time, not UTC)
				const startDate = new Date(editingRecord.start)
				const endDate = editingRecord.end ? new Date(editingRecord.end) : null

				setStartDateTime(formatDateTimeLocal(startDate))
				setEndDateTime(endDate ? formatDateTimeLocal(endDate) : "")
			}
		} else {
			// Reset form with default dates: today at 12pm and 1pm
			const today = new Date()
			const noon = new Date(today)
			noon.setHours(12, 0, 0, 0)
			const onePm = new Date(today)
			onePm.setHours(13, 0, 0, 0)

			setSelectedSystem("")
			setIsGlobal(true)
			setWindowType("one-time")
			setStartDateTime(formatDateTimeLocal(noon))
			setEndDateTime(formatDateTimeLocal(onePm))
			setStartTime("12:00")
			setEndTime("13:00")
		}
	}, [editingRecord])

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault()

		try {
			let startValue: string
			let endValue: string | undefined

			if (windowType === "daily") {
				// For daily windows, convert local time to UTC
				// Create a date with the time in local timezone, then convert to UTC
				const startDate = new Date(`2000-01-01T${startTime}:00`)
				startValue = startDate.toISOString()

				if (endTime) {
					const endDate = new Date(`2000-01-01T${endTime}:00`)
					endValue = endDate.toISOString()
				}
			} else {
				// For one-time windows, use the datetime values
				startValue = new Date(startDateTime).toISOString()
				endValue = endDateTime ? new Date(endDateTime).toISOString() : undefined
			}

			const data = {
				user: pb.authStore.record?.id,
				system: isGlobal ? undefined : selectedSystem,
				type: windowType,
				start: startValue,
				end: endValue,
			}

			if (editingRecord) {
				await pb.collection("quiet_hours").update(editingRecord.id, data)
				toast({
					title: t`Updated`,
					description: t`Quiet hours have been updated.`,
				})
			} else {
				await pb.collection("quiet_hours").create(data)
				toast({
					title: t`Created`,
					description: t`Quiet hours have been created.`,
				})
			}

			onClose()
		} catch (e) {
			toast({
				variant: "destructive",
				title: t`Error`,
				description: t`Failed to save quiet hours.`,
			})
		}
	}

	return (
		<DialogContent>
			<DialogHeader>
				<DialogTitle>{editingRecord ? <Trans>Edit Quiet Hours</Trans> : <Trans>Add Quiet Hours</Trans>}</DialogTitle>
				<DialogDescription>
					<Trans>Configure quiet hours where notifications will not be sent.</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="space-y-4">
				<Tabs value={isGlobal ? "global" : "system"} onValueChange={(value) => setIsGlobal(value === "global")}>
					<TabsList className="grid w-full grid-cols-2">
						<TabsTrigger value="global">
							<Trans>All Systems</Trans>
						</TabsTrigger>
						<TabsTrigger value="system">
							<Trans>Specific System</Trans>
						</TabsTrigger>
					</TabsList>

					<TabsContent value="system" className="mt-4 space-y-4">
						<div className="grid gap-2">
							<Label htmlFor="system">
								<Trans>System</Trans>
							</Label>
							<Select value={selectedSystem} onValueChange={setSelectedSystem}>
								<SelectTrigger id="system">
									<SelectValue placeholder={t`Select a system`} />
								</SelectTrigger>
								<SelectContent>
									{systems.map((system) => (
										<SelectItem key={system.id} value={system.id}>
											{system.name}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
							{/* Hidden input for native form validation */}
							<input
								className="sr-only"
								type="text"
								tabIndex={-1}
								autoComplete="off"
								value={selectedSystem}
								onChange={() => {}}
								required={!isGlobal}
							/>
						</div>
					</TabsContent>
				</Tabs>

				<div className="grid gap-2">
					<Label htmlFor="type">
						<Trans>Type</Trans>
					</Label>
					<Select value={windowType} onValueChange={(value: "one-time" | "daily") => setWindowType(value)}>
						<SelectTrigger id="type">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="one-time">
								<Trans>One-time</Trans>
							</SelectItem>
							<SelectItem value="daily">
								<Trans>Daily</Trans>
							</SelectItem>
						</SelectContent>
					</Select>
				</div>

				{windowType === "one-time" ? (
					<>
						<div className="grid gap-2">
							<Label htmlFor="start-datetime">
								<Trans>Start Time</Trans>
							</Label>
							<Input
								id="start-datetime"
								type="datetime-local"
								value={startDateTime}
								onChange={(e) => setStartDateTime(e.target.value)}
								min={formatDateTimeLocal(new Date(new Date().setHours(0, 0, 0, 0)))}
								required
								className="tabular-nums tracking-tighter"
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="end-datetime">
								<Trans>End Time</Trans>
							</Label>
							<Input
								id="end-datetime"
								type="datetime-local"
								value={endDateTime}
								onChange={(e) => setEndDateTime(e.target.value)}
								min={startDateTime || formatDateTimeLocal(new Date())}
								required
								className="tabular-nums tracking-tighter"
							/>
						</div>
					</>
				) : (
					<div className="grid gap-2 grid-cols-2">
						<div>
							<Label htmlFor="start-time">
								<Trans>Start Time</Trans>
							</Label>
							<Input
								className="tabular-nums tracking-tighter"
								id="start-time"
								type="time"
								value={startTime}
								onChange={(e) => setStartTime(e.target.value)}
								required
							/>
						</div>
						<div>
							<Label htmlFor="end-time">
								<Trans>End Time</Trans>
							</Label>
							<Input
								className="tabular-nums tracking-tighter"
								id="end-time"
								type="time"
								value={endTime}
								onChange={(e) => setEndTime(e.target.value)}
								required
							/>
						</div>
					</div>
				)}

				<DialogFooter>
					<Button type="button" variant="outline" onClick={onClose}>
						<Trans>Cancel</Trans>
					</Button>
					<Button type="submit">{editingRecord ? <Trans>Update</Trans> : <Trans>Create</Trans>}</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
