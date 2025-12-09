import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import {
	type ColumnDef,
	type ColumnFiltersState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type Row,
	type SortingState,
	type Table as TableType,
	useReactTable,
	type VisibilityState,
} from "@tanstack/react-table"
import { useVirtualizer, type VirtualItem } from "@tanstack/react-virtual"
import {
	ArrowDownIcon,
	ArrowUpDownIcon,
	ArrowUpIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	EyeIcon,
	FilterIcon,
	LayoutGridIcon,
	LayoutListIcon,
	Settings2Icon,
	XIcon,
} from "lucide-react"
import { memo, useEffect, useMemo, useRef, useState } from "react"
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
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { SystemStatus } from "@/lib/enums"
import { $downSystems, $pausedSystems, $systems, $upSystems, $userSettings } from "@/lib/stores"
import { cn, runOnce, useBrowserStorage } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { useDebounce } from "@/lib/useDebounce"
import { bulkRenameTag, bulkDeleteTag, getSystemsWithTag } from "@/lib/bulkOperations"
import { toast } from "@/components/ui/use-toast"
import { getSystemTags } from "@/lib/tagCache"
import AlertButton from "../alerts/alert-button"
import { $router, Link } from "../router"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { SystemsTableColumns, ActionsButton, IndicatorDot } from "./systems-table-columns"

type ViewMode = "table" | "grid"
type StatusFilter = "all" | SystemRecord["status"]

const preloadSystemDetail = runOnce(() => import("@/components/routes/system.tsx"))

