import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	type ColumnFiltersState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type Row,
	type RowSelectionState,
	type SortingState,
	type Table as TableType,
	useReactTable,
	type VisibilityState,
} from "@tanstack/react-table"
import { useVirtualizer, type VirtualItem } from "@tanstack/react-virtual"
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
import { memo, useCallback, useMemo, useRef, useState } from "react"
import { getMonitorColumns } from "@/components/network-monitors-table/network-monitors-columns"
import { Card, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useToast } from "@/components/ui/use-toast"
import { isReadOnlyUser } from "@/lib/api"
import { pb } from "@/lib/api"
import { $allSystemsById, $direction, $userSettings } from "@/lib/stores"
import {
	cn,
	isVisuallyLonger,
	matchesFilterGroups,
	parseFilterGroups,
	parseSemVer,
	useBrowserStorage,
} from "@/lib/utils"
import type { ChartData, NetworkMonitorRecord } from "@/types"
import { AddMonitorDialog, EditMonitorDialog } from "./monitor-dialog"
import { ArrowLeftRightIcon, EthernetPortIcon, GlobeIcon, ServerIcon, XIcon } from "lucide-react"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { LossChart, AvgMinMaxResponseChart } from "@/components/routes/system/charts/monitors-charts"
import { useNetworkMonitorStats } from "@/lib/use-network-monitors"
import { useStore } from "@nanostores/react"
import { atom } from "nanostores"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { getPagePath } from "@nanostores/router"

