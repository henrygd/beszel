import {
	ColumnDef,
	ColumnFiltersState,
	getFilteredRowModel,
	SortingState,
	getSortedRowModel,
	flexRender,
	VisibilityState,
	getCoreRowModel,
	useReactTable,
	Row,
	Table as TableType,
} from "@tanstack/react-table"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Button } from "@/components/ui/button"
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
import { SystemRecord } from "@/types"
import {
	ArrowUpDownIcon,
	LayoutGridIcon,
	LayoutListIcon,
	ArrowDownIcon,
	ArrowUpIcon,
	Settings2Icon,
	EyeIcon,
	FilterIcon,
} from "lucide-react"
import { memo, useEffect, useMemo, useState } from "react"
import { $systems } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { cn, runOnce, useLocalStorage } from "@/lib/utils"
import { $router, Link } from "../router"
import { useLingui, Trans } from "@lingui/react/macro"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { Input } from "@/components/ui/input"
import { getPagePath } from "@nanostores/router"
import SystemsTableColumns, { ActionsButton, IndicatorDot } from "./systems-table-columns"
import AlertButton from "../alerts/alert-button"
import { SystemStatus } from "@/lib/enums"

type ViewMode = "table" | "grid"
type StatusFilter = "all" | "up" | "down" | "paused"

const preloadSystemDetail = runOnce(() => import("@/components/routes/system.tsx"))

