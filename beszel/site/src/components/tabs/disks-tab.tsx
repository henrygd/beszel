"use client"

import * as React from "react"
import {
	ColumnDef,
	ColumnFiltersState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getPaginationRowModel,
	getSortedRowModel,
	SortingState,
	useReactTable,
	VisibilityState,
} from "@tanstack/react-table"
import { Activity, Box, Binary, Container, ChevronDown, Clock, HardDrive, Thermometer, Tags, MoreHorizontal } from "lucide-react"

import { Button } from "../ui/button"
import { Card, CardHeader, CardTitle, CardDescription } from "../ui/card"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "../ui/dialog"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "../ui/dropdown-menu"
import { Input } from "../ui/input"
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "../ui/table"
import { Badge } from "../ui/badge"
import { SmartData, SmartAttribute } from "@/types"


// Column definition for S.M.A.R.T. attributes table
export const smartColumns: ColumnDef<SmartAttribute>[] = [
	{
		accessorKey: "id",
		header: "ID",
		cell: ({ row }) => {
			const id = row.getValue("id") as number | undefined
			return <div className="font-medium">{id || ""}</div>
		},
		enableSorting: false,
	},
	{
		accessorKey: "n",
		header: "Name",
		cell: ({ row }) => (
			<div className="font-medium">{row.getValue("n")}</div>
		),
		enableSorting: false,
	},
	{
		accessorKey: "rs",
		header: "Value",
		cell: ({ row }) => {
			// if raw string is not empty, use it, otherwise use raw value
			const rawString = row.getValue("rs") as string | undefined
			const rawValue = row.original.rv
			const displayValue = rawString || rawValue?.toString() || "-"
			return <div className="font-mono text-sm">{displayValue}</div>
		},
		enableSorting: false,
	},
	{
		accessorKey: "v",
		header: "Normalized",
		cell: ({ row }) => (
			<div className="font-medium">{row.getValue("v")}</div>
		),
		enableSorting: false,
	},
	{
		accessorKey: "w",
		header: "Worst",
		cell: ({ row }) => {
			const worst = row.getValue("w") as number | undefined
			return <div>{worst || ""}</div>
		},
		enableSorting: false,
	},
	{
		accessorKey: "t",
		header: "Threshold",
		cell: ({ row }) => {
			const threshold = row.getValue("t") as number | undefined
			return <div>{threshold || ""}</div>
		},
		enableSorting: false,
	},
	{
		accessorKey: "f",
		header: "Flags",
		cell: ({ row }) => {
			const flags = row.getValue("f") as string | undefined
			return <div className="font-mono text-sm">{flags || ""}</div>
		},
		enableSorting: false,
	},
	{
		accessorKey: "wf",
		header: "Failing",
		cell: ({ row }) => {
			const whenFailed = row.getValue("wf") as string | undefined
			return <div className="font-mono text-sm">{whenFailed || ""}</div>
		},
		enableSorting: false,
	},
]

export type DiskInfo = {
	device: string
	model: string
	serialNumber: string
	firmwareVersion: string
	capacity: string
	status: string
	temperature: number
	deviceType: string
	powerOnHours?: number
	powerCycles?: number
}

// Function to format capacity display
function formatCapacity(bytes: number): string {
	const units = [
		{ name: 'PB', size: 1024 ** 5 },
		{ name: 'TB', size: 1024 ** 4 },
		{ name: 'GB', size: 1024 ** 3 },
		{ name: 'MB', size: 1024 ** 2 },
		{ name: 'KB', size: 1024 ** 1 },
		{ name: 'B', size: 1 }
	]
	
	for (const unit of units) {
		if (bytes >= unit.size) {
			const value = bytes / unit.size
			// For bytes, don't show decimals; for other units show one decimal place
			const decimals = unit.name === 'B' ? 0 : 1
			return `${value.toFixed(decimals)} ${unit.name}`
		}
	}
	
	return '0 B'
}

