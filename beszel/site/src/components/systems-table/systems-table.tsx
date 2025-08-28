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
	PenBoxIcon,
	ClockIcon,
	FilterIcon,
	HourglassIcon,
} from "lucide-react"
import { memo, useEffect, useMemo, useState } from "react"
import { $systems } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import {
	cn,
	copyToClipboard,
	isReadOnlyUser,
	useLocalStorage,
	formatTemperature,
	decimalString,
	formatBytes,
	formatUptimeString,
} from "@/lib/utils"
import AlertsButton from "../alerts/alert-button"
import { $router, Link, navigate } from "../router"
import { EthernetIcon, GpuIcon, ThermometerIcon } from "../ui/icons"
import { useLingui, Trans, Plural } from "@lingui/react/macro"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { Input } from "@/components/ui/input"
import { ClassValue } from "clsx"
import { cn, useLocalStorage } from "@/lib/utils"
import { $router, Link } from "../router"
import { useLingui, Trans } from "@lingui/react/macro"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { Input } from "../ui/input"
import { getPagePath } from "@nanostores/router"
import SystemsTableColumns, { ActionsButton, IndicatorDot } from "./systems-table-columns"
import AlertButton from "../alerts/alert-button"
import { SystemStatus } from "@/lib/enums"

type ViewMode = "table" | "grid"
type StatusFilter = "all" | "up" | "down" | "paused"

