import {
	CellContext,
	ColumnDef,
	ColumnFiltersState,
	getFilteredRowModel,
	SortingState,
	getSortedRowModel,
	flexRender,
	getCoreRowModel,
	useReactTable,
	Column,
} from "@tanstack/react-table"

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

import { Button, buttonVariants } from "@/components/ui/button"

import {
	DropdownMenu,
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
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { $hubVersion, $systems, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { cn, copyToClipboard, decimalString, isReadOnlyUser } from "@/lib/utils"
import AlertsButton from "../alerts/alert-button"
import { navigate } from "../router"
import { EthernetIcon } from "../ui/icons"
import { useTranslation } from "react-i18next"

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

function sortableHeader(column: Column<SystemRecord, unknown>, name: string, Icon: any, hideSortIcon = false) {
	return (
		<Button
			variant="ghost"
			className="h-9 px-3 rtl:ml-auto flex"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			<Icon className="me-2 h-4 w-4" />
			{name}
			{!hideSortIcon && <ArrowUpDownIcon className="ml-2 h-4 w-4" />}
		</Button>
	)
}

export default function SystemsTable({ filter }: { filter?: string }) {
	const { t } = useTranslation()

	const data = useStore($systems)
	const hubVersion = useStore($hubVersion)
	const [sorting, setSorting] = useState<SortingState>([])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])

	useEffect(() => {
		if (filter !== undefined) {
			table.getColumn("name")?.setFilterValue(filter)
		}
	}, [filter])

	const columns = useMemo(() => {
		return [
			{
				// size: 200,
				size: 200,
				minSize: 0,
				accessorKey: "name",
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
				header: ({ column }) => sortableHeader(column, t("systems_table.system"), ServerIcon),
			},
			{
				accessorKey: "info.cpu",
				invertSorting: true,
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, t("systems_table.cpu"), CpuIcon),
			},
			{
				accessorKey: "info.mp",
				invertSorting: true,
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, t("systems_table.memory"), MemoryStickIcon),
			},
			{
				accessorKey: "info.dp",
				invertSorting: true,
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, t("systems_table.disk"), HardDriveIcon),
			},
			{
				accessorFn: (originalRow) => originalRow.info.b || 0,
				id: "n",
				invertSorting: true,
				size: 115,
				header: ({ column }) => sortableHeader(column, t("systems_table.net"), EthernetIcon),
				cell: (info) => {
					const val = info.getValue() as number
					return (
						<span className="tabular-nums whitespace-nowrap pl-1">{decimalString(val, val >= 100 ? 1 : 2)} MB/s</span>
					)
				},
			},
			{
				accessorKey: "info.v",
				invertSorting: true,
				size: 50,
				header: ({ column }) => sortableHeader(column, t("systems_table.agent"), WifiIcon, true),
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
				id: "actions",
				size: 120,
				// minSize: 0,
				cell: ({ row }) => {
					const { id, name, status, host } = row.original
					return (
						<div className={"flex justify-end items-center gap-1"}>
							<AlertsButton system={row.original} />
							<AlertDialog>
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button variant="ghost" size={"icon"} data-nolink>
											<span className="sr-only">{t("systems_table.open_menu")}</span>
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
													<PlayCircleIcon className="mr-2.5 h-4 w-4" />
													{t("systems_table.resume")}
												</>
											) : (
												<>
													<PauseCircleIcon className="mr-2.5 h-4 w-4" />
													{t("systems_table.pause")}
												</>
											)}
										</DropdownMenuItem>
										<DropdownMenuItem onClick={() => copyToClipboard(host)}>
											<CopyIcon className="mr-2.5 h-4 w-4" />
											{t("systems_table.copy_host")}
										</DropdownMenuItem>
										<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
										<AlertDialogTrigger asChild>
											<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")}>
												<Trash2Icon className="mr-2.5 h-4 w-4" />
												{t("systems_table.delete")}
											</DropdownMenuItem>
										</AlertDialogTrigger>
									</DropdownMenuContent>
								</DropdownMenu>
								<AlertDialogContent>
									<AlertDialogHeader>
										<AlertDialogTitle>{t("systems_table.delete_confirm", { name })}</AlertDialogTitle>
										<AlertDialogDescription>
											{t("systems_table.delete_confirm_des_1")} <code className="bg-muted rounded-sm px-1">{name}</code>{" "}
											{t("systems_table.delete_confirm_des_2")}
										</AlertDialogDescription>
									</AlertDialogHeader>
									<AlertDialogFooter>
										<AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
										<AlertDialogAction
											className={cn(buttonVariants({ variant: "destructive" }))}
											onClick={() => pb.collection("systems").delete(id)}
										>
											{t("continue")}
										</AlertDialogAction>
									</AlertDialogFooter>
								</AlertDialogContent>
							</AlertDialog>
						</div>
					)
				},
			},
		] as ColumnDef<SystemRecord>[]
	}, [hubVersion])

	const table = useReactTable({
		data,
		columns,
		getCoreRowModel: getCoreRowModel(),
		onSortingChange: setSorting,
		getSortedRowModel: getSortedRowModel(),
		onColumnFiltersChange: setColumnFilters,
		getFilteredRowModel: getFilteredRowModel(),
		state: {
			sorting,
			columnFilters,
		},
		defaultColumn: {
			minSize: 0,
			size: Number.MAX_SAFE_INTEGER,
			maxSize: Number.MAX_SAFE_INTEGER,
		},
	})

	return (
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
								{t("systems_table.no_systems_found")}
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
		</div>
	)
}