// Function to convert SmartData to DiskInfo
function convertSmartDataToDiskInfo(smartDataRecord: Record<string, SmartData>): DiskInfo[] {
	return Object.entries(smartDataRecord).map(([key, smartData]) => ({
		device: smartData.dn || key,
		model: smartData.mn || "Unknown",
		serialNumber: smartData.sn || "Unknown",
		firmwareVersion: smartData.fv || "Unknown",
		capacity: smartData.c ? formatCapacity(smartData.c) : "Unknown",
		status: smartData.s || "Unknown",
		temperature: smartData.t || 0,
		deviceType: smartData.dt || "Unknown",
		// These fields need to be extracted from SmartAttribute if available
		powerOnHours: smartData.a?.find(attr => attr.n.toLowerCase().includes("poweronhours") || attr.n.toLowerCase().includes("power_on_hours"))?.rv,
		powerCycles: smartData.a?.find(attr => attr.n.toLowerCase().includes("power") && attr.n.toLowerCase().includes("cycle"))?.rv,
	}))
}

// S.M.A.R.T. details dialog component
function SmartDialog({ disk, smartData }: { disk: DiskInfo; smartData?: SmartData }) {
	const [open, setOpen] = React.useState(false)
	
	const smartAttributes = smartData?.a || []
	
	// Find all attributes where when failed is not empty
	const failedAttributes = smartAttributes.filter(attr => attr.wf && attr.wf.trim() !== '')
	
	const table = useReactTable({
		data: smartAttributes,
		columns: smartColumns,
		getCoreRowModel: getCoreRowModel(),
		enableSorting: false,
	})

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<DropdownMenuItem onSelect={(e) => e.preventDefault()}>
					View S.M.A.R.T.
				</DropdownMenuItem>
			</DialogTrigger>
			<DialogContent className="max-w-4xl max-h-[80vh] overflow-hidden flex flex-col">
				<DialogHeader>
					<DialogTitle>S.M.A.R.T. Details - {disk.device}</DialogTitle>
					<DialogDescription>
						S.M.A.R.T. attributes for {disk.model} ({disk.serialNumber})
					</DialogDescription>
				</DialogHeader>
				{smartData?.s && (
					<div className={`p-4 rounded-md ${
						smartData.s === "PASSED" 
							? "bg-green-100 dark:bg-green-900 border border-green-200 dark:border-green-800" 
							: "bg-red-100 dark:bg-red-900 border border-red-200 dark:border-red-800"
					}`}>
						<h4 className={`font-semibold ${
							smartData.s === "PASSED" 
								? "text-green-800 dark:text-green-200" 
								: "text-red-800 dark:text-red-200"
						}`}>
							S.M.A.R.T. Self-Test: {smartData.s}
						</h4>
						{failedAttributes.length > 0 && (
							<p className="mt-2 text-red-800 dark:text-red-200">
								Failed Attributes: {failedAttributes.map(attr => attr.n).join(", ")}
							</p>
						)}
					</div>
				)}
				<div className="flex-1 overflow-auto">
					{smartAttributes.length > 0 ? (
						<div className="rounded-md border">
							<Table>
								<TableHeader>
									{table.getHeaderGroups().map((headerGroup) => (
										<TableRow key={headerGroup.id}>
											{headerGroup.headers.map((header) => (
												<TableHead key={header.id}>
													{header.isPlaceholder
														? null
														: flexRender(
																header.column.columnDef.header,
																header.getContext()
															)}
												</TableHead>
											))}
										</TableRow>
									))}
								</TableHeader>
								<TableBody>
									{table.getRowModel().rows.map((row) => {
										// Check if the attribute is failed
										const isFailedAttribute = row.original.wf && row.original.wf.trim() !== '';
										
										return (
											<TableRow 
												key={row.id}
												className={isFailedAttribute ? "text-red-600 dark:text-red-400" : ""}
											>
												{row.getVisibleCells().map((cell) => (
													<TableCell key={cell.id}>
														{flexRender(
															cell.column.columnDef.cell,
															cell.getContext()
														)}
													</TableCell>
												))}
											</TableRow>
										);
									})}
								</TableBody>
							</Table>
						</div>
					) : (
						<div className="text-center py-8 text-muted-foreground">
							No S.M.A.R.T. attributes available for this device.
						</div>
					)}
				</div>
			</DialogContent>
		</Dialog>
	)
}

