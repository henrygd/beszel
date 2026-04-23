import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	type ColumnFiltersState,
	type ColumnDef,
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
	AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Button, buttonVariants } from "@/components/ui/button"
import { memo, useMemo, useRef, useState } from "react"
import { getProbeColumns } from "@/components/network-probes-table/network-probes-columns"
import { Card, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useToast } from "@/components/ui/use-toast"
import { isReadOnlyUser } from "@/lib/api"
import { pb } from "@/lib/api"
import { $allSystemsById } from "@/lib/stores"
import { cn, getVisualStringWidth, useBrowserStorage } from "@/lib/utils"
import { Trash2Icon } from "lucide-react"
import type { NetworkProbeRecord } from "@/types"
import { AddProbeDialog } from "./probe-dialog"

export default function NetworkProbesTableNew({
	systemId,
	probes,
}: {
	systemId?: string
	probes: NetworkProbeRecord[]
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
	const { toast } = useToast()
	const canManageProbes = !isReadOnlyUser()

	const { longestName, longestTarget } = useMemo(() => {
		let longestName = 0
		let longestTarget = 0
		for (const p of probes) {
			longestName = Math.max(longestName, getVisualStringWidth(p.name || p.target))
			longestTarget = Math.max(longestTarget, getVisualStringWidth(p.target))
		}
		return { longestName, longestTarget }
	}, [probes])

	// Filter columns based on whether systemId is provided
	const columns = useMemo(() => {
		let columns = getProbeColumns(longestName, longestTarget)
		columns = systemId ? columns.filter((col) => col.id !== "system") : columns
		columns = canManageProbes ? columns : columns.filter((col) => col.id !== "actions")
		if (!canManageProbes) {
			return columns
		}

		const selectionColumn: ColumnDef<NetworkProbeRecord> = {
			id: "select",
			header: ({ table }) => (
				<Checkbox
					className="ms-2"
					checked={table.getIsAllRowsSelected() || (table.getIsSomeRowsSelected() && "indeterminate")}
					onCheckedChange={(value) => table.toggleAllRowsSelected(!!value)}
					aria-label={t`Select all`}
				/>
			),
			cell: ({ row }) => (
				<Checkbox
					checked={row.getIsSelected()}
					onCheckedChange={(value) => row.toggleSelected(!!value)}
					aria-label={t`Select row`}
				/>
			),
			enableSorting: false,
			enableHiding: false,
			size: 44,
		}

		return [selectionColumn, ...columns]
	}, [systemId, longestName, longestTarget, canManageProbes])

	const handleBulkDelete = async () => {
		setDeleteOpen(false)
		const selectedIds = Object.keys(rowSelection)
		if (!selectedIds.length) {
			return
		}

		try {
			let batch = pb.createBatch()
			let inBatch = 0
			for (const id of selectedIds) {
				batch.collection("network_probes").delete(id)
				inBatch++
				if (inBatch >= 20) {
					await batch.send()
					batch = pb.createBatch()
					inBatch = 0
				}
			}
			if (inBatch) {
				await batch.send()
			}
			table.resetRowSelection()
		} catch (err: unknown) {
			toast({
				variant: "destructive",
				title: t`Error`,
				description: (err as Error)?.message || t`Failed to delete probes.`,
			})
		}
	}

	const table = useReactTable({
		data: probes,
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
			const probe = row.original
			const systemName = $allSystemsById.get()[probe.system]?.name ?? ""
			const searchString = `${probe.name}${probe.target}${probe.protocol}${systemName}`.toLocaleLowerCase()
			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
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
							<Trans>Network Probes</Trans>
						</CardTitle>
						<div className="text-sm text-muted-foreground flex items-center flex-wrap">
							<Trans>Response time monitoring from agents.</Trans>
						</div>
					</div>
					<div className="md:ms-auto flex items-center gap-2">
						{canManageProbes && table.getFilteredSelectedRowModel().rows.length > 0 && (
							<div className="fixed bottom-0 left-0 w-full p-4 grid grid-cols-1 items-center gap-4 z-50 backdrop-blur-md shrink-0 md:static md:p-0 md:w-auto md:gap-3">
								<AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
									<AlertDialogTrigger asChild>
										<Button variant="destructive" className="h-9 shrink-0">
											<Trash2Icon className="size-4 shrink-0" />
											<span className="ms-1">
												<Trans>Delete</Trans>
											</span>
										</Button>
									</AlertDialogTrigger>
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
						)}
						{probes.length > 0 && (
							<Input
								placeholder={t`Filter...`}
								value={globalFilter}
								onChange={(e) => setGlobalFilter(e.target.value)}
								className="ms-auto px-4 w-full max-w-full md:w-50"
							/>
						)}
						{canManageProbes ? <AddProbeDialog systemId={systemId} /> : null}
					</div>
				</div>
			</CardHeader>
			<div className="rounded-md">
				<NetworkProbesTable table={table} rows={rows} colLength={visibleColumns.length} rowSelection={rowSelection} />
			</div>
		</Card>
	)
}

const NetworkProbesTable = memo(function NetworkProbeTable({
	table,
	rows,
	colLength,
	rowSelection: _rowSelection,
}: {
	table: TableType<NetworkProbeRecord>
	rows: Row<NetworkProbeRecord>[]
	colLength: number
	rowSelection: RowSelectionState
}) {
	// The virtualizer will need a reference to the scrollable container element
	const scrollRef = useRef<HTMLDivElement>(null)

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
				// don't set min height if there are less than 2 rows, do set if we need to display the empty state
				(!rows.length || rows.length > 2) && "min-h-50"
			)}
			ref={scrollRef}
		>
			{/* add header height to table size */}
			<div style={{ height: `${virtualizer.getTotalSize() + 48}px`, paddingTop, paddingBottom }}>
				<table className="text-sm w-full h-full text-nowrap">
					<NetworkProbeTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return (
									<NetworkProbeTableRow
										key={row.id}
										row={row}
										virtualRow={virtualRow}
										isSelected={row.getIsSelected()}
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
		</div>
	)
})

function NetworkProbeTableHead({ table }: { table: TableType<NetworkProbeRecord> }) {
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

const NetworkProbeTableRow = memo(function NetworkProbeTableRow({
	row,
	virtualRow,
	isSelected,
}: {
	row: Row<NetworkProbeRecord>
	virtualRow: VirtualItem
	isSelected: boolean
}) {
	return (
		<TableRow data-state={isSelected && "selected"} className="transition-opacity">
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
