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
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
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
	LayoutGridIcon,
	LayoutListIcon,
	ArrowDownIcon,
	ArrowUpIcon,
	Settings2Icon,
	EyeIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { $hubVersion, $systems, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { cn, copyToClipboard, decimalString, isReadOnlyUser, useLocalStorage } from "@/lib/utils"
import AlertsButton from "../alerts/alert-button"
import { $router, Link, navigate } from "../router"
import { EthernetIcon } from "../ui/icons"
import { Trans, t } from "@lingui/macro"
import { useLingui } from "@lingui/react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { Input } from "../ui/input"
import { ClassValue } from "clsx"
import { getPagePath } from "@nanostores/router"

type ViewMode = "table" | "grid"

function CellFormatter(info: CellContext<SystemRecord, unknown>) {
	const val = info.getValue() as number
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight">
			<span className="min-w-[3.5em]">{decimalString(val, 1)}%</span>
			<span className="grow min-w-10 block bg-muted h-[1em] relative rounded-sm overflow-hidden">
				<span
					className={cn(
						"absolute inset-0 w-full h-full origin-left",
						(info.row.original.status !== "up" && "bg-primary/30") ||
							(val < 65 && "bg-green-500") ||
							(val < 90 && "bg-yellow-500") ||
							"bg-red-600"
					)}
					style={{
						transform: `scalex(${val / 100})`,
					}}
				></span>
			</span>
		</div>
	)
}

function sortableHeader(column: Column<SystemRecord, unknown>, hideSortIcon = false) {
	return (
		<Button
			variant="ghost"
			className="h-9 px-3 flex"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{/* @ts-ignore */}
			{column.columnDef?.icon && <column.columnDef.icon className="me-2 size-4" />}
			{column.id}
			{!hideSortIcon && <ArrowUpDownIcon className="ms-2 size-4" />}
		</Button>
	)
}