export const columns: ColumnDef<DiskInfo>[] = [
	{
		accessorKey: "device",
		header: () => (
			<div className="flex items-center">
				<HardDrive className="mr-2 h-4 w-4" />
				Device
			</div>
		),
		cell: ({ row }) => (
			<div className="font-medium">{row.getValue("device")}</div>
		),
		enableSorting: false,
	},
	{
		accessorKey: "model",
		header: () => (
			<div className="flex items-center">
				<Box className="mr-2 h-4 w-4" />
				Model
			</div>
		),
		cell: ({ row }) => (
			<div className="max-w-[200px] truncate" title={row.getValue("model")}>
				{row.getValue("model")}
			</div>
		),
		enableSorting: false,
	},
	{
		accessorKey: "capacity",
		header: () => (
			<div className="flex items-center">
				<Container className="mr-2 h-4 w-4" />
				Capacity
			</div>
		),
		cell: ({ row }) => (
			<div className="font-medium">{row.getValue("capacity")}</div>
		),
		enableSorting: false,
	},
	{
		accessorKey: "temperature",
		header: () => (
			<div className="flex items-center">
				<Thermometer className="mr-2 h-4 w-4" />
				Temp.
			</div>
		),
		cell: ({ row }) => {
			const temp = row.getValue("temperature") as number
			const getTemperatureColor = (temp: number) => {
				if (temp >= 60) return "destructive"
				if (temp >= 45) return "secondary"
				return "default"
			}
			return (
				<Badge variant={getTemperatureColor(temp)}>
					{temp}°C
				</Badge>
			)
		},
		enableSorting: false,
	},
	{
		accessorKey: "status",
		header: () => (
			<div className="flex items-center">
				<Activity className="mr-2 h-4 w-4" />
				Status
			</div>
		),
		cell: ({ row }) => {
			const status = row.getValue("status") as string
			return (
				<Badge 
					variant={status === "PASSED" ? "default" : "destructive"}
					className={status === "PASSED" ? "bg-green-500 hover:bg-green-600 text-white" : ""}
				>
					{status}
				</Badge>
			)
		},
		enableSorting: false,
	},
	{
		accessorKey: "deviceType",
		header: () => (
			<div className="flex items-center">
				<Tags className="mr-2 h-4 w-4" />
				Type
			</div>
		),
		cell: ({ row }) => (
			<Badge variant="outline" className="uppercase">
				{row.getValue("deviceType")}
			</Badge>
		),
		enableSorting: false,
	},
	{
		accessorKey: "powerOnHours",
		header: () => (
			<div className="flex items-center">
				<Clock className="mr-2 h-4 w-4" />
				Power On Time
			</div>
		),
		cell: ({ row }) => {
			const hours = row.getValue("powerOnHours") as number | undefined
			if (!hours && hours !== 0) {
				return (
					<div className="text-sm text-muted-foreground">
						N/A
					</div>
				)
			}
			const days = Math.floor(hours / 24)
			return (
				<div className="text-sm">
					<div>{hours.toLocaleString()} hours</div>
					<div className="text-muted-foreground text-xs">{days} days</div>
				</div>
			)
		},
		enableSorting: false,
	},
	{
		accessorKey: "serialNumber",
		header: () => (
			<div className="flex items-center">
				<Binary className="mr-2 h-4 w-4" />
				Serial Number
			</div>
		),
		cell: ({ row }) => (
			<div className="font-mono text-sm">{row.getValue("serialNumber")}</div>
		),
		enableSorting: false,
	},
	{
		id: "actions",
		enableHiding: false,
		cell: () => null, // This will be overwritten by columnsWithSmartData
	},
]

