import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
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
import { memo, RefObject, useEffect, useRef, useState } from "react"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { pb } from "@/lib/api"
import type { PveVmRecord } from "@/types"
import { pveVmCols, formatUptime } from "@/components/pve-table/pve-table-columns"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { cn, decimalString, formatBytes, useBrowserStorage } from "@/lib/utils"
import { Sheet, SheetTitle, SheetHeader, SheetContent, SheetDescription } from "../ui/sheet"
import { $allSystemsById } from "@/lib/stores"
import { LoaderCircleIcon, XIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { listenKeys } from "nanostores"
import { getPagePath } from "@nanostores/router"

export default function PveTable({ systemId }: { systemId?: string }) {
	const loadTime = Date.now()
	const [data, setData] = useState<PveVmRecord[] | undefined>(undefined)
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		`sort-pve-${systemId ? 1 : 0}`,
		[{ id: systemId ? "name" : "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
	const [rowSelection, setRowSelection] = useState({})
	const [globalFilter, setGlobalFilter] = useState("")

	useEffect(() => {
		function fetchData(systemId?: string) {
			pb.collection<PveVmRecord>("pve_vms")
				.getList(0, 2000, {
					fields: "id,name,type,cpu,mem,net,maxcpu,maxmem,uptime,system,updated",
					filter: systemId ? pb.filter("system={:system}", { system: systemId }) : undefined,
				})
				.then(({ items }) => {
					if (items.length === 0) {
						setData((curItems) => {
							if (systemId) {
								return curItems?.filter((item) => item.system !== systemId) ?? []
							}
							return []
						})
						return
					}
					setData((curItems) => {
						const lastUpdated = Math.max(items[0].updated, items.at(-1)?.updated ?? 0)
						const vmIds = new Set<string>()
						const newItems: PveVmRecord[] = []
						for (const item of items) {
							if (Math.abs(lastUpdated - item.updated) < 70_000) {
								vmIds.add(item.id)
								newItems.push(item)
							}
						}
						for (const item of curItems ?? []) {
							if (!vmIds.has(item.id) && lastUpdated - item.updated < 70_000) {
								newItems.push(item)
							}
						}
						return newItems
					})
				})
		}

		// initial load
		fetchData(systemId)

		// if no systemId, pull pve vms after every system update
		if (!systemId) {
			return $allSystemsById.listen((_value, _oldValue, systemId) => {
				// exclude initial load of systems
				if (Date.now() - loadTime > 500) {
					fetchData(systemId)
				}
			})
		}

		// if systemId, fetch pve vms after the system is updated
		return listenKeys($allSystemsById, [systemId], (_newSystems) => {
			fetchData(systemId)
		})
	}, [])

	const table = useReactTable({
		data: data ?? [],
		columns: pveVmCols.filter((col) => (systemId ? col.id !== "system" : true)),
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		onColumnVisibilityChange: setColumnVisibility,
		onRowSelectionChange: setRowSelection,
		defaultColumn: {
			sortUndefined: "last",
			size: 100,
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
			const vm = row.original
			const systemName = $allSystemsById.get()[vm.system]?.name ?? ""
			const id = vm.id ?? ""
			const name = vm.name ?? ""
			const type = vm.type ?? ""
			const searchString = `${systemName} ${id} ${name} ${type}`.toLowerCase()

			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})

	const rows = table.getRowModel().rows
	const visibleColumns = table.getVisibleLeafColumns()

	return (
		<Card className="p-6 @container w-full">
			<CardHeader className="p-0 mb-4">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>All Proxmox VMs</Trans>
						</CardTitle>
						<CardDescription className="flex">
							<Trans>CPU is percent of overall host CPU usage.</Trans>
						</CardDescription>
					</div>
					<div className="relative ms-auto w-full max-w-full md:w-64">
						<Input
							placeholder={t`Filter...`}
							value={globalFilter}
							onChange={(e) => setGlobalFilter(e.target.value)}
							className="ps-4 pe-10 w-full"
						/>
						{globalFilter && (
							<button
								type="button"
								aria-label={t`Clear`}
								className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 flex items-center justify-center text-muted-foreground hover:text-foreground"
								onClick={() => setGlobalFilter("")}
							>
								<XIcon className="h-4 w-4" />
							</button>
						)}
					</div>
				</div>
			</CardHeader>
			<div className="rounded-md">
				<AllPveTable table={table} rows={rows} colLength={visibleColumns.length} data={data} />
			</div>
		</Card>
	)
}

const AllPveTable = memo(function AllPveTable({
	table,
	rows,
	colLength,
	data,
}: {
	table: TableType<PveVmRecord>
	rows: Row<PveVmRecord>[]
	colLength: number
	data: PveVmRecord[] | undefined
}) {
	const scrollRef = useRef<HTMLDivElement>(null)
	const activeVm = useRef<PveVmRecord | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const openSheet = (vm: PveVmRecord) => {
		activeVm.current = vm
		setSheetOpen(true)
	}

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
			{/* add header height to table size */}
			<div style={{ height: `${virtualizer.getTotalSize() + 48}px`, paddingTop, paddingBottom }}>
				<table className="text-sm w-full h-full text-nowrap">
					<PveTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return <PveTableRow key={row.id} row={row} virtualRow={virtualRow} openSheet={openSheet} />
							})
						) : (
							<TableRow>
								<TableCell colSpan={colLength} className="h-37 text-center pointer-events-none">
									{data ? (
										<Trans>No results.</Trans>
									) : (
										<LoaderCircleIcon className="animate-spin size-10 opacity-60 mx-auto" />
									)}
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</table>
			</div>
			<PveVmSheet sheetOpen={sheetOpen} setSheetOpen={setSheetOpen} activeVm={activeVm} />
		</div>
	)
})

function PveVmSheet({
	sheetOpen,
	setSheetOpen,
	activeVm,
}: {
	sheetOpen: boolean
	setSheetOpen: (open: boolean) => void
	activeVm: RefObject<PveVmRecord | null>
}) {
	const vm = activeVm.current
	if (!vm) return null

	const memFormatted = formatBytes(vm.mem, false, undefined, true)
	const maxMemFormatted = formatBytes(vm.maxmem, false, undefined, false)
	const netFormatted = formatBytes(vm.net, true, undefined, false)

	return (
		<Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
			<SheetContent className="w-full sm:max-w-120 p-2">
				<SheetHeader>
					<SheetTitle>{vm.name}</SheetTitle>
					<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
						<Link className="hover:underline" href={getPagePath($router, "system", { id: vm.system })}>
							{$allSystemsById.get()[vm.system]?.name ?? ""}
						</Link>
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						{vm.type}
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						<Trans>Up {formatUptime(vm.uptime)}</Trans>
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						{vm.id}
					</SheetDescription>
				</SheetHeader>
				<div className="px-3 pb-3 -mt-2 flex flex-col gap-3">
					<h3 className="text-sm font-medium">
						<Trans>Details</Trans>
					</h3>
					<dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
						<dt className="text-muted-foreground">
							<Trans>CPU Usage</Trans>
						</dt>
						<dd className="tabular-nums">{`${decimalString(vm.cpu, vm.cpu >= 10 ? 1 : 2)}%`}</dd>

						<dt className="text-muted-foreground">
							<Trans>Memory Used</Trans>
						</dt>
						<dd className="tabular-nums">{`${decimalString(memFormatted.value, memFormatted.value >= 10 ? 1 : 2)} ${memFormatted.unit}`}</dd>

						<dt className="text-muted-foreground">
							<Trans>Network</Trans>
						</dt>
						<dd className="tabular-nums">{`${decimalString(netFormatted.value, netFormatted.value >= 10 ? 1 : 2)} ${netFormatted.unit}`}</dd>

						<dt className="text-muted-foreground">
							<Trans>vCPUs</Trans>
						</dt>
						<dd className="tabular-nums">{vm.maxcpu}</dd>

						<dt className="text-muted-foreground">
							<Trans>Max Memory</Trans>
						</dt>
						<dd className="tabular-nums">{`${decimalString(maxMemFormatted.value, maxMemFormatted.value >= 10 ? 1 : 2)} ${maxMemFormatted.unit}`}</dd>

						<dt className="text-muted-foreground">
							<Trans>Uptime</Trans>
						</dt>
						<dd className="tabular-nums">{formatUptime(vm.uptime)}</dd>
					</dl>
				</div>
			</SheetContent>
		</Sheet>
	)
}

function PveTableHead({ table }: { table: TableType<PveVmRecord> }) {
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

const PveTableRow = memo(function PveTableRow({
	row,
	virtualRow,
	openSheet,
}: {
	row: Row<PveVmRecord>
	virtualRow: VirtualItem
	openSheet: (vm: PveVmRecord) => void
}) {
	return (
		<TableRow
			data-state={row.getIsSelected() && "selected"}
			className="cursor-pointer transition-opacity"
			onClick={() => openSheet(row.original)}
		>
			{row.getVisibleCells().map((cell) => (
				<TableCell
					key={cell.id}
					className="py-0 ps-4.5"
					style={{
						height: virtualRow.size,
					}}
				>
					{flexRender(cell.column.columnDef.cell, cell.getContext())}
				</TableCell>
			))}
		</TableRow>
	)
})
