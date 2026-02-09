import { t } from "@lingui/core/macro"
import {
	type ColumnDef,
	type ColumnFiltersState,
	type Column,
	type SortingState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table"
import {
	Activity,
	Box,
	Clock,
	HardDrive,
	BinaryIcon,
	RotateCwIcon,
	LoaderCircleIcon,
	CheckCircle2Icon,
	XCircleIcon,
	ArrowLeftRightIcon,
	MoreHorizontalIcon,
	RefreshCwIcon,
	ServerIcon,
	Trash2Icon,
	XIcon,
} from "lucide-react"
import { Card, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { pb } from "@/lib/api"
import type { SmartDeviceRecord, SmartAttribute } from "@/types"
import {
	formatBytes,
	toFixedFloat,
	formatTemperature,
	cn,
	secondsToString,
	hourWithSeconds,
	formatShortDate,
} from "@/lib/utils"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { $allSystemsById } from "@/lib/stores"
import { ThermometerIcon } from "@/components/ui/icons"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Separator } from "@/components/ui/separator"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useCallback, useMemo, useEffect, useState } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

// Column definition for S.M.A.R.T. attributes table
export const smartColumns: ColumnDef<SmartAttribute>[] = [
	{
		accessorKey: "id",
		header: "ID",
	},
	{
		accessorFn: (row) => row.n,
		header: "Name",
	},
	{
		accessorFn: (row) => row.rs || row.rv?.toString(),
		header: "Value",
	},
	{
		accessorKey: "v",
		header: "Normalized",
	},
	{
		accessorKey: "w",
		header: "Worst",
	},
	{
		accessorKey: "t",
		header: "Threshold",
	},
	{
		// accessorFn: (row) => row.wf,
		accessorKey: "wf",
		header: "Failing",
	},
]

// Function to format capacity display
function formatCapacity(bytes: number): string {
	const { value, unit } = formatBytes(bytes)
	return `${toFixedFloat(value, value >= 10 ? 1 : 2)} ${unit}`
}

const SMART_DEVICE_FIELDS = "id,system,name,model,state,capacity,temp,type,hours,cycles,updated"