export default function SystemsTable() {
	const data = useStore($systems)
	const hubVersion = useStore($hubVersion)
	const [filter, setFilter] = useState<string>()
	const [sorting, setSorting] = useState<SortingState>([{ id: t`System`, desc: false }])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useLocalStorage<VisibilityState>("cols", {})
	const [viewMode, setViewMode] = useLocalStorage<ViewMode>("viewMode", window.innerWidth > 1024 ? "table" : "grid")
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
				icon: ServerIcon,
				cell: (info) => (
					<span className="flex gap-0.5 items-center text-base md:pe-5">
						<IndicatorDot system={info.row.original} />
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
				),
				header: ({ column }) => sortableHeader(column),
			},
			{
				accessorKey: "info.cpu",
				id: t`CPU`,
				invertSorting: true,
				cell: CellFormatter,
				icon: CpuIcon,
				header: ({ column }) => sortableHeader(column),
			},
			{
				accessorKey: "info.mp",
				id: t`Memory`,
				invertSorting: true,
				cell: CellFormatter,
				icon: MemoryStickIcon,
				header: ({ column }) => sortableHeader(column),
			},
			{
				accessorKey: "info.dp",
				id: t`Disk`,
				invertSorting: true,
				cell: CellFormatter,
				icon: HardDriveIcon,
				header: ({ column }) => sortableHeader(column),
			},
			{
				accessorFn: (originalRow) => originalRow.info.b || 0,
				id: t`Net`,
				invertSorting: true,
				size: 115,
				icon: EthernetIcon,
				header: ({ column }) => sortableHeader(column),
				cell(info) {
					const val = info.getValue() as number
					return (
						<span
							className={cn("tabular-nums whitespace-nowrap", {
								"ps-1": viewMode === "table",
							})}
						>
							{decimalString(val, val >= 100 ? 1 : 2)} MB/s
						</span>
					)
				},
			},
			{
				accessorKey: "info.v",
				id: t`Agent`,
				invertSorting: true,
				size: 50,
				icon: WifiIcon,
				header: ({ column }) => sortableHeader(column, true),
				cell(info) {
					const version = info.getValue() as string
					if (!version || !hubVersion) {
						return null
					}
					const system = info.row.original
					return (
						<span
							className={cn("flex gap-2 items-center md:pe-5 tabular-nums", {
								"ps-1": viewMode === "table",
							})}
						>
							<IndicatorDot
								system={system}
								className={
									(system.status !== "up" && "bg-primary/30") ||
									(version === hubVersion && "bg-green-500") ||
									"bg-yellow-500"
								}
							/>
							<span>{info.getValue() as string}</span>
						</span>
					)
				},
			},
			{
				id: t({ message: "Actions", comment: "Table column" }),
				size: 120,
				cell: ({ row }) => (
					<div className="flex justify-end items-center gap-1">
						<AlertsButton system={row.original} />
						<ActionsButton system={row.original} />
					</div>
				),
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
									<Settings2Icon className="me-1.5 size-4 opacity-80" />
									<Trans>View</Trans>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
								<div className="grid grid-cols-1 md:grid-cols-3 divide-y md:divide-s md:divide-y-0">
									<div>
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<LayoutGridIcon className="size-4" />
											<Trans>Layout</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<DropdownMenuRadioGroup
											className="px-1 pb-1"
											value={viewMode}
											onValueChange={(view) => setViewMode(view as ViewMode)}
										>
											<DropdownMenuRadioItem value="table" onSelect={(e) => e.preventDefault()} className="gap-2">
												<LayoutListIcon className="size-4" />
												<Trans>Table</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="grid" onSelect={(e) => e.preventDefault()} className="gap-2">
												<LayoutGridIcon className="size-4" />
												<Trans>Grid</Trans>
											</DropdownMenuRadioItem>
										</DropdownMenuRadioGroup>
									</div>

									<div>
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<ArrowUpDownIcon className="size-4" />
											<Trans>Sort By</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<div className="px-1 pb-1">
											{table.getAllColumns().map((column) => {
												if (column.id === t`Actions` || !column.getCanSort()) return null
												let Icon = <span className="w-6"></span>
												// if current sort column, show sort direction
												if (sorting[0]?.id === column.id) {
													if (sorting[0]?.desc) {
														Icon = <ArrowUpIcon className="me-2 size-4" />
													} else {
														Icon = <ArrowDownIcon className="me-2 size-4" />
													}
												}
												return (
													<DropdownMenuItem
														onSelect={(e) => {
															e.preventDefault()
															setSorting([{ id: column.id, desc: sorting[0]?.id === column.id && !sorting[0]?.desc }])
														}}
														key={column.id}
													>
														{Icon}
														{column.id}
													</DropdownMenuItem>
												)
											})}
										</div>
									</div>

									<div>
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<EyeIcon className="size-4" />
											<Trans>Visible Fields</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<div className="px-1.5 pb-1">
											{table
												.getAllColumns()
												.filter((column) => column.getCanHide())
												.map((column) => {
													return (
														<DropdownMenuCheckboxItem
															key={column.id}
															onSelect={(e) => e.preventDefault()}
															checked={column.getIsVisible()}
															onCheckedChange={(value) => column.toggleVisibility(!!value)}
														>
															{column.id}
														</DropdownMenuCheckboxItem>
													)
												})}
										</div>
									</div>
								</div>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</CardHeader>
			<div className="p-6 pt-0 max-sm:py-3 max-sm:px-2">
				{viewMode === "table" ? (
					// table layout
					<div className="rounded-md border overflow-hidden">
						<Table>
							<TableHeader>
								{table.getHeaderGroups().map((headerGroup) => (
									<TableRow key={headerGroup.id}>
										{headerGroup.headers.map((header) => {
											return (
												<TableHead className="px-2" key={header.id}>
													{header.isPlaceholder
														? null
														: flexRender(header.column.columnDef.header, header.getContext())}
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
													navigate(getPagePath($router, "system", { name: row.original.name }))
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
				) : (
					// grid layout
					<div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
						{table.getRowModel().rows?.length ? (
							table.getRowModel().rows.map((row) => {
								const system = row.original
								const { status } = system
								return (
									<Card
										key={system.id}
										className={cn(
											"cursor-pointer hover:shadow-md transition-all bg-transparent w-full dark:border-border duration-200 relative",
											{
												"opacity-50": status === "paused",
											}
										)}
									>
										<CardHeader className="py-1 ps-5 pe-3 bg-muted/30 border-b border-border/60">
											<div className="flex items-center justify-between gap-2">
												<CardTitle className="text-base tracking-normal shrink-1 text-primary/90 flex items-center min-h-10 gap-2.5 min-w-0">
													<div className="flex items-center gap-2.5 min-w-0">
														<IndicatorDot system={system} />
														<CardTitle className="text-[.95em]/normal tracking-normal truncate text-primary/90">
															{system.name}
														</CardTitle>
													</div>
												</CardTitle>
												{table.getColumn(t`Actions`)?.getIsVisible() && (
													<div className="flex gap-1 flex-shrink-0 relative z-10">
														<AlertsButton system={system} />
														<ActionsButton system={system} />
													</div>
												)}
											</div>
										</CardHeader>
										<CardContent className="space-y-2.5 text-sm px-5 pt-3.5 pb-4">
											{table.getAllColumns().map((column) => {
												if (!column.getIsVisible() || column.id === t`System` || column.id === t`Actions`) return null
												const cell = row.getAllCells().find((cell) => cell.column.id === column.id)
												if (!cell) return null
												return (
													<div key={column.id} className="flex items-center gap-3">
														{/* @ts-ignore */}
														{column.columnDef?.icon && (
															// @ts-ignore
															<column.columnDef.icon className="size-4 text-muted-foreground" />
														)}
														<div className="flex items-center gap-3 flex-1">
															<span className="text-muted-foreground min-w-16">{column.id}:</span>
															<div className="flex-1">{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>
														</div>
													</div>
												)
											})}
										</CardContent>
										<Link
											href={getPagePath($router, "system", { name: row.original.name })}
											className="inset-0 absolute w-full h-full"
										>
											<span className="sr-only">{row.original.name}</span>
										</Link>
									</Card>
								)
							})
						) : (
							<div className="col-span-full text-center py-8">
								<Trans>No systems found.</Trans>
							</div>
						)}
					</div>
				)}
			</div>
		</Card>
	)
}

function IndicatorDot({ system, className }: { system: SystemRecord; className?: ClassValue }) {
	className ||= {
		"bg-green-500": system.status === "up",
		"bg-red-500": system.status === "down",
		"bg-primary/40": system.status === "paused",
		"bg-yellow-500": system.status === "pending",
	}
	return (
		<span
			className={cn("flex-shrink-0 size-2 rounded-full", className)}
			// style={{ marginBottom: "-1px" }}
		/>
	)
}

function ActionsButton({ system }: { system: SystemRecord }) {
	// const [opened, setOpened] = useState(false)
	const { id, status, host, name } = system
	return (
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
								<PlayCircleIcon className="me-2.5 size-4" />
								<Trans>Resume</Trans>
							</>
						) : (
							<>
								<PauseCircleIcon className="me-2.5 size-4" />
								<Trans>Pause</Trans>
							</>
						)}
					</DropdownMenuItem>
					<DropdownMenuItem onClick={() => copyToClipboard(host)}>
						<CopyIcon className="me-2.5 size-4" />
						<Trans>Copy host</Trans>
					</DropdownMenuItem>
					<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
					<AlertDialogTrigger asChild>
						<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")}>
							<Trash2Icon className="me-2.5 size-4" />
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
							This action cannot be undone. This will permanently delete all current records for {name} from the
							database.
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
	)
}