export default function DisksTab({ smartData }: { smartData?: Record<string, SmartData> }) {
	const [sorting, setSorting] = React.useState<SortingState>([])
	const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = React.useState<VisibilityState>({})
	const [rowSelection, setRowSelection] = React.useState({})

	// Convert SmartData to DiskInfo, if no data use empty array
	const diskData = React.useMemo(() => {
		return smartData ? convertSmartDataToDiskInfo(smartData) : []
	}, [smartData])

	// Create column definitions with SmartData
	const columnsWithSmartData = React.useMemo(() => {
		return columns.map(column => {
			if (column.id === "actions") {
				return {
					...column,
					cell: ({ row }: { row: any }) => {
						const disk = row.original as DiskInfo
						// Find the corresponding SmartData
						const diskSmartData = smartData ? Object.values(smartData).find(
							sd => sd.dn === disk.device || sd.mn === disk.model
						) : undefined

						return (
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="ghost" className="h-8 w-8 p-0">
										<span className="sr-only">Open menu</span>
										<MoreHorizontal className="h-4 w-4" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuLabel>Actions</DropdownMenuLabel>
									<SmartDialog disk={disk} smartData={diskSmartData} />
									<DropdownMenuSeparator />
									<DropdownMenuItem
										onClick={() => navigator.clipboard.writeText(disk.device)}
									>
										Copy device path
									</DropdownMenuItem>
									<DropdownMenuItem
										onClick={() => navigator.clipboard.writeText(disk.serialNumber)}
									>
										Copy serial number
									</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
						)
					}
				}
			}
			return column
		})
	}, [smartData])

	const table = useReactTable({
		data: diskData,
		columns: columnsWithSmartData,
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		getCoreRowModel: getCoreRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onColumnVisibilityChange: setColumnVisibility,
		onRowSelectionChange: setRowSelection,
		state: {
			sorting,
			columnFilters,
			columnVisibility,
			rowSelection,
		},
	})

	return (
		<div>
			<Card>
				<CardHeader>
					<CardTitle>Disk Information</CardTitle>
					<CardDescription>Disk information and S.M.A.R.T. data</CardDescription>
				</CardHeader>
				<div className="px-6 pb-6">
					<div className="w-full">
						<div className="flex items-center py-4">
							<Input
								placeholder="Filter devices..."
								value={(table.getColumn("device")?.getFilterValue() as string) ?? ""}
								onChange={(event) =>
									table.getColumn("device")?.setFilterValue(event.target.value)
								}
								className="max-w-sm"
							/>
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" className="ml-auto">
										Columns <ChevronDown className="ml-2 h-4 w-4" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									{table
										.getAllColumns()
										.filter((column) => column.getCanHide())
										.map((column) => {
											return (
												<DropdownMenuCheckboxItem
													key={column.id}
													className="capitalize"
													checked={column.getIsVisible()}
													onCheckedChange={(value) =>
														column.toggleVisibility(!!value)
													}
												>
													{column.id}
												</DropdownMenuCheckboxItem>
											)
										})}
								</DropdownMenuContent>
							</DropdownMenu>
						</div>
						<div className="rounded-md border grid">
							<Table>
								<TableHeader>
									{table.getHeaderGroups().map((headerGroup) => (
										<TableRow key={headerGroup.id}>
											{headerGroup.headers.map((header) => {
												return (
													<TableHead key={header.id}>
														{header.isPlaceholder
															? null
															: flexRender(
																	header.column.columnDef.header,
																	header.getContext()
																)}
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
											>
												{row.getVisibleCells().map((cell) => (
													<TableCell key={cell.id}>
														{flexRender(
															cell.column.columnDef.cell,
															cell.getContext()
														)}
													</TableCell>
												))}
											</TableRow>
										))
									) : (
										<TableRow>
											<TableCell
												colSpan={columns.length}
												className="h-24 text-center"
											>
												{smartData ? "No disk data available." : "Loading disk data..."}
											</TableCell>
										</TableRow>
									)}
								</TableBody>
							</Table>
						</div>
						<div className="flex items-center justify-end space-x-2 py-4">
							<div className="text-muted-foreground flex-1 text-sm">
								{table.getFilteredRowModel().rows.length} disk device(s)
							</div>
							<div className="space-x-2">
								<Button
									variant="outline"
									size="sm"
									onClick={() => table.previousPage()}
									disabled={!table.getCanPreviousPage()}
								>
									Previous
								</Button>
								<Button
									variant="outline"
									size="sm"
									onClick={() => table.nextPage()}
									disabled={!table.getCanNextPage()}
								>
									Next
								</Button>
							</div>
						</div>
					</div>
				</div>
			</Card>
		</div>
	)
} 