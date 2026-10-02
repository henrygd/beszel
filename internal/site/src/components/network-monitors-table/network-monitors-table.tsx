import { getCertDaysLeft, getCertExpiryLevel, getMonitorTarget } from "@/lib/network-monitor-utils"
import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	type ColumnFiltersState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getPaginationRowModel,
	getSortedRowModel,
	type Row,
	type RowSelectionState,
	type SortingState,
	type Table as TableType,
	useReactTable,
	type VisibilityState,
} from "@tanstack/react-table"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button, buttonVariants } from "@/components/ui/button"
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { subscribeKeys } from "nanostores"
import { getMonitorColumns } from "@/components/network-monitors-table/network-monitors-columns"
import { Card, CardHeader, CardTitle } from "@/components/ui/card"
import { DataTablePagination, usePagination } from "@/components/ui/data-table-pagination"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useToast } from "@/components/ui/use-toast"
import { isReadOnlyUser, queueUserSettings } from "@/lib/api"
import { pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import { $allSystemsById, $direction, $userSettings, getUserChartTime } from "@/lib/stores"
import { cn, formatShortDate, matchesFilterGroups, parseFilterGroups, parseSemVer } from "@/lib/utils"
import type { ChartOptions, MonitorCertInfo, NetworkMonitorRecord } from "@/types"
import { AddMonitorDialog, EditMonitorDialog, MonitorMultiSelect, SystemMultiSelect } from "./monitor-dialog"
import {
	ArrowDownIcon,
	ArrowLeftRightIcon,
	ArrowUpDownIcon,
	ArrowUpIcon,
	EthernetPortIcon,
	EyeIcon,
	GlobeIcon,
	LandmarkIcon,
	LoaderCircleIcon,
	ServerIcon,
	Settings2Icon,
	ShieldCheckIcon,
	XIcon,
} from "lucide-react"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { LossChart, AvgMinMaxResponseChart, ResponseChart } from "@/components/routes/system/charts/monitors-charts"
import { getMonitorCompareState } from "@/lib/monitor-compare"
import { useCompareMonitors, useNetworkMonitorStats } from "@/lib/use-network-monitors"
import { useStore } from "@nanostores/react"
import { atom } from "nanostores"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { getPagePath } from "@nanostores/router"

export default function NetworkMonitorsTableNew({
	systemId,
	monitors,
	isLoading,
}: {
	systemId?: string
	monitors: NetworkMonitorRecord[]
	isLoading: boolean
}) {
	const sortSettingsKey = systemId ? "monitorSortModeSystem" : "monitorSortMode"
	const sortStorageKey = `besz-sort-np-target-${systemId ? 1 : 0}`
	const [sorting, setSorting] = useState<SortingState>(
		() =>
			$userSettings.get()[sortSettingsKey] ??
			JSON.parse(sessionStorage.getItem(sortStorageKey) || "null") ?? [
				{ id: systemId ? "target" : "system", desc: false },
			]
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>(
		() => $userSettings.get().monitorCols ?? JSON.parse(localStorage.getItem("besz-monitor-cols") || "{}")
	)
	const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
	const [globalFilter, setGlobalFilter] = useState("")
	const { pagination, onPaginationChange, resetPageIndex } = usePagination()
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [pendingDelete, setPendingDelete] = useState<NetworkMonitorRecord[]>([])
	const [editingMonitor, setEditingMonitor] = useState<NetworkMonitorRecord>()

	const { toast } = useToast()
	const canManageMonitors = !isReadOnlyUser()

	// Apply settings from server once they load (handles incognito / new devices)
	const appliedSettings = useRef(new Set<string>())
	useEffect(() => {
		return subscribeKeys($userSettings, ["monitorCols", sortSettingsKey], (vals) => {
			if (!appliedSettings.current.has("monitorCols") && vals.monitorCols !== undefined) {
				appliedSettings.current.add("monitorCols")
				setColumnVisibility(vals.monitorCols)
			}
			if (!appliedSettings.current.has(sortSettingsKey) && vals[sortSettingsKey] !== undefined) {
				appliedSettings.current.add(sortSettingsKey)
				setSorting(vals[sortSettingsKey] as SortingState)
			}
		})
	}, [sortSettingsKey])

	const handleColumnVisibilityChange = useCallback(
		(updater: VisibilityState | ((prev: VisibilityState) => VisibilityState)) => {
			setColumnVisibility((prev) => {
				const next = typeof updater === "function" ? updater(prev) : updater
				localStorage.setItem("besz-monitor-cols", JSON.stringify(next))
				$userSettings.setKey("monitorCols", next)
				queueUserSettings({ monitorCols: next })
				return next
			})
		},
		[]
	)

	const handleSortingChange = useCallback(
		(updater: SortingState | ((prev: SortingState) => SortingState)) => {
			setSorting((prev) => {
				const next = typeof updater === "function" ? updater(prev) : updater
				sessionStorage.setItem(sortStorageKey, JSON.stringify(next))
				$userSettings.setKey(sortSettingsKey, next)
				queueUserSettings({ [sortSettingsKey]: next })
				return next
			})
			resetPageIndex()
		},
		[sortSettingsKey, sortStorageKey, resetPageIndex]
	)

	const runMonitorBatch = useCallback(
		async (ids: string[], enqueue: (batch: ReturnType<typeof pb.createBatch>, id: string) => void) => {
			let batch = pb.createBatch()
			let inBatch = 0
			for (const id of ids) {
				enqueue(batch, id)
				if (++inBatch >= 20) {
					await batch.send()
					batch = pb.createBatch()
					inBatch = 0
				}
			}
			if (inBatch) {
				await batch.send()
			}
		},
		[]
	)

	const handleDeleteRequest = useCallback((monitorsToDelete: NetworkMonitorRecord[]) => {
		if (!monitorsToDelete.length) {
			return
		}
		setPendingDelete(monitorsToDelete)
		setDeleteOpen(true)
	}, [])

	const handleConfirmDelete = async () => {
		setDeleteOpen(false)
		const ids = pendingDelete.map((monitor) => monitor.id)
		if (!ids.length) {
			return
		}

		try {
			if (ids.length === 1) {
				await pb.collection("network_monitors").delete(ids[0])
			} else {
				await runMonitorBatch(ids, (batch, id) => batch.collection("network_monitors").delete(id))
			}
			setPendingDelete([])
			setRowSelection({})
		} catch (err: unknown) {
			toast({
				variant: "destructive",
				title: t`Error`,
				description: (err as Error)?.message || t`Failed to delete monitors.`,
			})
		}
	}

	const handleSetEnabled = useCallback(
		async (monitorsToUpdate: NetworkMonitorRecord[], enabled: boolean) => {
			if (!monitorsToUpdate.length) {
				return
			}

			const pendingUpdates = monitorsToUpdate.filter((monitor) => monitor.enabled !== enabled)
			if (!pendingUpdates.length) {
				return
			}

			try {
				if (pendingUpdates.length === 1) {
					await pb.collection("network_monitors").update(pendingUpdates[0].id, { enabled })
					return
				}
				await runMonitorBatch(
					pendingUpdates.map((monitor) => monitor.id),
					(batch, id) => batch.collection("network_monitors").update(id, { enabled })
				)
				if (monitorsToUpdate.length > 1) {
					setRowSelection({})
				}
			} catch (err: unknown) {
				toast({
					variant: "destructive",
					title: t`Error`,
					description: (err as Error)?.message || t`Failed to update monitors.`,
				})
			}
		},
		[runMonitorBatch, toast]
	)

	const columns = useMemo(() => {
		let columns = getMonitorColumns({
			onEdit: setEditingMonitor,
			onDelete: handleDeleteRequest,
			onSetEnabled: handleSetEnabled,
		})
		columns = systemId ? columns.filter((col) => col.id !== "system") : columns
		columns = canManageMonitors ? columns : columns.filter((col) => col.id !== "actions")
		return columns
	}, [canManageMonitors, handleDeleteRequest, handleSetEnabled, systemId])

	const table = useReactTable({
		data: monitors,
		columns,
		getRowId: (row) => row.id,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		autoResetPageIndex: false,
		onPaginationChange,
		onSortingChange: handleSortingChange,
		onColumnFiltersChange: setColumnFilters,
		onColumnVisibilityChange: handleColumnVisibilityChange,
		onRowSelectionChange: setRowSelection,
		defaultColumn: {
			sortUndefined: "last",
			size: 900,
			minSize: 0,
		},
		state: {
			sorting,
			columnFilters,
			columnVisibility,
			rowSelection,
			globalFilter,
			pagination,
		},
		onGlobalFilterChange: (value) => {
			setGlobalFilter(value)
			resetPageIndex()
		},
		globalFilterFn: (row, _columnId, filterValue) => {
			const value = (filterValue as string).trim()
			if (!value) return true
			const monitor = row.original
			const systemName = $allSystemsById.get()[monitor.system]?.name ?? ""
			const searchString = `${getMonitorTarget(monitor)}${monitor.protocol}${systemName}`.toLocaleLowerCase()
			return matchesFilterGroups(searchString, parseFilterGroups(value))
		},
	})

	const rows = table.getRowModel().rows
	const visibleColumns = table.getVisibleLeafColumns()
	const visibleColumnsKey = visibleColumns.map((column) => column.id).join(",")

	return (
		<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 mb-3 sm:mb-4">
				<div className="grid md-lg:flex gap-x-5 gap-y-3 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>Network Monitors</Trans>
						</CardTitle>
						<div className="text-sm text-muted-foreground flex items-center flex-wrap">
							<Trans>Response time monitoring from agents.</Trans>
						</div>
					</div>
					<div className="md-lg:ms-auto flex items-center gap-2">
						{monitors.length > 0 && (
							<div className="relative grow">
								<Input
									placeholder={t`Filter...`}
									value={globalFilter}
									onChange={(e) => table.setGlobalFilter(e.target.value)}
									className="ms-auto px-4 w-full max-w-full md-lg:w-50"
								/>
								{globalFilter && (
									<Button
										type="button"
										variant="ghost"
										size="icon"
										aria-label={t`Clear`}
										className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
										onClick={() => table.setGlobalFilter("")}
									>
										<XIcon className="h-4 w-4" />
									</Button>
								)}
							</div>
						)}
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline">
									<Settings2Icon className="me-1.5 size-4 opacity-80" />
									<Trans>View</Trans>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
								<div className="grid grid-cols-2 divide-y md:divide-s md:divide-y-0">
									<div className="border-r">
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<ArrowUpDownIcon className="size-4" />
											<Trans>Sort By</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<div className="px-1 pb-1">
											{table.getAllColumns().map((column) => {
												if (!column.getCanSort()) return null
												let Icon = <span className="w-6"></span>
												if (sorting[0]?.id === column.id) {
													Icon = sorting[0]?.desc ? (
														<ArrowUpIcon className="me-2 size-4" />
													) : (
														<ArrowDownIcon className="me-2 size-4" />
													)
												}
												return (
													<DropdownMenuItem
														onSelect={(e) => {
															e.preventDefault()
															handleSortingChange([
																{ id: column.id, desc: sorting[0]?.id === column.id && !sorting[0]?.desc },
															])
														}}
														key={column.id}
													>
														{Icon}
														{column.columnDef.meta?.label ?? column.id}
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
												.map((column) => (
													<DropdownMenuCheckboxItem
														key={column.id}
														onSelect={(e) => e.preventDefault()}
														checked={column.getIsVisible()}
														onCheckedChange={(value) => column.toggleVisibility(!!value)}
													>
														{column.columnDef.meta?.label ?? column.id}
													</DropdownMenuCheckboxItem>
												))}
										</div>
									</div>
								</div>
							</DropdownMenuContent>
						</DropdownMenu>
						{canManageMonitors ? <AddMonitorDialog systemId={systemId} monitors={monitors} /> : null}
						{canManageMonitors ? (
							<EditMonitorDialog
								systemId={systemId}
								monitor={editingMonitor}
								open={!!editingMonitor}
								setOpen={(open) => {
									if (!open) {
										setEditingMonitor(undefined)
									}
								}}
							/>
						) : null}
						<AlertDialog
							open={deleteOpen}
							onOpenChange={(open) => {
								setDeleteOpen(open)
								if (!open) {
									setPendingDelete([])
								}
							}}
						>
							<AlertDialogContent>
								<AlertDialogHeader>
									<AlertDialogTitle>
										<Trans>Are you sure?</Trans>
									</AlertDialogTitle>
									<AlertDialogDescription>
										<Trans>This will permanently delete all selected records from the database.</Trans>
									</AlertDialogDescription>
								</AlertDialogHeader>
								<AlertDialogFooter>
									<AlertDialogCancel>
										<Trans>Cancel</Trans>
									</AlertDialogCancel>
									<AlertDialogAction
										className={cn(buttonVariants({ variant: "destructive" }))}
										onClick={handleConfirmDelete}
									>
										<Trans>Continue</Trans>
									</AlertDialogAction>
								</AlertDialogFooter>
							</AlertDialogContent>
						</AlertDialog>
					</div>
				</div>
			</CardHeader>
			<div className="rounded-md">
				<NetworkMonitorsTable
					table={table}
					rows={rows}
					colLength={visibleColumns.length}
					visibleColumnsKey={visibleColumnsKey}
					rowSelection={rowSelection}
					isLoading={isLoading}
					includesAllSystems={!systemId}
				/>
			</div>
			<DataTablePagination table={table} />
		</Card>
	)
}

const NetworkMonitorsTable = memo(function NetworkMonitorTable({
	table,
	rows,
	colLength,
	visibleColumnsKey,
	rowSelection,
	isLoading,
	includesAllSystems,
}: {
	table: TableType<NetworkMonitorRecord>
	rows: Row<NetworkMonitorRecord>[]
	colLength: number
	visibleColumnsKey: string
	rowSelection: RowSelectionState
	isLoading: boolean
	/** The table lists every system's monitors, so the sheet can compare without fetching. */
	includesAllSystems: boolean
}) {
	const [sheetOpen, setSheetOpen] = useState(false)
	const [activeMonitorId, setActiveMonitorId] = useState<string | null>(null)
	const activeMonitor = activeMonitorId
		? table.options.data.find((monitor) => monitor.id === activeMonitorId)
		: undefined
	const openSheet = useCallback((monitor: NetworkMonitorRecord) => {
		setActiveMonitorId(monitor.id)
		setSheetOpen(true)
	}, [])

	return (
		<div className="max-w-full relative overflow-auto border rounded-md">
			<table className="text-sm w-full h-full text-nowrap">
				<NetworkMonitorTableHead table={table} />
				<TableBody>
					{rows.length ? (
						rows.map((row) => {
							return (
								<NetworkMonitorTableRow
									key={row.id}
									row={row}
									isSelected={row.getIsSelected()}
									rowSelection={rowSelection}
									visibleColumnsKey={visibleColumnsKey}
									openSheet={openSheet}
								/>
							)
						})
					) : (
						<TableRow>
							<TableCell colSpan={colLength} className="h-37 text-center pointer-events-none">
								{isLoading ? (
									<LoaderCircleIcon className="animate-spin size-10 opacity-60 mx-auto" />
								) : (
									<Trans>No results.</Trans>
								)}
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</table>
			<NetworkMonitorSheet
				open={sheetOpen}
				onOpenChange={(nextOpen) => {
					setSheetOpen(nextOpen)
				}}
				monitor={activeMonitor}
				monitors={table.options.data}
				includesAllSystems={includesAllSystems}
			/>
		</div>
	)
})

function NetworkMonitorTableHead({ table }: { table: TableType<NetworkMonitorRecord> }) {
	return (
		<TableHeader className="sticky top-0 z-50 w-full border-b-2">
			{table.getHeaderGroups().map((headerGroup) => (
				<tr key={headerGroup.id}>
					{headerGroup.headers.map((header) => {
						return (
							<TableHead className="px-2" key={header.id}>
								{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
							</TableHead>
						)
					})}
				</tr>
			))}
		</TableHeader>
	)
}

const NetworkMonitorTableRow = memo(function NetworkMonitorTableRow({
	row,
	isSelected,
	rowSelection: _rowSelection,
	// Column visibility doesn't change the row object identity, so this prop exists only
	// to force a re-render (and a fresh row.getVisibleCells() read) when columns are toggled.
	visibleColumnsKey: _visibleColumnsKey,
	openSheet,
}: {
	row: Row<NetworkMonitorRecord>
	isSelected: boolean
	// Menus depend on the entire selection, including changes to other rows.
	rowSelection: RowSelectionState
	visibleColumnsKey: string
	openSheet: (monitor: NetworkMonitorRecord) => void
}) {
	const system = useStore($allSystemsById)[row.original.system]
	return (
		<TableRow
			data-state={isSelected && "selected"}
			className={cn("cursor-pointer transition-opacity", {
				"opacity-50": system?.status === SystemStatus.Paused,
			})}
			onClick={() => openSheet(row.original)}
		>
			{row.getVisibleCells().map((cell) => (
				<TableCell
					key={cell.id}
					className="py-0"
					style={{
						width: `${cell.column.getSize()}px`,
						height: 54,
					}}
				>
					{flexRender(cell.column.columnDef.cell, cell.getContext())}
				</TableCell>
			))}
		</TableRow>
	)
})

function NetworkMonitorSheet({
	open,
	onOpenChange,
	monitor,
	monitors,
	includesAllSystems,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	monitor?: NetworkMonitorRecord
	monitors: NetworkMonitorRecord[]
	includesAllSystems: boolean
}) {
	if (!monitor) {
		return null
	}

	return (
		<NetworkMonitorSheetContent
			key={monitor.system}
			open={open}
			onOpenChange={onOpenChange}
			monitor={monitor}
			monitors={monitors}
			includesAllSystems={includesAllSystems}
		/>
	)
}

const certExpiryTextColors = { ok: "", warning: "text-yellow-600 dark:text-yellow-500", critical: "text-red-500" }

function CertExpiry({ cert }: { cert: MonitorCertInfo }) {
	const daysLeft = getCertDaysLeft(cert)
	const expires = formatShortDate(new Date(cert.expires).toISOString())
	const level = getCertExpiryLevel(daysLeft)
	return (
		<>
			<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
			<ShieldCheckIcon className={cn("size-3.5 text-muted-foreground -me-1", certExpiryTextColors[level])} />
			<span className={certExpiryTextColors[level]}>
				{daysLeft < 0 ? <Trans>Certificate expired {expires}</Trans> : <Trans>Certificate expires {expires}</Trans>}
			</span>
			{cert.issuer && (
				<>
					<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
					<LandmarkIcon className="size-3.5 text-muted-foreground -me-0.5" />
					<span>{cert.issuer}</span>
				</>
			)}
		</>
	)
}

function NetworkMonitorSheetContent({
	open,
	onOpenChange,
	monitor,
	monitors,
	includesAllSystems,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	monitor: NetworkMonitorRecord
	/** Table monitors; used to find other targets on the same system to compare against. */
	monitors: NetworkMonitorRecord[]
	/** Whether `monitors` covers every system, so other systems' monitors needn't be fetched. */
	includesAllSystems: boolean
}) {
	// Keep monitor exploration independent of the system charts' time range.
	const [chartTimeStore] = useState(() => {
		const defaultTime = getUserChartTime()
		return atom(defaultTime === "1m" ? "1h" : defaultTime)
	})
	const chartTime = useStore(chartTimeStore)
	const direction = useStore($direction)
	const systems = useStore($allSystemsById)
	const system = systems[monitor.system]

	const [compareTargetIds, setCompareTargetIds] = useState<Set<string>>(() => new Set())
	const [compareSystemIds, setCompareSystemIds] = useState<Set<string>>(() => new Set())
	// Scoped to this sheet so a filter doesn't carry over to other monitors' sheets.
	const [compareFilterStore, setCompareFilterStore] = useState(() => atom(""))
	// The sheet is keyed by system (to keep the time range), so reset comparison state per monitor.
	const [compareMonitorId, setCompareMonitorId] = useState(monitor.id)
	if (compareMonitorId !== monitor.id) {
		setCompareMonitorId(monitor.id)
		setCompareSystemIds(new Set())
		setCompareTargetIds(new Set())
		setCompareFilterStore(atom(""))
	}
	// Other systems' monitors come from the table when it lists every system, otherwise from one fetch.
	const fetchedMonitors = useCompareMonitors(monitor.system, monitor.protocol, open && !includesAllSystems)
	const compare = useMemo(
		() =>
			getMonitorCompareState({
				monitor,
				localMonitors: monitors,
				otherMonitors: includesAllSystems ? monitors : fetchedMonitors,
				selectedSystemIds: compareSystemIds,
				selectedTargetIds: compareTargetIds,
				getSystemName: (id) => systems[id]?.name ?? id,
			}),
		[monitor, monitors, includesAllSystems, fetchedMonitors, compareSystemIds, compareTargetIds, systems]
	)
	const { compareMonitors } = compare
	const comparing = compareMonitors.length > 1

	const monitorStats = useNetworkMonitorStats({
		systemId: monitor.system,
		monitorIds: comparing ? compareMonitors.map((m) => m.id) : [monitor.id],
		interval: monitor.interval,
		chartTime,
		enabled: open,
	})

	const chartData = useMemo<ChartOptions>(
		() => ({
			agentVersion: parseSemVer(system?.info?.v),
			orientation: direction === "rtl" ? "right" : "left",
			chartTime,
		}),
		[system?.info?.v, direction, chartTime]
	)
	const hasMonitorStats = comparing
		? monitorStats.some((record) => record.stats != null)
		: monitorStats.some((record) => record.stats?.[monitor.id] != null)
	const monitorLabel = getMonitorTarget(monitor)

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-220 overflow-auto p-4 sm:p-6">
				<SheetHeader className="mb-0 border-b p-0 pb-4">
					<SheetTitle>{monitorLabel}</SheetTitle>
					<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
						<ServerIcon className="size-3.5 text-muted-foreground" />
						<Link className="hover:underline" href={getPagePath($router, "system", { id: system?.id ?? "" })}>
							{system?.name ?? ""}
						</Link>
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						<ArrowLeftRightIcon className="size-3.5 text-muted-foreground -me-0.5" />
						{monitor.protocol.toUpperCase()}
						{monitor.protocol === "tcp" && monitor.port > 0 && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<EthernetPortIcon className="size-3.5 text-muted-foreground" />
								<span>{monitor.port}</span>
							</>
						)}
						{monitor.protocol === "dns" && monitor.server && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<GlobeIcon className="size-3.5 text-muted-foreground" />
								<span>{monitor.server}</span>
							</>
						)}
						{monitor.certInfo?.expires ? <CertExpiry cert={monitor.certInfo} /> : null}
					</SheetDescription>
				</SheetHeader>
				<div className="grid gap-4">
					<div className="flex flex-wrap items-center gap-2">
						<ChartTimeSelect
							className="bg-card flex-1 min-w-0 basis-full sm:basis-0"
							agentVersion={chartData.agentVersion}
							chartTimeStore={chartTimeStore}
							allowRealtime={false}
						/>
						<MonitorMultiSelect
							id="monitor-compare-targets"
							className="flex-1 min-w-0 basis-full sm:basis-0 bg-card"
							monitors={compare.targetOptions}
							selectedMonitorIds={compare.selectedTargetIds}
							onChange={setCompareTargetIds}
							disabled={compare.targetOptions.length === 0}
							canSelectMore={compare.canAddTarget}
							placeholder={t`Compare with other targets`}
						/>
						<SystemMultiSelect
							id="monitor-compare-systems"
							className="flex-1 min-w-0 basis-full sm:basis-0 bg-card"
							systemIds={compare.systemOptions}
							selectedSystemIds={compare.selectedSystemIds}
							onChange={setCompareSystemIds}
							disabled={compare.systemOptions.length === 0}
							canSelectMore={compare.canAddSystem}
							placeholder={t`Compare with other systems`}
						/>
					</div>
					{comparing ? (
						<>
							<ResponseChart
								monitorStats={monitorStats}
								grid={false}
								monitors={compareMonitors}
								chartData={chartData}
								empty={!hasMonitorStats}
								getLabel={compare.getLabel}
								filterStore={compareFilterStore}
							/>
							<LossChart
								monitorStats={monitorStats}
								grid={false}
								monitors={compareMonitors}
								chartData={chartData}
								empty={!hasMonitorStats}
								getLabel={compare.getLabel}
								filterStore={compareFilterStore}
							/>
						</>
					) : (
						<>
							<AvgMinMaxResponseChart
								monitorStats={monitorStats}
								monitor={monitor}
								chartData={chartData}
								empty={!hasMonitorStats}
							/>
							<LossChart
								monitorStats={monitorStats}
								grid={false}
								monitors={[monitor]}
								chartData={chartData}
								empty={!hasMonitorStats}
								showFilter={false}
							/>
						</>
					)}
				</div>
			</SheetContent>
		</Sheet>
	)
}