export default function SystemsTable() {
	const data = useStore($systems)
	const downSystems = $downSystems.get()
	const upSystems = $upSystems.get()
	const pausedSystems = $pausedSystems.get()
	const userSettings = useStore($userSettings)
	const { i18n, t } = useLingui()
	const [filter, setFilter] = useState<string>("")
	const debouncedFilter = useDebounce(filter, 300)
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		"sortMode",
		[{ id: "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useBrowserStorage<VisibilityState>("cols", {})
	const [selectedTags, setSelectedTags] = useState<string[]>([])
	const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set())

	const locale = i18n.locale

	// Use cached tag computation for better performance
	const allTags = useMemo(() => getSystemTags(data), [data])

	// Filter data based on status filter
	const statusFilteredData = useMemo(() => {
		if (statusFilter === "all") {
			return data
		}
		if (statusFilter === SystemStatus.Up) {
			return Object.values(upSystems) ?? []
		}
		if (statusFilter === SystemStatus.Down) {
			return Object.values(downSystems) ?? []
		}
		return Object.values(pausedSystems) ?? []
	}, [data, statusFilter])

	// Optimized filtering with combined operations and early returns
	const filteredData = useMemo(() => {
		if (!selectedTags.length) return statusFilteredData

		const selectedTagsSet = new Set(selectedTags)
		return statusFilteredData.filter((sys) =>
			sys.tags?.some(tag => selectedTagsSet.has(tag)) ?? false
		)
	}, [statusFilteredData, selectedTags])

	const [viewMode, setViewMode] = useBrowserStorage<ViewMode>(
		"viewMode",
		// show grid view on mobile if there are less than 200 systems (looks better but table is more efficient)
		window.innerWidth < 1024 && filteredData.length < 200 ? "grid" : "table"
	)

	useEffect(() => {
		if (debouncedFilter !== undefined) {
			table.getColumn("system")?.setFilterValue(debouncedFilter)
		}
	}, [debouncedFilter])

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

	const [upSystemsLength, downSystemsLength, pausedSystemsLength] = useMemo(() => {
		return [Object.values(upSystems).length, Object.values(downSystems).length, Object.values(pausedSystems).length]
	}, [upSystems, downSystems, pausedSystems])

	// Optimized group processing with reduce
	const groupedSystems = useMemo(() => {
		return filteredData.reduce((groups, system) => {
			const group = system.group || "Ungrouped"
			groups[group] ??= []
			groups[group].push(system)
			return groups
		}, {} as Record<string, SystemRecord[]>)
	}, [filteredData])

	// Determine if there is at least one system with a non-empty group
	const hasAnyGrouped = useMemo(() => filteredData.some(sys => sys.group && sys.group.trim()), [filteredData])

	// TODO: hiding temp then gpu messes up table headers
	const CardHead = useMemo(() => {
		return (
			<CardHeader className="pb-4.5 px-2 sm:px-6 max-sm:pt-5 max-sm:pb-1">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>All Systems</Trans>
						</CardTitle>
						<CardDescription className="flex">
							<Trans>Click on a system to view more information.</Trans>
						</CardDescription>
					</div>

					<div className="flex gap-2 ms-auto w-full md:w-auto items-center">
						<div className="relative flex-1 md:w-80">
							<Input
								placeholder={t`Filter...`}
								onChange={(e) => setFilter(e.target.value)}
								value={filter}
								className="ps-4 pe-10 w-full"
							/>
							{filter && (
								<Button
									type="button"
									variant="ghost"
									size="icon"
									aria-label={t`Clear`}
									className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
									onClick={() => setFilter("")}
								>
									<XIcon className="h-4 w-4" />
								</Button>
							)}
						</div>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline" className="flex gap-2 items-center">
									<span>{t`Tags`}</span>
									{selectedTags.length > 0 && (
										<Badge variant="secondary" className="px-1.5 py-0.5 text-xs">
											{selectedTags.length}
										</Badge>
									)}
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="start" className="min-w-[14rem] max-h-72 overflow-y-auto">
								<DropdownMenuLabel>{t`Filter by tags`}</DropdownMenuLabel>
								<DropdownMenuSeparator />
								{allTags.length === 0 && <div className="px-3 py-2 text-muted-foreground">{t`No tags found`}</div>}
								{allTags.map((tag) => (
									<DropdownMenuCheckboxItem
										key={tag}
										checked={selectedTags.includes(tag)}
										onCheckedChange={(checked) => {
											setSelectedTags((prev) =>
												checked ? [...prev, tag] : prev.filter((t) => t !== tag)
											)
										}}
									>
										{tag}
									</DropdownMenuCheckboxItem>
								))}
							</DropdownMenuContent>
						</DropdownMenu>
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
												<Trans>Up ({upSystemsLength})</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="down" onSelect={(e) => e.preventDefault()}>
												<Trans>Down ({downSystemsLength})</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="paused" onSelect={(e) => e.preventDefault()}>
												<Trans>Paused ({pausedSystemsLength})</Trans>
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
	}, [
		visibleColumns.length,
		sorting,
		viewMode,
		locale,
		statusFilter,
		upSystemsLength,
		downSystemsLength,
		pausedSystemsLength,
		selectedTags,
		allTags,
		filter,
	])

	return (
		<Card>
			{CardHead}
			<div className="p-6 pt-0 max-sm:py-3 max-sm:px-2">
				{userSettings.groupingEnabled ? (
					// Grouped layout
					Object.entries(groupedSystems)
						.filter(([group]) => group !== "Ungrouped" || hasAnyGrouped)
						.map(([group, systems]) => {
							const systemRows = systems.map(sys => table.getRowModel().rows.find(r => r.original.id === sys.id)).filter((row): row is Row<SystemRecord> => row !== undefined)
							const isCollapsed = collapsedGroups.has(group)
							
							return (
								<div key={group} className="mb-8">
									<button
										onClick={() => {
											const newCollapsed = new Set(collapsedGroups)
											if (isCollapsed) {
												newCollapsed.delete(group)
											} else {
												newCollapsed.add(group)
											}
											setCollapsedGroups(newCollapsed)
										}}
										className="flex items-center gap-2 text-xl font-bold mb-2 hover:text-primary transition-colors cursor-pointer"
									>
										{isCollapsed ? (
											<ChevronRightIcon className="h-5 w-5" />
										) : (
											<ChevronDownIcon className="h-5 w-5" />
										)}
										{group}
									</button>
									<div className="text-sm text-muted-foreground mb-3">
										{(() => {
											const onlineCount = systemRows.filter(row => row.original.status === "up").length
											const offlineCount = systemRows.filter(row => row.original.status !== "up").length
											return (
												<span>
													{onlineCount} online, {offlineCount} offline
												</span>
											)
										})()}
									</div>
									{!isCollapsed && (
										<>
											{viewMode === "table" ? (
												<div className="rounded-md border overflow-hidden">
													<AllSystemsTable table={table} rows={systemRows} colLength={visibleColumns.length} />
												</div>
											) : (
												<div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
													{systemRows.length ? (
														systemRows.map((row) => {
															return <SystemCard key={row.original.id} row={row} table={table} colLength={visibleColumns.length} />
														})
													) : (
														<div className="col-span-full text-center py-8">
															<Trans>No systems found.</Trans>
														</div>
													)}
												</div>
											)}
										</>
									)}
								</div>
							)
						})
				) : (
					// Simple layout (original)
					viewMode === "table" ? (
						<div className="rounded-md">
							<AllSystemsTable table={table} rows={rows} colLength={visibleColumns.length} />
						</div>
					) : (
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
					)
				)}
			</div>
		</Card>
	)
}

const AllSystemsTable = memo(
	({ table, rows, colLength }: { table: TableType<SystemRecord>; rows: Row<SystemRecord>[]; colLength: number }) => {
		// The virtualizer will need a reference to the scrollable container element
		const scrollRef = useRef<HTMLDivElement>(null)

		const virtualizer = useVirtualizer<HTMLDivElement, HTMLTableRowElement>({
			count: rows.length,
			estimateSize: () => (rows.length > 10 ? 56 : 60),
			getScrollElement: () => scrollRef.current,
			overscan: 5,
		})
		const virtualRows = virtualizer.getVirtualItems()

		const paddingTop = Math.max(0, virtualRows[0]?.start ?? 0 - virtualizer.options.scrollMargin)
		const paddingBottom = Math.max(0, virtualizer.getTotalSize() - (virtualRows[virtualRows.length - 1]?.end ?? 0))

		return (
			<div
				className={cn(
					"h-min max-h-[calc(100dvh-17rem)] max-w-full relative overflow-auto border rounded-md",
					// don't set min height if there are less than 2 rows, do set if we need to display the empty state
					(!rows.length || rows.length > 2) && "min-h-50"
				)}
				ref={scrollRef}
			>
				{/* add header height to table size */}
				<div style={{ height: `${virtualizer.getTotalSize() + 50}px`, paddingTop, paddingBottom }}>
					<table className="text-sm w-full h-full">
						<SystemsTableHead table={table} />
						<TableBody onMouseEnter={preloadSystemDetail}>
							{rows.length ? (
								virtualRows.map((virtualRow) => {
									const row = rows[virtualRow.index] as Row<SystemRecord>
									return (
										<SystemTableRow
											key={row.id}
											row={row}
											virtualRow={virtualRow}
											length={rows.length}
											colLength={colLength}
										/>
									)
								})
							) : (
								<TableRow>
									<TableCell colSpan={colLength} className="h-37 text-center pointer-events-none">
										<Trans>No systems found.</Trans>
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</table>
				</div>
			</div>
		)
	}
)

function SystemsTableHead({ table }: { table: TableType<SystemRecord> }) {
	const { t } = useLingui()
	return (
		<TableHeader className="sticky top-0 z-50 w-full border-b-2">
			{table.getHeaderGroups().map((headerGroup) => (
				<tr key={headerGroup.id}>
					{headerGroup.headers.map((header) => {
						return (
							<TableHead className="px-1.5" key={header.id}>
								{flexRender(header.column.columnDef.header, header.getContext())}
							</TableHead>
						)
					})}
				</tr>
			))}
		</TableHeader>
	)
}

const SystemTableRow = memo(
	({
		row,
		virtualRow,
		colLength,
	}: {
		row: Row<SystemRecord>
		virtualRow: VirtualItem
		length: number
		colLength: number
	}) => {
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
								height: virtualRow.size,
							}}
							className="py-0"
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
						<div className="flex items-center gap-2 w-full overflow-hidden">
							<CardTitle className="text-base tracking-normal text-primary/90 flex items-center min-w-0 flex-1 gap-2.5">
								<div className="flex items-center gap-2.5 min-w-0 flex-1">
									<IndicatorDot system={system} />
									<span className="text-[.95em]/normal tracking-normal text-primary/90 truncate">{system.name}</span>
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
					<CardContent className="text-sm px-5 pt-3.5 pb-4">
						<div className="grid gap-2.5" style={{ gridTemplateColumns: "24px minmax(80px, max-content) 1fr" }}>
							{table.getAllColumns().map((column) => {
								if (!column.getIsVisible() || column.id === "system" || column.id === "actions") return null
								const cell = row.getAllCells().find((cell) => cell.column.id === column.id)
								if (!cell) return null
								// @ts-expect-error
								const { Icon, name } = column.columnDef as ColumnDef<SystemRecord, unknown>
								return (
									<>
										<div key={`${column.id}-icon`} className="flex items-center">
											{column.id === "lastSeen" ? (
												<EyeIcon className="size-4 text-muted-foreground" />
											) : (
												Icon && <Icon className="size-4 text-muted-foreground" />
											)}
										</div>
										<div key={`${column.id}-label`} className="flex items-center text-muted-foreground pr-3">
											{name()}:
										</div>
										<div key={`${column.id}-value`} className="flex items-center">
											{flexRender(cell.column.columnDef.cell, cell.getContext())}
										</div>
									</>
								)
							})}
						</div>
					</CardContent>
					<Link
						href={getPagePath($router, "system", { id: row.original.id })}
						className="inset-0 absolute w-full h-full"
					>
						<span className="sr-only">{row.original.name}</span>
					</Link>
				</Card>
			)
		}, [system, colLength, t])
	}
)