export default function SystemsTable() {
	const data = useStore($systems)
	const { i18n, t } = useLingui()
	const [filter, setFilter] = useState<string>()
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
	const [sorting, setSorting] = useState<SortingState>([{ id: "system", desc: false }])
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

	useEffect(() => {
		if (filter !== undefined) {
			table.getColumn("system")?.setFilterValue(filter)
		}
	}, [filter])

	const columnDefs = useMemo(() => {
		const statusTranslations = {
			up: () => t`Up`.toLowerCase(),
			down: () => t`Down`.toLowerCase(),
			paused: () => t`Paused`.toLowerCase(),
		}
		return [
			{
				size: 200,
				minSize: 0,
				accessorKey: "name",
				id: "system",
				name: () => t`System`,
				filterFn: (row, _, filterVal) => {
					const filterLower = filterVal.toLowerCase()
					const { name, status } = row.original
					// Check if the filter matches the name or status for this row
					if (
						name.toLowerCase().includes(filterLower) ||
						statusTranslations[status as keyof typeof statusTranslations]?.().includes(filterLower)
					) {
						return true
					}
					return false
				},
				enableHiding: false,
				invertSorting: false,
				Icon: ServerIcon,
				cell: (info) => (
					<span className="flex gap-0.5 items-center text-base md:ps-1 md:pe-5">
						<IndicatorDot system={info.row.original} />
						<Button
							data-nolink
							variant={"ghost"}
							className="text-primary/90 h-7 px-1.5 gap-1.5"
							onClick={() => copyToClipboard(info.getValue() as string)}
						>
							{info.getValue() as string}
							<CopyIcon className="size-2.5" />
						</Button>
					</span>
				),
				header: sortableHeader,
			},
			{
				accessorFn: ({ info }) => decimalString(info.cpu, info.cpu >= 10 ? 1 : 2),
				id: "cpu",
				name: () => t`CPU`,
				cell: CellFormatter,
				Icon: CpuIcon,
				header: sortableHeader,
			},
			{
				// accessorKey: "info.mp",
				accessorFn: (originalRow) => originalRow.info.mp,
				id: "memory",
				name: () => t`Memory`,
				cell: CellFormatter,
				Icon: MemoryStickIcon,
				header: sortableHeader,
			},
			{
				accessorFn: (originalRow) => originalRow.info.dp,
				id: "disk",
				name: () => t`Disk`,
				cell: CellFormatter,
				Icon: HardDriveIcon,
				header: sortableHeader,
			},
			{
				accessorFn: (originalRow) => originalRow.info.g,
				id: "gpu",
				name: () => "GPU",
				cell: CellFormatter,
				Icon: GpuIcon,
				header: sortableHeader,
			},
			{
				id: "loadAverage",
				accessorFn: ({ info }) => {
					const { l1 = 0, l5 = 0, l15 = 0 } = info
					return l1 + l5 + l15
				},
				name: () => t({ message: "Load Avg", comment: "Short label for load average" }),
				size: 0,
				Icon: HourglassIcon,
				header: sortableHeader,
				cell(info: CellContext<SystemRecord, unknown>) {
					const { info: sysInfo, status } = info.row.original
					if (sysInfo.l1 === undefined) {
						return null
					}

					const { l1 = 0, l5 = 0, l15 = 0, t: cpuThreads = 1 } = sysInfo
					const loadAverages = [l1, l5, l15]

					function getDotColor() {
						const max = Math.max(...loadAverages)
						const normalized = max / cpuThreads
						if (status !== "up") return "bg-primary/30"
						if (normalized < 0.7) return "bg-green-500"
						if (normalized < 1) return "bg-yellow-500"
						return "bg-red-600"
					}

					return (
						<div className="flex items-center gap-[.35em] w-full tabular-nums tracking-tight">
							<span className={cn("inline-block size-2 rounded-full me-0.5", getDotColor())} />
							{loadAverages.map((la, i) => (
								<span key={i}>{decimalString(la, la >= 10 ? 1 : 2)}</span>
							))}
						</div>
					)
				},
			},
			{
				accessorFn: ({ info }) => info.bb || (info.b || 0) * 1024 * 1024,
				id: "net",
				name: () => t`Net`,
				size: 0,
				Icon: EthernetIcon,
				header: sortableHeader,
				cell(info) {
					const sys = info.row.original
					if (sys.status === "paused") {
						return null
					}
					const userSettings = useStore($userSettings)
					const { value, unit } = formatBytes(info.getValue() as number, true, userSettings.unitNet, false)
					return (
						<span className="tabular-nums whitespace-nowrap">
							{decimalString(value, value >= 100 ? 1 : 2)} {unit}
						</span>
					)
				},
			},
			{
				accessorFn: (originalRow) => originalRow.info.dt,
				id: "temp",
				name: () => t({ message: "Temp", comment: "Temperature label in systems table" }),
				size: 50,
				Icon: ThermometerIcon,
				header: sortableHeader,
				cell(info) {
					const val = info.getValue() as number
					if (!val) {
						return null
					}
					const userSettings = useStore($userSettings)
					const { value, unit } = formatTemperature(val, userSettings.unitTemp)
					return (
						<span className={cn("tabular-nums whitespace-nowrap", viewMode === "table" && "ps-0.5")}>
							{decimalString(value, value >= 100 ? 1 : 2)} {unit}
						</span>
					)
				},
			},
			{
				accessorFn: (originalRow) => originalRow.info.u,
				id: "uptime",
				name: () => t`Uptime`,
				invertSorting: true,
				sortUndefined: -1,
				size: 60,
				Icon: ClockIcon,
				header: sortableHeader,
				cell(info) {
					const uptime = info.getValue() as number
					if (!uptime) return null
					return <span>{formatUptimeString(uptime)}</span>
				},
			},
			{
				accessorFn: (originalRow) => originalRow.updated,
				id: "lastSeen",
				name: () => t`Last Seen`,
				size: 120,
				header: sortableHeader,
				sortUndefined: -1,
				cell(info) {
					const system = info.row.original
					if (!system.updated) {
						return (
							<span className={cn("tabular-nums whitespace-nowrap", { "ps-1": viewMode === "table" })}>-</span>
						);
					}
					const now = Date.now();
					const lastSeenTime = new Date(system.updated).getTime();
					const diff = Math.max(0, Math.floor((now - lastSeenTime) / 1000)); // in seconds
					let display: React.ReactNode;
					if (system.status !== "up") {
						// Always show absolute time for offline systems
						const d = new Date(system.updated);
						display = d.toLocaleString(undefined, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
					} else if (diff < 60) {
						display = t`Just now`;
					} else if (diff < 3600) {
						const mins = Math.trunc(diff / 60);
						display = <Plural value={mins} one="# minute ago" other="# minutes ago" />;
					} else if (diff < 172800) {
						const hours = Math.trunc(diff / 3600);
						display = <Plural value={hours} one="# hour ago" other="# hours ago" />;
					} else {
						const days = Math.trunc(diff / 86400);
						display = <Plural value={days} one="# day ago" other="# days ago" />;
					}
					return (
						<span className={cn("flex items-center gap-1 tabular-nums whitespace-nowrap", { "ps-1": viewMode === "table" })}>
							{display}
						</span>
					);
				},
			},
			{
				accessorKey: "info.v",
				accessorFn: (originalRow) => originalRow.info.v,
				id: "agent",
				name: () => t`Agent`,
				// invertSorting: true,
				size: 50,
				Icon: WifiIcon,
				header: sortableHeader,
				cell(info) {
					const version = info.getValue() as string
					if (!version) {
						return null
					}
					const system = info.row.original
					return (
						<span className={cn("flex gap-2 items-center md:pe-5 tabular-nums", viewMode === "table" && "ps-0.5")}>
							<IndicatorDot
								system={system}
								className={
									(system.status !== "up" && "bg-primary/30") ||
									(version === globalThis.BESZEL.HUB_VERSION && "bg-green-500") ||
									"bg-yellow-500"
								}
							/>
							<span className="truncate max-w-14">{info.getValue() as string}</span>
						</span>
					)
				},
			},
			{
				id: "actions",
				// @ts-ignore
				name: () => t({ message: "Actions", comment: "Table column" }),
				size: 50,
				cell: ({ row }) => (
					<div className="flex justify-end items-center gap-1 -ms-3">
						<AlertsButton system={row.original} />
						<ActionsButton system={row.original} />
					</div>
				),
			},
		] as ColumnDef<SystemRecord>[]
	}, [])
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
						<CardTitle className="mb-2.5">
							<Trans>All Systems</Trans>
						</CardTitle>
						<CardDescription>
							<Trans>Updated in real time. Click on a system to view information.</Trans>
						</CardDescription>
					</div>
					<div className="flex gap-2 ms-auto w-full md:w-auto">
						<Input placeholder={t`Filter...`} onChange={(e) => setFilter(e.target.value)} className="px-4 w-full md:w-80" />
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline">
									<Settings2Icon className="me-1.5 size-4 opacity-80" />
									<Trans>View</Trans>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
								<div className="grid grid-cols-1 md:grid-cols-4 divide-y md:divide-s md:divide-y-0">
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
											<FilterIcon className="size-4" />
											<Trans>Status</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<DropdownMenuRadioGroup
											className="px-1 pb-1"
											value={statusFilter}
											onValueChange={(value) => setStatusFilter(value as StatusFilter)}
										>
											<DropdownMenuRadioItem value="all" onSelect={(e) => e.preventDefault()} className="gap-2">
												<Trans>All Systems</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="up" onSelect={(e) => e.preventDefault()} className="gap-2">
												<Trans>Up</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="down" onSelect={(e) => e.preventDefault()} className="gap-2">
												<Trans>Down</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="paused" onSelect={(e) => e.preventDefault()} className="gap-2">
												<Trans>Paused</Trans>
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
	}, [visibleColumns.length, sorting, viewMode, locale, statusFilter])

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
				<TableBody>
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
									) : Icon && <Icon className="size-4 text-muted-foreground" />}
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
