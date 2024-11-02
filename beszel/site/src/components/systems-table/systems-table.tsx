import {
	CellContext,
	ColumnDef,
	ColumnFiltersState,
	getFilteredRowModel,
	SortingState,
	getSortedRowModel,
	flexRender,
	VisibilityState,
	getCoreRowModel,
	useReactTable,
	Column,
} from "@tanstack/react-table"

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

import { Button, buttonVariants } from "@/components/ui/button"

import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
	AlertDialogTrigger,
} from "@/components/ui/alert-dialog"

import { SystemRecord } from "@/types"
import {
	MoreHorizontalIcon,
	ArrowUpDownIcon,
	MemoryStickIcon,
	CopyIcon,
	PauseCircleIcon,
	PlayCircleIcon,
	Trash2Icon,
	WifiIcon,
	HardDriveIcon,
	ServerIcon,
	CpuIcon,
	ChevronDownIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { $hubVersion, $systems, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { cn, copyToClipboard, decimalString, isReadOnlyUser, useLocalStorage } from "@/lib/utils"
import AlertsButton from "../alerts/alert-button"
import { navigate } from "../router"
import { EthernetIcon } from "../ui/icons"
import { Trans, t } from "@lingui/macro"
import { useLingui } from "@lingui/react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { Input } from "../ui/input"

function CellFormatter(info: CellContext<SystemRecord, unknown>) {
	const val = info.getValue() as number
	return (
		<div className="flex gap-1 items-center tabular-nums tracking-tight">
			<span className="min-w-[3.5em]">{decimalString(val, 1)}%</span>
			<span className="grow min-w-10 block bg-muted h-[1em] relative rounded-sm overflow-hidden">
				<span
					className={cn(
						"absolute inset-0 w-full h-full origin-left",
						(val < 65 && "bg-green-500") || (val < 90 && "bg-yellow-500") || "bg-red-600"
					)}
					style={{ transform: `scalex(${val}%)` }}
				></span>
			</span>
		</div>
	)
}

function sortableHeader(column: Column<SystemRecord, unknown>, Icon: any, hideSortIcon = false) {
	return (
		<Button
			variant="ghost"
			className="h-9 px-3 flex"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			<Icon className="me-2 h-4 w-4" />
			{column.id}
			{!hideSortIcon && <ArrowUpDownIcon className="ms-2 h-4 w-4" />}
		</Button>
	)
}

export default function SystemsTable() {
	const data = useStore($systems)
	const hubVersion = useStore($hubVersion)
	const [filter, setFilter] = useState<string>()
	const [sorting, setSorting] = useState<SortingState>([])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useLocalStorage<VisibilityState>("cols", {})
	const { i18n } = useLingui()

	useEffect(() => {
		if (filter !== undefined) {
			table.getColumn(t`System`)?.setFilterValue(filter)
		}
	}, [filter])

	const columns = useMemo(() => {
		return [
			{
				// size: 200,
				size: 200,
				minSize: 0,
				accessorKey: "name",
				id: t`System`,
				enableHiding: false,
				cell: (info) => {
					const { status } = info.row.original
					return (
						<span className="flex gap-0.5 items-center text-base md:pe-5">
							<span
								className={cn("w-2 h-2 left-0 rounded-full", {
									"bg-green-500": status === "up",
									"bg-red-500": status === "down",
									"bg-primary/40": status === "paused",
									"bg-yellow-500": status === "pending",
								})}
								style={{ marginBottom: "-1px" }}
							></span>
							<Button
								data-nolink
								variant={"ghost"}
								className="text-primary/90 h-7 px-1.5 gap-1.5"
								onClick={() => copyToClipboard(info.getValue() as string)}
							>
								{info.getValue() as string}
								<CopyIcon className="h-2.5 w-2.5" />
							</Button>
						</span>
					)
				},
				header: ({ column }) => sortableHeader(column, ServerIcon),
			},
			{
				accessorKey: "info.cpu",
				id: t`CPU`,
				invertSorting: true,
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, CpuIcon),
			},
			{
				accessorKey: "info.mp",
				id: t`Memory`,
				invertSorting: true,
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, MemoryStickIcon),
			},
			{
				accessorKey: "info.dp",
				id: t`Disk`,
				invertSorting: true,
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, HardDriveIcon),
			},
			{
				accessorFn: (originalRow) => originalRow.info.b || 0,
				id: t`Net`,
				invertSorting: true,
				size: 115,
				header: ({ column }) => sortableHeader(column, EthernetIcon),
				cell: (info) => {
					const val = info.getValue() as number
					return (
						<span className="tabular-nums whitespace-nowrap ps-1">{decimalString(val, val >= 100 ? 1 : 2)} MB/s</span>
					)
				},
			},
			{
				accessorKey: "info.v",
				id: t`Agent`,
				invertSorting: true,
				size: 50,
				header: ({ column }) => sortableHeader(column, WifiIcon, true),
				cell: (info) => {
					const version = info.getValue() as string
					if (!version || !hubVersion) {
						return null
					}
					return (
						<span className="flex gap-2 items-center md:pe-5 tabular-nums ps-1">
							<span
								className={cn("w-2 h-2 left-0 rounded-full", version === hubVersion ? "bg-green-500" : "bg-yellow-500")}
								style={{ marginBottom: "-1px" }}
							></span>
							<span>{info.getValue() as string}</span>
						</span>
					)
				},
			},
			{
				id: t({ message: "Actions", comment: "Table column" }),
				size: 120,
				cell: ({ row }) => {
					const { id, name, status, host } = row.original
					return (
						<div className={"flex justify-end items-center gap-1"}>
							<AlertsButton system={row.original} />
							<AlertDialog>
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button variant="ghost" size={"icon"} data-nolink>
											<span className="sr-only">
												<Trans>Open menu</Trans>
											</span>
											<MoreHorizontalIcon className="w-5" />
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent align="end">
										<DropdownMenuItem
											className={cn(isReadOnlyUser() && "hidden")}
											onClick={() => {
												pb.collection("systems").update(id, {
													status: status === "paused" ? "pending" : "paused",
												})
											}}
										>
											{status === "paused" ? (
												<>
													<PlayCircleIcon className="me-2.5 h-4 w-4" />
													<Trans>Resume</Trans>
												</>
											) : (
												<>
													<PauseCircleIcon className="me-2.5 h-4 w-4" />
													<Trans>Pause</Trans>
												</>
											)}
										</DropdownMenuItem>
										<DropdownMenuItem onClick={() => copyToClipboard(host)}>
											<CopyIcon className="me-2.5 h-4 w-4" />
											<Trans>Copy host</Trans>
										</DropdownMenuItem>
										<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
										<AlertDialogTrigger asChild>
											<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")}>
												<Trash2Icon className="me-2.5 h-4 w-4" />
												<Trans>Delete</Trans>
											</DropdownMenuItem>
										</AlertDialogTrigger>
									</DropdownMenuContent>
								</DropdownMenu>
								<AlertDialogContent>
									<AlertDialogHeader>
										<AlertDialogTitle>
											<Trans>Are you sure you want to delete {name}?</Trans>
										</AlertDialogTitle>
										<AlertDialogDescription>
											<Trans>
												This action cannot be undone. This will permanently delete all current records for {name} from
												the database.
											</Trans>
										</AlertDialogDescription>
									</AlertDialogHeader>
									<AlertDialogFooter>
										<AlertDialogCancel>
											<Trans>Cancel</Trans>
										</AlertDialogCancel>
										<AlertDialogAction
											className={cn(buttonVariants({ variant: "destructive" }))}
											onClick={() => pb.collection("systems").delete(id)}
										>
											<Trans>Continue</Trans>
										</AlertDialogAction>
									</AlertDialogFooter>
								</AlertDialogContent>
							</AlertDialog>
						</div>
					)
				},
			},
		] as ColumnDef<SystemRecord>[]
	}, [hubVersion, i18n.locale])

	const table = useReactTable({
		data,
		columns,
		getCoreRowModel: getCoreRowModel(),
		onSortingChange: setSorting,
		getSortedRowModel: getSortedRowModel(),
		onColumnFiltersChange: setColumnFilters,
		getFilteredRowModel: getFilteredRowModel(),
		onColumnVisibilityChange: setColumnVisibility,
		state: {
			sorting,
			columnFilters,
			columnVisibility,
		},
		defaultColumn: {
			minSize: 0,
			size: Number.MAX_SAFE_INTEGER,
			maxSize: Number.MAX_SAFE_INTEGER,
		},
	})

	return (
		<Card>
			<CardHeader className="pb-5 px-2 sm:px-6 max-sm:pt-5 max-sm:pb-1">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2.5">
							<Trans>All Systems</Trans>
						</CardTitle>
						<CardDescription>
							<Trans>Updated in real time. Click on a system to view information.</Trans>
						</CardDescription>
					</div>
					<div className="flex gap-2 ms-auto w-full md:w-80">
						<Input placeholder={t`Filter...`} onChange={(e) => setFilter(e.target.value)} className="px-4" />
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline">
									<Trans comment="Context: table columns">Columns</Trans>{" "}
									<ChevronDownIcon className="ms-1.5 h-4 w-4 opacity-90" />
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
												checked={column.getIsVisible()}
												onCheckedChange={(value) => column.toggleVisibility(!!value)}
											>
												{column.id}
											</DropdownMenuCheckboxItem>
										)
									})}
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</CardHeader>
			<CardContent className="max-sm:p-2">
				<div className="rounded-md border overflow-hidden">
					<Table>
						<TableHeader className="bg-muted/40">
							{table.getHeaderGroups().map((headerGroup) => (
								<TableRow key={headerGroup.id}>
									{headerGroup.headers.map((header) => {
										return (
											<TableHead className="px-2" key={header.id}>
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
										key={row.original.id}
										data-state={row.getIsSelected() && "selected"}
										className={cn("cursor-pointer transition-opacity", {
											"opacity-50": row.original.status === "paused",
										})}
										onClick={(e) => {
											const target = e.target as HTMLElement
											if (!target.closest("[data-nolink]") && e.currentTarget.contains(target)) {
												navigate(`/system/${encodeURIComponent(row.original.name)}`)
											}
										}}
									>
										{row.getVisibleCells().map((cell) => (
											<TableCell
												key={cell.id}
												style={{
													width: cell.column.getSize() === Number.MAX_SAFE_INTEGER ? "auto" : cell.column.getSize(),
												}}
												className={cn("overflow-hidden relative", data.length > 10 ? "py-2" : "py-2.5")}
											>
												{flexRender(cell.column.columnDef.cell, cell.getContext())}
											</TableCell>
										))}
									</TableRow>
								))
							) : (
								<TableRow>
									<TableCell colSpan={columns.length} className="h-24 text-center">
										<Trans>No systems found.</Trans>
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</div>
			</CardContent>
		</Card>
	)
}