export const columns: ColumnDef<SmartDeviceRecord>[] = [
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
			return <span className="ms-1.5 xl:w-30 block truncate">{allSystems[getValue() as string]?.name ?? ""}</span>
		},
	},
	{
		accessorKey: "name",
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		header: ({ column }) => <HeaderButton column={column} name={t`Device`} Icon={HardDrive} />,
		cell: ({ getValue }) => (
			<div className="font-medium max-w-40 truncate ms-1.5" title={getValue() as string}>
				{getValue() as string}
			</div>
		),
	},
	{
		accessorKey: "model",
		sortingFn: (a, b) => a.original.model.localeCompare(b.original.model),
		header: ({ column }) => <HeaderButton column={column} name={t`Model`} Icon={Box} />,
		cell: ({ getValue }) => (
			<div className="max-w-48 truncate ms-1.5" title={getValue() as string}>
				{getValue() as string}
			</div>
		),
	},
	{
		accessorKey: "capacity",
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Capacity`} Icon={BinaryIcon} />,
		cell: ({ getValue }) => <span className="ms-1.5">{formatCapacity(getValue() as number)}</span>,
	},
	{
		accessorKey: "state",
		header: ({ column }) => <HeaderButton column={column} name={t`Status`} Icon={Activity} />,
		cell: ({ getValue }) => {
			const status = getValue() as string
			return (
				<div className="ms-1.5">
					<Badge variant={status === "PASSED" ? "success" : status === "FAILED" ? "danger" : "warning"}>{status}</Badge>
				</div>
			)
		},
	},
	{
		accessorKey: "type",
		sortingFn: (a, b) => a.original.type.localeCompare(b.original.type),
		header: ({ column }) => <HeaderButton column={column} name={t`Type`} Icon={ArrowLeftRightIcon} />,
		cell: ({ getValue }) => (
			<div className="ms-1.5">
				<Badge variant="outline" className="uppercase">
					{getValue() as string}
				</Badge>
			</div>
		),
	},
	{
		accessorKey: "hours",
		invertSorting: true,
		header: ({ column }) => (
			<HeaderButton column={column} name={t({ message: "Power On", comment: "Power On Time" })} Icon={Clock} />
		),
		cell: ({ getValue }) => {
			const hours = (getValue() ?? 0) as number
			if (!hours && hours !== 0) {
				return <div className="text-sm text-muted-foreground ms-1.5">N/A</div>
			}
			const seconds = hours * 3600
			return (
				<div className="text-sm ms-1.5">
					<div>{secondsToString(seconds, "hour")}</div>
					<div className="text-muted-foreground text-xs">{secondsToString(seconds, "day")}</div>
				</div>
			)
		},
	},
	{
		accessorKey: "cycles",
		invertSorting: true,
		header: ({ column }) => (
			<HeaderButton column={column} name={t({ message: "Cycles", comment: "Power Cycles" })} Icon={RotateCwIcon} />
		),
		cell: ({ getValue }) => {
			const cycles = getValue() as number | undefined
			if (!cycles && cycles !== 0) {
				return <div className="text-muted-foreground ms-1.5">N/A</div>
			}
			return <span className="ms-1.5">{cycles.toLocaleString()}</span>
		},
	},
	{
		accessorKey: "temp",
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Temp`} Icon={ThermometerIcon} />,
		cell: ({ getValue }) => {
			const temp = getValue() as number | undefined | null
			// Most devices won't report a real 0C temperature; treat 0 as "unknown".
			if (temp == null || temp === 0) {
				return <div className="text-muted-foreground ms-1.5">N/A</div>
			}
			const { value, unit } = formatTemperature(temp)
			return <span className="ms-1.5">{`${value} ${unit}`}</span>
		},
	},
	// {
	// 	accessorKey: "serial",
	// 	sortingFn: (a, b) => a.original.serial.localeCompare(b.original.serial),
	// 	header: ({ column }) => <HeaderButton column={column} name={t`Serial Number`} Icon={HashIcon} />,
	// 	cell: ({ getValue }) => <span className="ms-1.5">{getValue() as string}</span>,
	// },
	// {
	// 	accessorKey: "firmware",
	// 	sortingFn: (a, b) => a.original.firmware.localeCompare(b.original.firmware),
	// 	header: ({ column }) => <HeaderButton column={column} name={t`Firmware`} Icon={CpuIcon} />,
	// 	cell: ({ getValue }) => <span className="ms-1.5">{getValue() as string}</span>,
	// },
	{
		id: "updated",
		invertSorting: true,
		accessorFn: (record) => record.updated,
		header: ({ column }) => <HeaderButton column={column} name={t`Updated`} Icon={Clock} />,
		cell: ({ getValue }) => {
			const timestamp = getValue() as string
			// if today, use hourWithSeconds, otherwise use formatShortDate
			const formatter =
				new Date(timestamp).toDateString() === new Date().toDateString() ? hourWithSeconds : formatShortDate
			return <span className="ms-1.5 tabular-nums">{formatter(timestamp)}</span>
		},
	},
]

