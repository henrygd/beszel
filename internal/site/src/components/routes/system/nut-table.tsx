import { t } from "@lingui/core/macro"
import {
	type ColumnDef,
	type ColumnFiltersState,
	type Column,
	type Row,
	type SortingState,
	type Table as TableType,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table"
import { useVirtualizer, type VirtualItem } from "@tanstack/react-virtual"
import {
	Activity,
	Box,
	Battery,
	BatteryLow,
	Bolt,
	Clock,
	Plug,
	RefreshCwIcon,
	ServerIcon,
	Trash2Icon,
	XIcon,
	MoreHorizontalIcon,
	LoaderCircleIcon,
	CheckCircle2Icon,
	XCircleIcon,
	AlertTriangleIcon,
	ZapIcon,
} from "lucide-react"
import { Card, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { isReadOnlyUser, pb } from "@/lib/api"
import type { NutDeviceRecord, NutOutlet } from "@/types"
import { cn, toFixedFloat, hourWithSeconds, formatShortDate, secondsToString } from "@/lib/utils"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { $allSystemsById } from "@/lib/stores"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Separator } from "@/components/ui/separator"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { memo, useCallback, useMemo, useEffect, useRef, useState } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

const NUT_DEVICE_FIELDS =
	"id,system,name,model,manufacturer,serial,firmware,driver,device_type,state,health,battery_charge,battery_voltage,battery_runtime,input_voltage,output_voltage,input_nominal,load,output_current,output_power,updated"

function healthVariant(health: string): "success" | "warning" | "danger" | "outline" {
	switch (health) {
		case "ONLINE":
			return "success"
		case "ON_BATTERY":
			return "warning"
		case "LOW_BATTERY":
		case "OVERLOAD":
			return "danger"
		case "FAULT":
			return "danger"
		default:
			return "outline"
	}
}

function deviceTypeIcon(type: string) {
	switch (type) {
		case "ups":
			return Battery
		case "pdu":
			return Plug
		case "ats":
			return ZapIcon
		default:
			return Box
	}
}

export const createColumns = (
	longestName: string,
	longestModel: string,
	longestDevice: string
): ColumnDef<NutDeviceRecord>[] => [
	{
		id: "system",
		accessorFn: (record) => record.system,
		sortingFn: (a, b) => {
			const allSystems = $allSystemsById.get()
			const systemNameA = allSystems[a.original.system]?.name ?? ""
			const systemNameB = allSystems[b.original.system]?.name ?? ""
			return systemNameA.localeCompare(systemNameB)
		},
		header: ({ column }) => <HeaderButton column={column} name={t`System`} Icon={ServerIcon} />,
		cell: ({ getValue }) => {
			const allSystems = useStore($allSystemsById)
			return (
				<div className="ms-1.5 relative w-fit max-w-44">
					<span className="invisible block whitespace-nowrap" aria-hidden="true">
						{longestName}
					</span>
					<span className="absolute inset-0 truncate">{allSystems[getValue() as string]?.name ?? ""}</span>
				</div>
			)
		},
	},
	{
		accessorKey: "name",
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		header: ({ column }) => <HeaderButton column={column} name={t`Device`} Icon={Battery} />,
		cell: ({ getValue }) => (
			<div className="font-medium ms-1 relative w-fit max-w-44" title={getValue() as string}>
				<span className="invisible block whitespace-nowrap" aria-hidden="true">
					{longestDevice}
				</span>
				<span className="absolute inset-0 truncate">{getValue() as string}</span>
			</div>
		),
	},
	{
		accessorKey: "model",
		sortingFn: (a, b) => a.original.model.localeCompare(b.original.model),
		header: ({ column }) => (
			<HeaderButton column={column} name={t({ message: "Model", comment: "Device model" })} Icon={Box} />
		),
		cell: ({ getValue }) => (
			<div className="ms-1 relative w-fit max-w-44" title={getValue() as string}>
				<span className="invisible block whitespace-nowrap" aria-hidden="true">
					{longestModel}
				</span>
				<span className="absolute inset-0 truncate">{getValue() as string}</span>
			</div>
		),
	},
	{
		accessorKey: "device_type",
		sortingFn: (a, b) => a.original.device_type.localeCompare(b.original.device_type),
		header: ({ column }) => <HeaderButton column={column} name={t`Type`} Icon={Plug} />,
		cell: ({ getValue }) => {
			const type = getValue() as string
			const Icon = deviceTypeIcon(type)
			return (
				<Badge variant="outline" className="ms-1 uppercase gap-1">
					<Icon className="size-3" />
					{type}
				</Badge>
			)
		},
	},
	{
		accessorKey: "health",
		header: ({ column }) => <HeaderButton column={column} name={t`Status`} Icon={Activity} />,
		cell: ({ getValue }) => {
			const health = getValue() as string
			return (
				<Badge className="ms-1" variant={healthVariant(health)}>
					{health.replace("_", " ")}
				</Badge>
			)
		},
	},
	{
		accessorKey: "battery_charge",
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Battery`} Icon={BatteryLow} />,
		cell: ({ getValue }) => {
			const charge = getValue() as number | undefined
			if (charge == null || charge === 0) {
				return <div className="text-sm text-muted-foreground ms-1">N/A</div>
			}
			return (
				<div className="text-sm ms-1">
					<span className={cn(charge < 20 && "text-red-500", charge < 50 && "text-yellow-500")}>
						{toFixedFloat(charge, 0)}%
					</span>
				</div>
			)
		},
	},
	{
		accessorKey: "load",
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Load`} Icon={Bolt} />,
		cell: ({ getValue }) => {
			const load = getValue() as number | undefined
			if (load == null || load === 0) {
				return <div className="text-sm text-muted-foreground ms-1">N/A</div>
			}
			return <span className="ms-1">{toFixedFloat(load, 0)}%</span>
		},
	},
	{
		accessorKey: "output_power",
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Power`} Icon={ZapIcon} />,
		cell: ({ getValue }) => {
			const power = getValue() as number | undefined
			if (power == null || power === 0) {
				return <div className="text-sm text-muted-foreground ms-1">N/A</div>
			}
			return <span className="ms-1">{toFixedFloat(power, 0)} W</span>
		},
	},
	{
		id: "updated",
		invertSorting: true,
		accessorFn: (record) => record.updated,
		header: ({ column }) => <HeaderButton column={column} name={t`Updated`} Icon={Clock} />,
		cell: ({ getValue }) => {
			const timestamp = getValue() as string
			const formatter =
				new Date(timestamp).toDateString() === new Date().toDateString() ? hourWithSeconds : formatShortDate
			return <span className="ms-1 tabular-nums">{formatter(timestamp)}</span>
		},
	},
]

function HeaderButton({
	column,
	name,
	Icon,
}: {
	column: Column<NutDeviceRecord>
	name: string
	Icon: React.ElementType
}) {
	const isSorted = column.getIsSorted()
	return (
		<Button
			className={cn(
				"h-9 px-3 flex items-center gap-2 duration-50",
				isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90"
			)}
			variant="ghost"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{Icon && <Icon className="size-4" />}
			{name}
		</Button>
	)
}

export default function NutTable({ systemId }: { systemId?: string }) {
	const [sorting, setSorting] = useState<SortingState>([{ id: systemId ? "name" : "system", desc: false }])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [rowSelection, setRowSelection] = useState({})
	const [nutDevices, setNutDevices] = useState<NutDeviceRecord[] | undefined>(undefined)
	const [activeDeviceId, setActiveDeviceId] = useState<string | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const [rowActionState, setRowActionState] = useState<{ type: "refresh" | "delete"; id: string } | null>(null)
	const [globalFilter, setGlobalFilter] = useState("")
	const allSystems = useStore($allSystemsById)

	const { longestName, longestModel, longestDevice } = useMemo(() => {
		const result = { longestName: "", longestModel: "", longestDevice: "" }
		if (!nutDevices || Object.keys(allSystems).length === 0) {
			return result
		}
		const seenSystems = new Set<string>()
		for (const device of nutDevices) {
			if (!systemId && !seenSystems.has(device.system)) {
				seenSystems.add(device.system)
				const name = allSystems[device.system]?.name ?? ""
				if (name.length > result.longestName.length) {
					result.longestName = name
				}
			}
			if ((device.model ?? "").length > result.longestModel.length) {
				result.longestModel = device.model ?? ""
			}
			if ((device.name ?? "").length > result.longestDevice.length) {
				result.longestDevice = device.name ?? ""
			}
		}
		return result
	}, [nutDevices, systemId, allSystems])

	const openSheet = (device: NutDeviceRecord) => {
		setActiveDeviceId(device.id)
		setSheetOpen(true)
	}

	useEffect(() => {
		const controller = new AbortController()

		pb.collection<NutDeviceRecord>("nut_devices")
			.getFullList({
				filter: systemId ? pb.filter("system = {:system}", { system: systemId }) : undefined,
				fields: NUT_DEVICE_FIELDS,
				signal: controller.signal,
			})
			.then(setNutDevices)
			.catch((err) => {
				if (!err.isAbort) {
					setNutDevices([])
				}
			})

		return () => controller.abort()
	}, [systemId])

	useEffect(() => {
		let unsubscribe: (() => void) | undefined
		const pbOptions = systemId
			? { fields: NUT_DEVICE_FIELDS, filter: pb.filter("system = {:system}", { system: systemId }) }
			: { fields: NUT_DEVICE_FIELDS }

		;(async () => {
			try {
				unsubscribe = await pb.collection("nut_devices").subscribe(
					"*",
					(event) => {
						const record = event.record as NutDeviceRecord
						setNutDevices((currentDevices) => {
							const devices = currentDevices ?? []
							const matchesSystemScope = !systemId || record.system === systemId

							if (event.action === "delete") {
								return devices.filter((device) => device.id !== record.id)
							}

							if (!matchesSystemScope) {
								return devices.filter((device) => device.id !== record.id)
							}

							const existingIndex = devices.findIndex((device) => device.id === record.id)
							if (existingIndex === -1) {
								return [record, ...devices]
							}

							const next = [...devices]
							next[existingIndex] = record
							return next
						})
					},
					pbOptions
				)
			} catch (error) {
				console.error("Failed to subscribe to NUT device updates:", error)
			}
		})()

		return () => {
			unsubscribe?.()
		}
	}, [systemId])

	const handleRowRefresh = useCallback(async (device: NutDeviceRecord) => {
		if (!device.system) return
		setRowActionState({ type: "refresh", id: device.id })
		try {
			await pb.send("/api/beszel/nut/refresh", {
				method: "POST",
				query: { system: device.system },
			})
		} catch (error) {
			console.error("Failed to refresh NUT device:", error)
		} finally {
			setRowActionState((state) => (state?.id === device.id ? null : state))
		}
	}, [])

	const handleDeleteDevice = useCallback(async (device: NutDeviceRecord) => {
		setRowActionState({ type: "delete", id: device.id })
		try {
			await pb.collection("nut_devices").delete(device.id)
		} catch (error) {
			console.error("Failed to delete NUT device:", error)
		} finally {
			setRowActionState((state) => (state?.id === device.id ? null : state))
		}
	}, [])

	const actionColumn = useMemo<ColumnDef<NutDeviceRecord>>(
		() => ({
			id: "actions",
			enableSorting: false,
			header: () => (
				<span className="sr-only">
					<Trans>Actions</Trans>
				</span>
			),
			cell: ({ row }) => {
				const device = row.original
				const isRowRefreshing = rowActionState?.id === device.id && rowActionState.type === "refresh"
				const isRowDeleting = rowActionState?.id === device.id && rowActionState.type === "delete"

				return (
					<div className="flex justify-end">
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									variant="ghost"
									size="icon"
									className="size-10"
									onClick={(event) => event.stopPropagation()}
									onMouseDown={(event) => event.stopPropagation()}
								>
									<span className="sr-only">
										<Trans>Open menu</Trans>
									</span>
									<MoreHorizontalIcon className="w-5" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" onClick={(event) => event.stopPropagation()}>
								<DropdownMenuItem
									onClick={(event) => {
										event.stopPropagation()
										handleRowRefresh(device)
									}}
									disabled={isRowRefreshing || isRowDeleting}
								>
									<RefreshCwIcon className={cn("me-2.5 size-4", isRowRefreshing && "animate-spin")} />
									<Trans>Refresh</Trans>
								</DropdownMenuItem>
								<DropdownMenuSeparator />
								<DropdownMenuItem
									onClick={(event) => {
										event.stopPropagation()
										handleDeleteDevice(device)
									}}
									disabled={isRowDeleting}
								>
									<Trash2Icon className="me-2.5 size-4" />
									<Trans>Delete</Trans>
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				)
			},
		}),
		[handleRowRefresh, handleDeleteDevice, rowActionState]
	)

	const tableColumns = useMemo(() => {
		const columns = createColumns(longestName, longestModel, longestDevice)
		const baseColumns = systemId ? columns.filter((col) => col.id !== "system") : columns
		return isReadOnlyUser() ? baseColumns : [...baseColumns, actionColumn]
	}, [systemId, actionColumn, longestName, longestModel, longestDevice])

	const table = useReactTable({
		data: nutDevices || ([] as NutDeviceRecord[]),
		columns: tableColumns,
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onRowSelectionChange: setRowSelection,
		state: {
			sorting,
			columnFilters,
			rowSelection,
			globalFilter,
		},
		onGlobalFilterChange: setGlobalFilter,
		globalFilterFn: (row, _columnId, filterValue) => {
			const device = row.original
			const systemName = $allSystemsById.get()[device.system]?.name ?? ""
			const name = device.name ?? ""
			const model = device.model ?? ""
			const health = device.health ?? ""
			const type = device.device_type ?? ""
			const searchString = `${systemName} ${name} ${model} ${health} ${type}`.toLowerCase()
			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})
	const rows = table.getRowModel().rows

	if (systemId && !nutDevices?.length && !columnFilters.length) {
		return null
	}

	return (
		<div>
			<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
				<CardHeader className="p-0 mb-3 sm:mb-4">
					<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
						<div className="px-2 sm:px-1">
							<CardTitle className="mb-2">UPS / PDU</CardTitle>
							<CardDescription className="flex">
								<Trans>Click on a device to view more information.</Trans>
							</CardDescription>
						</div>
						<div className="relative ms-auto w-full max-w-full md:w-64">
							<Input
								placeholder={t`Filter...`}
								value={globalFilter}
								onChange={(event) => setGlobalFilter(event.target.value)}
								className="px-4 w-full max-w-full md:w-64"
							/>
							{globalFilter && (
								<Button
									type="button"
									variant="ghost"
									size="icon"
									aria-label={t`Clear`}
									className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
									onClick={() => setGlobalFilter("")}
								>
									<XIcon className="h-4 w-4" />
								</Button>
							)}
						</div>
					</div>
				</CardHeader>
				<NutDevicesTable table={table} rows={rows} colLength={tableColumns.length} data={nutDevices} openSheet={openSheet} />
			</Card>
			<DeviceSheet deviceId={activeDeviceId} open={sheetOpen} onOpenChange={setSheetOpen} />
		</div>
	)
}

const NutDevicesTable = memo(function NutDevicesTable({
	table,
	rows,
	colLength,
	data,
	openSheet,
}: {
	table: TableType<NutDeviceRecord>
	rows: Row<NutDeviceRecord>[]
	colLength: number
	data: NutDeviceRecord[] | undefined
	openSheet: (device: NutDeviceRecord) => void
}) {
	const scrollRef = useRef<HTMLDivElement>(null)

	const virtualizer = useVirtualizer<HTMLDivElement, HTMLTableRowElement>({
		count: rows.length,
		estimateSize: () => 65,
		getScrollElement: () => scrollRef.current,
		overscan: 5,
	})
	const virtualRows = virtualizer.getVirtualItems()

	const paddingTop = Math.max(0, virtualRows[0]?.start ?? 0 - virtualizer.options.scrollMargin)
	const paddingBottom = Math.max(0, virtualizer.getTotalSize() - (virtualRows[virtualRows.length - 1]?.end ?? 0))

	return (
		<div
			className={cn(
				"h-min max-h-[calc(100dvh-17rem)] max-w-full relative overflow-auto rounded-md border",
				(!rows.length || rows.length > 2) && "min-h-50"
			)}
			ref={scrollRef}
		>
			<div style={{ height: `${virtualizer.getTotalSize() + 48}px`, paddingTop, paddingBottom }}>
				<table className="w-full text-sm text-nowrap">
					<NutTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return (
									<NutDeviceTableRow key={row.id} row={row} virtualRow={virtualRow} openSheet={openSheet} />
								)
							})
						) : (
							<TableCell colSpan={colLength} className="h-37 text-center pointer-events-none">
								{data ? (
									<Trans>No results.</Trans>
								) : (
									<LoaderCircleIcon className="animate-spin size-10 opacity-60 mx-auto" />
								)}
							</TableCell>
						)}
					</TableBody>
				</table>
			</div>
		</div>
	)
})

function NutTableHead({ table }: { table: TableType<NutDeviceRecord> }) {
	return (
		<TableHeader className="sticky top-0 z-50 w-full border-b-2">
			{table.getHeaderGroups().map((headerGroup) => (
				<TableRow key={headerGroup.id}>
					{headerGroup.headers.map((header) => (
						<TableHead key={header.id} className="px-2">
							{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
						</TableHead>
					))}
				</TableRow>
			))}
		</TableHeader>
	)
}

const NutDeviceTableRow = memo(function NutDeviceTableRow({
	row,
	virtualRow,
	openSheet,
}: {
	row: Row<NutDeviceRecord>
	virtualRow: VirtualItem
	openSheet: (device: NutDeviceRecord) => void
}) {
	return (
		<TableRow
			data-state={row.getIsSelected() && "selected"}
			className="cursor-pointer"
			onClick={() => openSheet(row.original)}
		>
			{row.getVisibleCells().map((cell) => (
				<TableCell
					key={cell.id}
					className="md:ps-5 py-0"
					style={{
						height: virtualRow.size,
					}}
				>
					{flexRender(cell.column.columnDef.cell, cell.getContext())}
				</TableCell>
			))}
		</TableRow>
	)
})

function DeviceSheet({
	deviceId,
	open,
	onOpenChange,
}: {
	deviceId: string | null
	open: boolean
	onOpenChange: (open: boolean) => void
}) {
	const [device, setDevice] = useState<NutDeviceRecord | null>(null)
	const [isLoading, setIsLoading] = useState(false)

	useEffect(() => {
		if (!deviceId) {
			setDevice(null)
			return
		}
		if (!open) return
		setIsLoading(true)
		pb.collection<NutDeviceRecord>("nut_devices")
			.getOne(deviceId)
			.then(setDevice)
			.catch(() => setDevice(null))
			.finally(() => setIsLoading(false))
	}, [open, deviceId])

	const unknown = "Unknown"
	const deviceName = device?.name || unknown
	const model = device?.model || unknown
	const manufacturer = device?.manufacturer
	const serial = device?.serial
	const firmware = device?.firmware
	const driver = device?.driver
	const health = device?.health || unknown
	const deviceType = device?.device_type || unknown
	const batteryCharge = device?.battery_charge
	const batteryVoltage = device?.battery_voltage
	const batteryRuntime = device?.battery_runtime
	const inputVoltage = device?.input_voltage
	const outputVoltage = device?.output_voltage
	const inputNominal = device?.input_nominal
	const load = device?.load
	const outputCurrent = device?.output_current
	const outputPower = device?.output_power
	const outlets = device?.outlets || []

	const isHealthy = health === "ONLINE"
	const isCritical = health === "FAULT" || health === "LOW_BATTERY" || health === "OVERLOAD"

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-220 gap-0">
				<SheetHeader className="mb-0 border-b">
					<SheetTitle>
						<Trans>UPS / PDU Details</Trans> - {deviceName}
					</SheetTitle>
					<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
						{model}
						{manufacturer && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								{manufacturer}
							</>
						)}
						{serial && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<Tooltip>
									<TooltipTrigger asChild>
										<span>{serial}</span>
									</TooltipTrigger>
									<TooltipContent>
										<Trans>Serial Number</Trans>
									</TooltipContent>
								</Tooltip>
							</>
						)}
						{firmware && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<Tooltip>
									<TooltipTrigger asChild>
										<span>{firmware}</span>
									</TooltipTrigger>
									<TooltipContent>
										<Trans>Firmware</Trans>
									</TooltipContent>
								</Tooltip>
							</>
						)}
					</SheetDescription>
				</SheetHeader>
				<div className="flex-1 overflow-hidden p-4 flex flex-col gap-4">
					{isLoading ? (
						<div className="flex justify-center py-8">
							<LoaderCircleIcon className="animate-spin size-10 opacity-60" />
						</div>
					) : (
						<>
							<Alert className="pb-3 shrink-0">
								{isHealthy ? (
									<CheckCircle2Icon className="size-4" />
								) : isCritical ? (
									<XCircleIcon className="size-4" />
								) : (
									<AlertTriangleIcon className="size-4" />
								)}
								<AlertTitle>
									<Trans>Status</Trans>: {health.replace("_", " ")}
								</AlertTitle>
								{driver && (
									<AlertDescription>
										<Trans>Driver</Trans>: {driver}
									</AlertDescription>
								)}
							</Alert>

							<div className="grid grid-cols-2 gap-3 text-sm">
								<div className="rounded-md border p-3">
									<div className="text-muted-foreground text-xs mb-1">
										<Trans>Device Type</Trans>
									</div>
									<div className="font-medium uppercase">{deviceType}</div>
								</div>
								{batteryCharge != null && batteryCharge > 0 && (
									<div className="rounded-md border p-3">
										<div className="text-muted-foreground text-xs mb-1">
											<Trans>Battery</Trans>
										</div>
										<div className="font-medium">
											{toFixedFloat(batteryCharge, 0)}%
											{batteryVoltage != null && batteryVoltage > 0 && (
												<span className="text-muted-foreground ml-2">
													{toFixedFloat(batteryVoltage, 1)} V
												</span>
											)}
										</div>
										{batteryRuntime != null && batteryRuntime > 0 && (
											<div className="text-muted-foreground text-xs mt-1">
												<Trans>Runtime</Trans>: {secondsToString(batteryRuntime, "minute")}
											</div>
										)}
									</div>
								)}
								{inputVoltage != null && inputVoltage > 0 && (
									<div className="rounded-md border p-3">
										<div className="text-muted-foreground text-xs mb-1">
											<Trans>Input Voltage</Trans>
										</div>
										<div className="font-medium">
											{toFixedFloat(inputVoltage, 1)} V
											{inputNominal != null && inputNominal > 0 && (
												<span className="text-muted-foreground ml-2">
													/ {toFixedFloat(inputNominal, 0)} V
												</span>
											)}
										</div>
									</div>
								)}
								{outputVoltage != null && outputVoltage > 0 && (
									<div className="rounded-md border p-3">
										<div className="text-muted-foreground text-xs mb-1">
											<Trans>Output Voltage</Trans>
										</div>
										<div className="font-medium">{toFixedFloat(outputVoltage, 1)} V</div>
									</div>
								)}
								{load != null && load > 0 && (
									<div className="rounded-md border p-3">
										<div className="text-muted-foreground text-xs mb-1">
											<Trans>Load</Trans>
										</div>
										<div className="font-medium">{toFixedFloat(load, 0)}%</div>
									</div>
								)}
								{outputPower != null && outputPower > 0 && (
									<div className="rounded-md border p-3">
										<div className="text-muted-foreground text-xs mb-1">
											<Trans>Output Power</Trans>
										</div>
										<div className="font-medium">
											{toFixedFloat(outputPower, 0)} W
											{outputCurrent != null && outputCurrent > 0 && (
												<span className="text-muted-foreground ml-2">
													{toFixedFloat(outputCurrent, 1)} A
												</span>
											)}
										</div>
									</div>
								)}
							</div>

							{outlets.length > 0 && (
								<div className="rounded-md border min-h-0 flex flex-col">
									<Table>
										<TableHeader className="sticky top-0 z-10">
											<TableRow>
												<TableHead>
													<Trans>Outlet</Trans>
												</TableHead>
												<TableHead>
													<Trans>Status</Trans>
												</TableHead>
												<TableHead>
													<Trans>Power</Trans>
												</TableHead>
												<TableHead>
													<Trans>Current</Trans>
												</TableHead>
												<TableHead>
													<Trans>Voltage</Trans>
												</TableHead>
											</TableRow>
										</TableHeader>
										<TableBody>
											{outlets.map((outlet: NutOutlet) => (
												<TableRow key={outlet.id}>
													<TableCell className="font-medium">
														{outlet.d || outlet.id}
													</TableCell>
													<TableCell>
														<Badge
															variant={
																outlet.s === "on"
																	? "success"
																	: outlet.s === "off"
																	? "danger"
																	: "outline"
															}
														>
															{outlet.s || "unknown"}
														</Badge>
													</TableCell>
													<TableCell>{outlet.p ? `${toFixedFloat(outlet.p, 0)} W` : "N/A"}</TableCell>
													<TableCell>{outlet.c ? `${toFixedFloat(outlet.c, 1)} A` : "N/A"}</TableCell>
													<TableCell>{outlet.v ? `${toFixedFloat(outlet.v, 1)} V` : "N/A"}</TableCell>
												</TableRow>
											))}
										</TableBody>
									</Table>
								</div>
							)}
						</>
					)}
				</div>
			</SheetContent>
		</Sheet>
	)
}