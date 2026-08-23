import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import {
	type ColumnFiltersState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type SortingState,
	useReactTable,
	type VisibilityState,
} from "@tanstack/react-table"
import {
	FilterIcon,
	LayoutGridIcon,
	LayoutListIcon,
	Settings2Icon,
	XIcon,
} from "lucide-react"
import { memo, useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { SystemStatus } from "@/lib/enums"
import { $downMonitors, $monitors, $pausedMonitors, $upMonitors } from "@/lib/stores"
import { cn, useBrowserStorage } from "@/lib/utils"
import type { MonitorRecord } from "@/types"
import { IndicatorDot, MonitorTableColumns } from "./monitors-table-columns"

// biome-ignore lint/suspicious/noExplicitAny: column definitions carry custom `name` property
type AnyColumnDef = any

function columnName(def: AnyColumnDef): React.ReactNode {
	if (typeof def?.name === "function") {
		return def.name()
	}
	return def?.name
}

type ViewMode = "table" | "grid"
type StatusFilter = "all" | MonitorRecord["status"]

export default memo(function MonitorsTable() {
	const data = useStore($monitors)
	const downMonitors = $downMonitors.get()
	const upMonitors = $upMonitors.get()
	const pausedMonitors = $pausedMonitors.get()
	const { t } = useLingui()
	const [filter, setFilter] = useState<string>("")
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
	const [sorting, setSorting] = useBrowserStorage<SortingState>("monitor-sort", [{ id: "monitor", desc: false }], sessionStorage)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useBrowserStorage<VisibilityState>("monitor-cols", {})

	// Filter data based on status filter
	const filteredData = useMemo(() => {
		if (statusFilter === "all") {
			return data
		}
		if (statusFilter === SystemStatus.Up) {
			return Object.values(upMonitors) ?? []
		}
		if (statusFilter === SystemStatus.Down) {
			return Object.values(downMonitors) ?? []
		}
		return Object.values(pausedMonitors) ?? []
	}, [data, statusFilter])

	const [viewMode, setViewMode] = useBrowserStorage<ViewMode>("monitor-viewMode", "table")

	useEffect(() => {
		if (filter !== undefined) {
			table.getColumn("monitor")?.setFilterValue(filter)
		}
	}, [filter])

	const columnDefs = useMemo(() => MonitorTableColumns(viewMode), [viewMode])

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
		},
	})

	const rows = table.getRowModel().rows
	const columns = table.getAllColumns()

	const [upMonitorsLength, downMonitorsLength, pausedMonitorsLength] = useMemo(() => {
		return [
			Object.values(upMonitors).length,
			Object.values(downMonitors).length,
			Object.values(pausedMonitors).length,
		]
	}, [upMonitors, downMonitors, pausedMonitors])

	if (!data.length) {
		return (
			<div className="flex flex-col items-center justify-center gap-3 my-14 text-muted-foreground">
				<FilterIcon className="size-10 opacity-30" />
				<p>
					<Trans>No monitors configured yet.</Trans>
				</p>
				<p className="text-sm">
					<Trans>Use the Add Monitor button to get started.</Trans>
				</p>
			</div>
		)
	}

	const CardHead = (
		<div className="flex items-center justify-between p-4 mb-3 sm:mb-4">
			<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
				<div className="px-2 sm:px-1">
					<h2 className="text-xl font-semibold mb-1">
						<Trans>Monitors</Trans>
					</h2>
					<p className="text-sm text-muted-foreground">
						<Trans>Click on a monitor to view more information.</Trans>
					</p>
				</div>

				<div className="flex gap-2 ms-auto w-full md:w-80">
					<div className="relative flex-1">
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
							<Button variant="outline">
								<Settings2Icon className="me-1.5 size-4 opacity-80" />
								<Trans>View</Trans>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
							<div className="grid grid-cols-1 md:grid-cols-2 divide-y md:divide-s md:divide-y-0">
								<div className="border-r">
									<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
										<LayoutGridIcon className="size-4" />
										<Trans>Layout</Trans>
									</DropdownMenuLabel>
									<DropdownMenuSeparator />
									<DropdownMenuRadioGroup className="px-1 pb-1" value={viewMode} onValueChange={(view) => setViewMode(view as ViewMode)}>
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
										<Trans>Columns</Trans>
									</DropdownMenuLabel>
									<DropdownMenuSeparator />
									<div className="px-1.5">
									{columns
										.filter((column) => column.getCanHide() && column.id !== "actions")
										.map((column) => (
											<DropdownMenuCheckboxItem
												key={column.id}
												className="gap-2 py-2"
												checked={column.getIsVisible()}
												onCheckedChange={(value) => column.toggleVisibility(!!value)}
											>
												{columnName(column.columnDef)}
											</DropdownMenuCheckboxItem>
										))}
									</div>
								</div>
							</div>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			</div>

			<div className="hidden sm:flex gap-4 text-sm text-muted-foreground">
				<span>
					<Trans>Up</Trans>: <span className="font-medium text-green-500">{upMonitorsLength}</span>
				</span>
				<span>
					<Trans>Down</Trans>: <span className="font-medium text-red-500">{downMonitorsLength}</span>
				</span>
				<span>
					<Trans>Paused</Trans>: <span className="font-medium">{pausedMonitorsLength}</span>
				</span>
			</div>
		</div>
	)

	const StatusFilters = (
		<div className="flex gap-1 p-1 bg-muted rounded-lg mb-3 sm:mb-4 self-start">
			{(["all", SystemStatus.Up, SystemStatus.Down, SystemStatus.Paused] as StatusFilter[]).map((status) => (
				<Button
					key={status}
					variant={statusFilter === status ? "secondary" : "ghost"}
					size="sm"
					className="h-7 px-3 text-xs"
					onClick={() => setStatusFilter(status)}
				>
					{status === "all" ? (
						<Trans>All</Trans>
					) : (
						<>
							{status === SystemStatus.Up && <Trans>Up</Trans>}
							{status === SystemStatus.Down && <Trans>Down</Trans>}
							{status === SystemStatus.Paused && <Trans>Paused</Trans>}
						</>
					)}
				</Button>
			))}
		</div>
	)

	return (
		<div className={cn("p-0 sm:p-6 rounded-md bg-card border border-border/60", "grid gap-3 sm:gap-4")}>
			{CardHead}
			{StatusFilters}

			{viewMode === "table" && (
				<div className="overflow-auto border-t border-border/60 -mx-6 sm:mx-0 sm:px-6 sm:mx-6 max-h-144">
					<table className="w-full text-sm">
						<TableHeader>
							{table.getHeaderGroups().map((headerGroup) => (
								<TableRow key={headerGroup.id} className="border-b-0 h-11">
									{headerGroup.headers.map((header) => (
										<TableHead key={header.id} className="p-0 h-full">
											{header.isPlaceholder ? null : (
												<div className="relative h-full">
													{flexRender(header.column.columnDef.header, header.getContext())}
												</div>
											)}
										</TableHead>
									))}
								</TableRow>
							))}
						</TableHeader>
						<TableBody>
							{rows.map((row) => (
								<TableRow key={row.id} className="h-11 cursor-pointer hover:bg-muted/40 transition-colors">
									{row.getVisibleCells().map((cell) => (
										<TableCell key={cell.id} className="p-0 align-middle h-full">
											<div className="h-full flex items-center px-3">
												{flexRender(cell.column.columnDef.cell, cell.getContext())}
											</div>
										</TableCell>
									))}
								</TableRow>
							))}
						</TableBody>
					</table>
				</div>
			)}

			{viewMode === "grid" && (
				<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3 sm:gap-4">
					{rows.map((row) => {
						const mon = row.original
						const statusLabel =
							mon.status === SystemStatus.Up
								? t`Up`
								: mon.status === SystemStatus.Down
									? t`Down`
									: mon.status === SystemStatus.Paused
										? t`Paused`
										: t`Pending`
						return (
							<GridCard key={row.id} monitor={mon} statusLabel={statusLabel}>
								{row.getVisibleCells().map((cell) => {
									if (cell.column.id === "actions" || cell.column.id === "monitor") return null
									return <div key={cell.id} className="text-sm">
										<span className="text-muted-foreground me-2">
											{columnName(cell.column.columnDef)}
										</span>
										{flexRender(cell.column.columnDef.cell, cell.getContext())}
									</div>
								})}
							</GridCard>
						)
					})}
				</div>
			)}
		</div>
	)
})

function GridCard({ monitor, statusLabel, children }: { monitor: MonitorRecord; statusLabel: string; children: React.ReactNode }) {
	return (
		<div className="p-4 rounded-md border border-border/60 bg-card hover:bg-muted/30 transition-colors flex flex-col gap-2">
			<div className="flex items-center gap-2 font-medium text-sm">
				<IndicatorDot monitor={monitor} />
				<span className="truncate">{monitor.name}</span>
				<span className="ms-auto text-xs text-muted-foreground">{statusLabel}</span>
			</div>
			<div className="flex flex-col gap-1">
				{children}
			</div>
		</div>
	)
}