function HeaderButton({
	column,
	name,
	Icon,
}: {
	column: Column<SmartDeviceRecord>
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

export default function DisksTable({ systemId }: { systemId?: string }) {
	const [sorting, setSorting] = useState<SortingState>([{ id: systemId ? "name" : "system", desc: false }])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [rowSelection, setRowSelection] = useState({})
	const [smartDevices, setSmartDevices] = useState<SmartDeviceRecord[] | undefined>(undefined)
	const [activeDiskId, setActiveDiskId] = useState<string | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const [rowActionState, setRowActionState] = useState<{ type: "refresh" | "delete"; id: string } | null>(null)
	const [globalFilter, setGlobalFilter] = useState("")

	const openSheet = (disk: SmartDeviceRecord) => {
		setActiveDiskId(disk.id)
		setSheetOpen(true)
	}

	// Fetch smart devices
	useEffect(() => {
		const controller = new AbortController()

		pb.collection<SmartDeviceRecord>("smart_devices")
			.getFullList({
				filter: systemId ? pb.filter("system = {:system}", { system: systemId }) : undefined,
				fields: SMART_DEVICE_FIELDS,
				signal: controller.signal,
			})
			.then(setSmartDevices)
			.catch((err) => {
				if (!err.isAbort) {
					setSmartDevices([])
				}
			})

		return () => controller.abort()
	}, [systemId])

	// Subscribe to updates
	useEffect(() => {
		let unsubscribe: (() => void) | undefined
		const pbOptions = systemId
			? { fields: SMART_DEVICE_FIELDS, filter: pb.filter("system = {:system}", { system: systemId }) }
			: { fields: SMART_DEVICE_FIELDS }

			; (async () => {
				try {
					unsubscribe = await pb.collection("smart_devices").subscribe(
						"*",
						(event) => {
							const record = event.record as SmartDeviceRecord
							setSmartDevices((currentDevices) => {
								const devices = currentDevices ?? []
								const matchesSystemScope = !systemId || record.system === systemId

								if (event.action === "delete") {
									return devices.filter((device) => device.id !== record.id)
								}

								if (!matchesSystemScope) {
									// Record moved out of scope; ensure it disappears locally.
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
					console.error("Failed to subscribe to SMART device updates:", error)
				}
			})()

		return () => {
			unsubscribe?.()
		}
	}, [systemId])

	const handleRowRefresh = useCallback(async (disk: SmartDeviceRecord) => {
		if (!disk.system) return
		setRowActionState({ type: "refresh", id: disk.id })
		try {
			await pb.send("/api/beszel/smart/refresh", {
				method: "POST",
				query: { system: disk.system },
			})
		} catch (error) {
			console.error("Failed to refresh SMART device:", error)
		} finally {
			setRowActionState((state) => (state?.id === disk.id ? null : state))
		}
	}, [])

	const handleDeleteDevice = useCallback(async (disk: SmartDeviceRecord) => {
		setRowActionState({ type: "delete", id: disk.id })
		try {
			await pb.collection("smart_devices").delete(disk.id)
			// setSmartDevices((current) => current?.filter((device) => device.id !== disk.id))
		} catch (error) {
			console.error("Failed to delete SMART device:", error)
		} finally {
			setRowActionState((state) => (state?.id === disk.id ? null : state))
		}
	}, [])

	const actionColumn = useMemo<ColumnDef<SmartDeviceRecord>>(
		() => ({
			id: "actions",
			enableSorting: false,
			header: () => (
				<span className="sr-only">
					<Trans>Actions</Trans>
				</span>
			),
			cell: ({ row }) => {
				const disk = row.original
				const isRowRefreshing = rowActionState?.id === disk.id && rowActionState.type === "refresh"
				const isRowDeleting = rowActionState?.id === disk.id && rowActionState.type === "delete"

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
										handleRowRefresh(disk)
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
										handleDeleteDevice(disk)
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

	// Filter columns based on whether systemId is provided
	const tableColumns = useMemo(() => {
		const baseColumns = systemId ? columns.filter((col) => col.id !== "system") : columns
		return [...baseColumns, actionColumn]
	}, [systemId, actionColumn])

	const table = useReactTable({
		data: smartDevices || ([] as SmartDeviceRecord[]),
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
			const disk = row.original
			const systemName = $allSystemsById.get()[disk.system]?.name ?? ""
			const device = disk.name ?? ""
			const model = disk.model ?? ""
			const status = disk.state ?? ""
			const type = disk.type ?? ""
			const searchString = `${systemName} ${device} ${model} ${status} ${type}`.toLowerCase()
			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})

	// Hide the table on system pages if there's no data, but always show on global page
	if (systemId && !smartDevices?.length && !columnFilters.length) {
		return null
	}

	return (
		<div>
			<Card className="p-6 @container w-full">
				<CardHeader className="p-0 mb-4">
					<div className="grid md:flex gap-5 w-full items-end">
						<div className="px-2 sm:px-1">
							<CardTitle className="mb-2">S.M.A.R.T.</CardTitle>
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
				<div className="rounded-md border text-nowrap">
					<Table>
						<TableHeader>
							{table.getHeaderGroups().map((headerGroup) => (
								<TableRow key={headerGroup.id}>
									{headerGroup.headers.map((header) => {
										return (
											<TableHead key={header.id} className="px-2">
												{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
											</TableHead>
										)
									})}
								</TableRow>
							))}
						</TableHeader>
						<TableBody>
							{table.getRowModel().rows?.length ? (
								table.getRowModel().rows.map((row) => (
									<TableRow
										key={row.id}
										data-state={row.getIsSelected() && "selected"}
										className="cursor-pointer"
										onClick={() => openSheet(row.original)}
									>
										{row.getVisibleCells().map((cell) => (
											<TableCell key={cell.id} className="md:ps-5">
												{flexRender(cell.column.columnDef.cell, cell.getContext())}
											</TableCell>
										))}
									</TableRow>
								))
							) : (
								<TableRow>
									<TableCell colSpan={tableColumns.length} className="h-24 text-center">
										{smartDevices ? (
											t`No results.`
										) : (
											<LoaderCircleIcon className="animate-spin size-10 opacity-60 mx-auto" />
										)}
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</div>
			</Card>
			<DiskSheet diskId={activeDiskId} open={sheetOpen} onOpenChange={setSheetOpen} />
		</div>
	)
}

function DiskSheet({
	diskId,
	open,
	onOpenChange,
}: {
	diskId: string | null
	open: boolean
	onOpenChange: (open: boolean) => void
}) {
	const [disk, setDisk] = useState<SmartDeviceRecord | null>(null)
	const [isLoading, setIsLoading] = useState(false)

	// Fetch full device record (including attributes) when sheet opens
	useEffect(() => {
		if (!diskId) {
			setDisk(null)
			return
		}
		// Only fetch when opening, not when closing (keeps data visible during close animation)
		if (!open) return
		setIsLoading(true)
		pb.collection<SmartDeviceRecord>("smart_devices")
			.getOne(diskId)
			.then(setDisk)
			.catch(() => setDisk(null))
			.finally(() => setIsLoading(false))
	}, [open, diskId])

	const smartAttributes = disk?.attributes || []

	// Find all attributes where when failed is not empty
	const failedAttributes = smartAttributes.filter((attr) => attr.wf && attr.wf.trim() !== "")

	// Filter columns to only show those that have values in at least one row
	const visibleColumns = useMemo(() => {
		return smartColumns.filter((column) => {
			const accessorKey = "accessorKey" in column ? (column.accessorKey as keyof SmartAttribute | undefined) : undefined
			if (!accessorKey) {
				return true
			}
			// Check if any row has a non-empty value for this column
			return smartAttributes.some((attr) => {
				return attr[accessorKey] !== undefined
			})
		})
	}, [smartAttributes])

	const table = useReactTable({
		data: smartAttributes,
		columns: visibleColumns,
		getCoreRowModel: getCoreRowModel(),
	})

	const unknown = "Unknown"
	const deviceName = disk?.name || unknown
	const model = disk?.model || unknown
	const capacity = disk?.capacity ? formatCapacity(disk.capacity) : unknown
	const serialNumber = disk?.serial || unknown
	const firmwareVersion = disk?.firmware || unknown
	const status = disk?.state || unknown

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-220 gap-0">
				<SheetHeader className="mb-0 border-b">
					<SheetTitle>
						<Trans>S.M.A.R.T. Details</Trans> - {deviceName}
					</SheetTitle>
					<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
						{model}
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						{capacity}
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						<Tooltip>
							<TooltipTrigger asChild>
								<span>{serialNumber}</span>
							</TooltipTrigger>
							<TooltipContent>
								<Trans>Serial Number</Trans>
							</TooltipContent>
						</Tooltip>
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						<Tooltip>
							<TooltipTrigger asChild>
								<span>{firmwareVersion}</span>
							</TooltipTrigger>
							<TooltipContent>
								<Trans>Firmware</Trans>
							</TooltipContent>
						</Tooltip>
					</SheetDescription>
				</SheetHeader>
				<div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
					{isLoading ? (
						<div className="flex justify-center py-8">
							<LoaderCircleIcon className="animate-spin size-10 opacity-60" />
						</div>
					) : (
						<>
							<Alert className="pb-3">
								{status === "PASSED" ? <CheckCircle2Icon className="size-4" /> : <XCircleIcon className="size-4" />}
								<AlertTitle>
									<Trans>S.M.A.R.T. Self-Test</Trans>: {status}
								</AlertTitle>
								{failedAttributes.length > 0 && (
									<AlertDescription>
										<Trans>Failed Attributes:</Trans> {failedAttributes.map((attr) => attr.n).join(", ")}
									</AlertDescription>
								)}
							</Alert>
							{smartAttributes.length > 0 ? (
								<div className="rounded-md border overflow-auto">
									<Table>
										<TableHeader>
											{table.getHeaderGroups().map((headerGroup) => (
												<TableRow key={headerGroup.id}>
													{headerGroup.headers.map((header) => (
														<TableHead key={header.id}>
															{header.isPlaceholder
																? null
																: flexRender(header.column.columnDef.header, header.getContext())}
														</TableHead>
													))}
												</TableRow>
											))}
										</TableHeader>
										<TableBody>
											{table.getRowModel().rows.map((row) => {
												// Check if the attribute is failed
												const isFailedAttribute = row.original.wf && row.original.wf.trim() !== ""

												return (
													<TableRow key={row.id} className={isFailedAttribute ? "text-red-600 dark:text-red-400" : ""}>
														{row.getVisibleCells().map((cell) => (
															<TableCell key={cell.id}>
																{flexRender(cell.column.columnDef.cell, cell.getContext())}
															</TableCell>
														))}
													</TableRow>
												)
											})}
										</TableBody>
									</Table>
								</div>
							) : (
								<div className="text-center py-8 text-muted-foreground">
									<Trans>No S.M.A.R.T. attributes available for this device.</Trans>
								</div>
							)}
						</>
					)}
				</div>
			</SheetContent>
		</Sheet>
	)
}