export default function NetworkMonitorsTableNew({
	systemId,
	monitors,
}: {
	systemId?: string
	monitors: NetworkMonitorRecord[]
}) {
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		`sort-np-${systemId ? 1 : 0}`,
		[{ id: systemId ? "name" : "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
	const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
	const [globalFilter, setGlobalFilter] = useState("")
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [pendingDeleteIds, setPendingDeleteIds] = useState<string[]>([])
	const [editingMonitor, setEditingMonitor] = useState<NetworkMonitorRecord>()

	const { toast } = useToast()
	const canManageMonitors = !isReadOnlyUser()

	const [longestName, longestTarget] = useMemo(() => {
		let longestName = ""
		let longestTarget = ""
		for (const p of monitors) {
			const name = p.name || p.target
			if (isVisuallyLonger(name, longestName)) {
				longestName = name
			}
			if (isVisuallyLonger(p.target, longestTarget)) {
				longestTarget = p.target
			}
		}
		return [longestName, longestTarget]
	}, [monitors])

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

	const handleDeleteRequest = useCallback(
		async (monitorsToDelete: NetworkMonitorRecord[]) => {
			if (!monitorsToDelete.length) {
				return
			}

			const ids = monitorsToDelete.map((monitor) => monitor.id)
			if (ids.length === 1) {
				try {
					await pb.collection("network_monitors").delete(ids[0])
				} catch (err: unknown) {
					toast({
						variant: "destructive",
						title: t`Error`,
						description: (err as Error)?.message || t`Failed to delete monitors.`,
					})
				}
				return
			}

			setPendingDeleteIds(ids)
			setDeleteOpen(true)
		},
		[toast]
	)

	const handleBulkDelete = async () => {
		setDeleteOpen(false)
		if (!pendingDeleteIds.length) {
			return
		}

		try {
			await runMonitorBatch(pendingDeleteIds, (batch, id) => batch.collection("network_monitors").delete(id))
			setPendingDeleteIds([])
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
		let columns = getMonitorColumns(longestName, longestTarget, {
			onEdit: setEditingMonitor,
			onDelete: handleDeleteRequest,
			onSetEnabled: handleSetEnabled,
		})
		columns = systemId ? columns.filter((col) => col.id !== "system") : columns
		columns = canManageMonitors ? columns : columns.filter((col) => col.id !== "actions")
		return columns
	}, [canManageMonitors, handleDeleteRequest, handleSetEnabled, longestName, systemId, longestTarget])

	const table = useReactTable({
		data: monitors,
		columns,
		getRowId: (row) => row.id,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		onColumnVisibilityChange: setColumnVisibility,
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
		},
		onGlobalFilterChange: setGlobalFilter,
		globalFilterFn: (row, _columnId, filterValue) => {
			const value = (filterValue as string).trim()
			if (!value) return true
			const monitor = row.original
			const systemName = $allSystemsById.get()[monitor.system]?.name ?? ""
			const searchString = `${monitor.name}${monitor.target}${monitor.protocol}${systemName}`.toLocaleLowerCase()
			return matchesFilterGroups(searchString, parseFilterGroups(value))
		},
	})

	const rows = table.getRowModel().rows
	const visibleColumns = table.getVisibleLeafColumns()

	return (
		<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 mb-3 sm:mb-4">
				<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>Network Monitors</Trans>
						</CardTitle>
						<div className="text-sm text-muted-foreground flex items-center flex-wrap">
							<Trans>Response time monitoring from agents.</Trans>
						</div>
					</div>
					<div className="md:ms-auto flex items-center gap-2">
						{monitors.length > 0 && (
							<div className="relative">
								<Input
									placeholder={t`Filter...`}
									title={t`Use commas to match any of multiple terms, e.g. "system1, system2"`}
									value={globalFilter}
									onChange={(e) => setGlobalFilter(e.target.value)}
									className="ms-auto px-4 w-full max-w-full md:w-50"
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
						)}
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
									setPendingDeleteIds([])
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
										onClick={handleBulkDelete}
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
				<NetworkMonitorsTable table={table} rows={rows} colLength={visibleColumns.length} rowSelection={rowSelection} />
			</div>
		</Card>
	)
}

const NetworkMonitorsTable = memo(function NetworkMonitorTable({
	table,
	rows,
	colLength,
	rowSelection: _rowSelection,
}: {
	table: TableType<NetworkMonitorRecord>
	rows: Row<NetworkMonitorRecord>[]
	colLength: number
	rowSelection: RowSelectionState
}) {
	const scrollRef = useRef<HTMLDivElement>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const [activeMonitorId, setActiveMonitorId] = useState<string | null>(null)
	const activeMonitor = activeMonitorId
		? table.options.data.find((monitor) => monitor.id === activeMonitorId)
		: undefined
	const openSheet = useCallback((monitor: NetworkMonitorRecord) => {
		setActiveMonitorId(monitor.id)
		setSheetOpen(true)
	}, [])

	const virtualizer = useVirtualizer<HTMLDivElement, HTMLTableRowElement>({
		count: rows.length,
		estimateSize: () => 54,
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
				(!rows.length || rows.length > 2) && "min-h-50"
			)}
			ref={scrollRef}
		>
			<div style={{ height: `${virtualizer.getTotalSize() + 48}px`, paddingTop, paddingBottom }}>
				<table className="text-sm w-full h-full text-nowrap">
					<NetworkMonitorTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return (
									<NetworkMonitorTableRow
										key={row.id}
										row={row}
										virtualRow={virtualRow}
										isSelected={row.getIsSelected()}
										openSheet={openSheet}
									/>
								)
							})
						) : (
							<TableRow>
								<TableCell colSpan={colLength} className="h-37 text-center pointer-events-none">
									<Trans>No results.</Trans>
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</table>
			</div>
			<NetworkMonitorSheet
				open={sheetOpen}
				onOpenChange={(nextOpen) => {
					setSheetOpen(nextOpen)
				}}
				monitor={activeMonitor}
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
	virtualRow,
	isSelected,
	openSheet,
}: {
	row: Row<NetworkMonitorRecord>
	virtualRow: VirtualItem
	isSelected: boolean
	openSheet: (monitor: NetworkMonitorRecord) => void
}) {
	return (
		<TableRow
			data-state={isSelected && "selected"}
			className="cursor-pointer transition-opacity"
			onClick={() => openSheet(row.original)}
		>
			{row.getVisibleCells().map((cell) => (
				<TableCell
					key={cell.id}
					className="py-0"
					style={{
						width: `${cell.column.getSize()}px`,
						height: virtualRow.size,
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
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	monitor?: NetworkMonitorRecord
}) {
	if (!monitor) {
		return null
	}

	return <NetworkMonitorSheetContent key={monitor.system} open={open} onOpenChange={onOpenChange} monitor={monitor} />
}

function NetworkMonitorSheetContent({
	open,
	onOpenChange,
	monitor,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	monitor: NetworkMonitorRecord
}) {
	// Keep monitor exploration independent of the system charts' time range.
	const [chartTimeStore] = useState(() => {
		const defaultTime = $userSettings.get().chartTime
		return atom(defaultTime === "1m" ? "1h" : defaultTime)
	})
	const chartTime = useStore(chartTimeStore)
	const direction = useStore($direction)
	const system = useStore($allSystemsById)[monitor.system]

	const monitorStats = useNetworkMonitorStats({ systemId: monitor.system, chartTime })

	const chartData = useMemo<ChartData>(
		() => ({
			agentVersion: parseSemVer(system?.info?.v),
			orientation: direction === "rtl" ? "right" : "left",
			chartTime,
		}),
		[system?.info?.v, direction, chartTime]
	)
	const hasMonitorStats = monitorStats.some((record) => record.stats?.[monitor.id] != null)
	const monitorLabel = monitor.name || monitor.target

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
						<ArrowLeftRightIcon className="size-3.5 text-muted-foreground" />
						{monitor.protocol.toUpperCase()}
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						<GlobeIcon className="size-3.5 text-muted-foreground" />
						{monitor.target}
						{monitor.protocol === "tcp" && monitor.port > 0 && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<EthernetPortIcon className="size-3.5 text-muted-foreground" />
								<span>{monitor.port}</span>
							</>
						)}
					</SheetDescription>
				</SheetHeader>
				<div className="grid gap-4">
					<ChartTimeSelect
						className="bg-card"
						agentVersion={chartData.agentVersion}
						chartTimeStore={chartTimeStore}
						allowRealtime={false}
					/>
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
				</div>
			</SheetContent>
		</Sheet>
	)
}