export default function SystemsTable() {
	const data = useStore($systems)
	const { i18n, t } = useLingui()
	const [filter, setFilter] = useState<string>()
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
	const [sorting, setSorting] = useLocalStorage<SortingState>("sortMode",[{ id: "system", desc: false }])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useLocalStorage<VisibilityState>("cols", {})
	const [viewMode, setViewMode] = useLocalStorage<ViewMode>("viewMode", window.innerWidth > 1024 ? "table" : "grid")

	const locale = i18n.locale

	// Filter data based on status filter
	const filteredData = useMemo(() => {
		if (statusFilter === "all") {
			return data
		}
		return data.filter((system) => system.status === statusFilter)
	}, [data, statusFilter])

	const runningRecords = useMemo(() => {
		return data.filter((record) => record.status === "up").length
	}, [data])

	const totalRecords = useMemo(() => {
		return data.length
	}, [data])

	useEffect(() => {
		if (filter !== undefined) {
			table.getColumn("system")?.setFilterValue(filter)
		}
	}, [filter])

	const columnDefs = useMemo(() => SystemsTableColumns(viewMode), [viewMode])

	const table = useReactTable({
		data: filteredData,
		columns: columnDefs,
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
			// sortDescFirst: true,
			invertSorting: true,
			sortUndefined: "last",
			minSize: 0,
			size: 900,
			maxSize: 900,
		},
	})

	const rows = table.getRowModel().rows
	const columns = table.getAllColumns()
	const visibleColumns = table.getVisibleLeafColumns()
	// TODO: hiding temp then gpu messes up table headers
	const CardHead = useMemo(() => {
		return (
			<CardHeader className="pb-5 px-2 sm:px-6 max-sm:pt-5 max-sm:pb-1">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2.5 flex">
							<Trans>All Systems - {runningRecords} / {totalRecords}</Trans>
							<p className={runningRecords === totalRecords ? "ml-2 text-emerald-600":"ml-2 text-red-600" }>Online</p>
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
								<div className="grid grid-cols-1 md:grid-cols-4 divide-y md:divide-s md:divide-y-0">
									<div className="border-r">
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

									<div className="border-r">
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<FilterIcon className="size-4" />
											<Trans>Status</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<DropdownMenuRadioGroup
											className="px-1 pb-1"
											value={statusFilter}
											onValueChange={(value) => setStatusFilter(value as StatusFilter)}
										>
											<DropdownMenuRadioItem value="all" onSelect={(e) => e.preventDefault()}>
												<Trans>All Systems</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="up" onSelect={(e) => e.preventDefault()}>
												<Trans>Up</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="down" onSelect={(e) => e.preventDefault()}>
												<Trans>Down</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="paused" onSelect={(e) => e.preventDefault()}>
												<Trans>Paused</Trans>
											</DropdownMenuRadioItem>
										</DropdownMenuRadioGroup>
									</div>

									<div className="border-r">
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<ArrowUpDownIcon className="size-4" />
											<Trans>Sort By</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<div className="px-1 pb-1">
											{columns.map((column) => {
												if (!column.getCanSort()) return null
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
														{/* @ts-ignore */}
														{column.columnDef.name()}
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
											{columns
												.filter((column) => column.getCanHide())
												.map((column) => {
													return (
														<DropdownMenuCheckboxItem
															key={column.id}
															onSelect={(e) => e.preventDefault()}
															checked={column.getIsVisible()}
															onCheckedChange={(value) => column.toggleVisibility(!!value)}
														>
															{/* @ts-ignore */}
															{column.columnDef.name()}
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
		)
	}, [visibleColumns.length, sorting, viewMode, locale, statusFilter, totalRecords])

	return (
		<Card>
			{CardHead}
			<div className="p-6 pt-0 max-sm:py-3 max-sm:px-2">
				{viewMode === "table" ? (
					// table layout
					<div className="rounded-md border overflow-hidden">
						<AllSystemsTable table={table} rows={rows} colLength={visibleColumns.length} />
					</div>
				) : (
					// grid layout
					<div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
						{rows?.length ? (
							rows.map((row) => {
								return <SystemCard key={row.original.id} row={row} table={table} colLength={visibleColumns.length} />
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

const AllSystemsTable = memo(
	({ table, rows, colLength }: { table: TableType<SystemRecord>; rows: Row<SystemRecord>[]; colLength: number }) => {
		return (
			<Table>
				<SystemsTableHead table={table} colLength={colLength} />
				<TableBody onMouseEnter={preloadSystemDetail}>
					{rows.length ? (
						rows.map((row) => (
							<SystemTableRow key={row.original.id} row={row} length={rows.length} colLength={colLength} />
						))
					) : (
						<TableRow>
							<TableCell colSpan={colLength} className="h-24 text-center">
								<Trans>No systems found.</Trans>
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
		)
	}
)

function SystemsTableHead({ table, colLength }: { table: TableType<SystemRecord>; colLength: number }) {
	const { i18n } = useLingui()
	return useMemo(() => {
		return (
			<TableHeader>
				{table.getHeaderGroups().map((headerGroup) => (
					<TableRow key={headerGroup.id}>
						{headerGroup.headers.map((header) => {
							return (
								<TableHead className="px-1.5" key={header.id}>
									{flexRender(header.column.columnDef.header, header.getContext())}
								</TableHead>
							)
						})}
					</TableRow>
				))}
			</TableHeader>
		)
	}, [i18n.locale, colLength])
}

const SystemTableRow = memo(
	({ row, length, colLength }: { row: Row<SystemRecord>; length: number; colLength: number }) => {
		const system = row.original
		const { t } = useLingui()
		return useMemo(() => {
			return (
				<TableRow
					// data-state={row.getIsSelected() && "selected"}
					className={cn("cursor-pointer transition-opacity relative safari:transform-3d", {
						"opacity-50": system.status === SystemStatus.Paused,
					})}
				>
					{row.getVisibleCells().map((cell) => (
						<TableCell
							key={cell.id}
							style={{
								width: cell.column.getSize(),
							}}
							className={length > 10 ? "py-2" : "py-2.5"}
						>
							{flexRender(cell.column.columnDef.cell, cell.getContext())}
						</TableCell>
					))}
				</TableRow>
			)
		}, [system, system.status, colLength, t])
	}
)

const SystemCard = memo(
	({ row, table, colLength }: { row: Row<SystemRecord>; table: TableType<SystemRecord>; colLength: number }) => {
		const system = row.original
		const { t } = useLingui()

		return useMemo(() => {
			return (
				<Card
					onMouseEnter={preloadSystemDetail}
					key={system.id}
					className={cn(
						"cursor-pointer hover:shadow-md transition-all bg-transparent w-full dark:border-border duration-200 relative",
						{
							"opacity-50": system.status === SystemStatus.Paused,
						}
					)}
				>
					<CardHeader className="py-1 ps-5 pe-3 bg-muted/30 border-b border-border/60">
						<div className="flex items-center justify-between gap-2">
							<CardTitle className="text-base tracking-normal shrink-1 text-primary/90 flex items-center min-w-0 gap-2.5">
								<div className="flex items-center gap-2.5 min-w-0">
									<IndicatorDot system={system} />
									<CardTitle className="text-[.95em]/normal tracking-normal truncate text-primary/90">
										{system.name}
									</CardTitle>
								</div>
							</CardTitle>
							{table.getColumn("actions")?.getIsVisible() && (
								<div className="flex gap-1 shrink-0 relative z-10">
									<AlertButton system={system} />
									<ActionsButton system={system} />
								</div>
							)}
						</div>
					</CardHeader>
					<CardContent className="grid gap-2.5 text-sm px-5 pt-3.5 pb-4">
						{table.getAllColumns().map((column) => {
							if (!column.getIsVisible() || column.id === "system" || column.id === "actions") return null
							const cell = row.getAllCells().find((cell) => cell.column.id === column.id)
							if (!cell) return null
							// @ts-ignore
							const { Icon, name } = column.columnDef as ColumnDef<SystemRecord, unknown>
							return (
								<div key={column.id} className="flex items-center gap-3">
									{column.id === "lastSeen" ? (
										<EyeIcon className="size-4 text-muted-foreground" />
									) : (
										Icon && <Icon className="size-4 text-muted-foreground" />
									)}
									<div className="flex items-center gap-3 flex-1">
										<span className="text-muted-foreground min-w-16">{name()}:</span>
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
		}, [system, colLength, t])
	}
)
