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
import { LoaderCircleIcon } from "lucide-react"
import { listenKeys } from "nanostores"
import { memo, type ReactNode, useEffect, useMemo, useRef, useState } from "react"
import { getStatusColor, systemdTableCols } from "@/components/systemd-table/systemd-table-columns"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { pb } from "@/lib/api"
import { ServiceStatus, ServiceStatusLabels, type ServiceSubState, ServiceSubStateLabels } from "@/lib/enums"
import { $allSystemsById } from "@/lib/stores"
import { cn, decimalString, formatBytes, useBrowserStorage } from "@/lib/utils"
import type { SystemdRecord, SystemdServiceDetails } from "@/types"
import { Separator } from "../ui/separator"

export default function SystemdTable({ systemId }: { systemId?: string }) {
	const loadTime = Date.now()
	const [data, setData] = useState<SystemdRecord[]>([])
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		`sort-sd-${systemId ? 1 : 0}`,
		[{ id: systemId ? "name" : "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
	const [globalFilter, setGlobalFilter] = useState("")

	// clear old data when systemId changes
	useEffect(() => {
		return setData([])
	}, [systemId])


	useEffect(() => {
		const lastUpdated = data[0]?.updated ?? 0

		function fetchData(systemId?: string) {
			pb.collection<SystemdRecord>("systemd_services")
				.getList(0, 2000, {
					fields: "name,state,sub,cpu,cpuPeak,memory,memPeak,updated",
					filter: systemId ? pb.filter("system={:system}", { system: systemId }) : undefined,
				})
				.then(
					({ items }) =>
						items.length &&
						setData((curItems) => {
							const lastUpdated = Math.max(items[0].updated, items.at(-1)?.updated ?? 0)
							const systemdNames = new Set()
							const newItems: SystemdRecord[] = []
							for (const item of items) {
								if (Math.abs(lastUpdated - item.updated) < 70_000) {
									systemdNames.add(item.name)
									newItems.push(item)
								}
							}
							for (const item of curItems) {
								if (!systemdNames.has(item.name) && lastUpdated - item.updated < 70_000) {
									newItems.push(item)
								}
							}
							return newItems
						})
				)
		}

		// initial load
		fetchData(systemId)

		// if no systemId, pull system containers after every system update
		if (!systemId) {
			return $allSystemsById.listen((_value, _oldValue, systemId) => {
				// exclude initial load of systems
				if (Date.now() - loadTime > 500) {
					fetchData(systemId)
				}
			})
		}

		// if systemId, fetch containers after the system is updated
		return listenKeys($allSystemsById, [systemId], (_newSystems) => {
			// don't fetch data if the last update is less than 9.5 minutes
			if (lastUpdated > Date.now() - 9.5 * 60 * 1000) {
				return
			}
			fetchData(systemId)
		})
	}, [systemId])

	const table = useReactTable({
		data,
		// columns: systemdTableCols.filter((col) => (systemId ? col.id !== "system" : true)),
		columns: systemdTableCols,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		onColumnVisibilityChange: setColumnVisibility,
		defaultColumn: {
			sortUndefined: "last",
			size: 100,
			minSize: 0,
		},
		state: {
			sorting,
			columnFilters,
			columnVisibility,
			globalFilter,
		},
		onGlobalFilterChange: setGlobalFilter,
		globalFilterFn: (row, _columnId, filterValue) => {
			const service = row.original
			const systemName = $allSystemsById.get()[service.system]?.name ?? ""
			const name = service.name ?? ""
			const statusLabel = ServiceStatusLabels[service.state as ServiceStatus] ?? ""
			const subState = service.sub ?? ""
			const searchString = `${systemName} ${name} ${statusLabel} ${subState}`.toLowerCase()

			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})

	const rows = table.getRowModel().rows
	const visibleColumns = table.getVisibleLeafColumns()

	const statusTotals = useMemo(() => {
		const totals = [0, 0, 0, 0, 0, 0]
		for (const service of data) {
			totals[service.state]++
		}
		return totals
	}, [data])

	if (!data.length && !globalFilter) {
		return null
	}

	return (
		<Card className="p-6 @container w-full">
			<CardHeader className="p-0 mb-4">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>Systemd Services</Trans>
						</CardTitle>
						<CardDescription className="flex items-center">
							<Trans>Total: {data.length}</Trans>
							<Separator orientation="vertical" className="h-4 mx-2 bg-primary/40" />
							<Trans>Failed: {statusTotals[ServiceStatus.Failed]}</Trans>
							<Separator orientation="vertical" className="h-4 mx-2 bg-primary/40" />
							<Trans>Updated every 10 minutes.</Trans>
						</CardDescription>
					</div>
					<Input
						placeholder={t`Filter...`}
						value={globalFilter}
						onChange={(e) => setGlobalFilter(e.target.value)}
						className="ms-auto px-4 w-full max-w-full md:w-64"
					/>
				</div>
			</CardHeader>
			<div className="rounded-md">
				<AllSystemdTable table={table} rows={rows} colLength={visibleColumns.length} systemId={systemId} />
			</div>
		</Card>
	)
}

const AllSystemdTable = memo(function AllSystemdTable({
	table,
	rows,
	colLength,
	systemId,
}: {
	table: TableType<SystemdRecord>
	rows: Row<SystemdRecord>[]
	colLength: number
	systemId?: string
}) {
	// The virtualizer will need a reference to the scrollable container element
	const scrollRef = useRef<HTMLDivElement>(null)
	const activeService = useRef<SystemdRecord | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const openSheet = (service: SystemdRecord) => {
		activeService.current = service
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
				// don't set min height if there are less than 2 rows, do set if we need to display the empty state
				(!rows.length || rows.length > 2) && "min-h-50"
			)}
			ref={scrollRef}
		>
			{/* add header height to table size */}
			<div style={{ height: `${virtualizer.getTotalSize() + 48}px`, paddingTop, paddingBottom }}>
				<table className="text-sm w-full h-full text-nowrap">
					<SystemdTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return <SystemdTableRow key={row.id} row={row} virtualRow={virtualRow} openSheet={openSheet} />
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
			<SystemdSheet
				sheetOpen={sheetOpen}
				setSheetOpen={setSheetOpen}
				activeService={activeService}
				systemId={systemId}
			/>
		</div>
	)
})

function SystemdSheet({
	sheetOpen,
	setSheetOpen,
	activeService,
	systemId,
}: {
	sheetOpen: boolean
	setSheetOpen: (open: boolean) => void
	activeService: React.RefObject<SystemdRecord | null>
	systemId?: string
}) {
	const service = activeService.current
	const [details, setDetails] = useState<SystemdServiceDetails | null>(null)
	const [isLoading, setIsLoading] = useState(false)
	const [error, setError] = useState<string | null>(null)

	useEffect(() => {
		if (!sheetOpen || !service) {
			return
		}

		setError(null)

		let cancelled = false
		setDetails(null)
		setIsLoading(true)

		pb.send<{ details: SystemdServiceDetails }>("/api/beszel/systemd/info", {
			query: {
				system: systemId,
				service: service.name,
			},
		})
			.then(({ details }) => {
				if (cancelled) return
				if (details) {
					setDetails(details)
				} else {
					setDetails(null)
					setError(t`No systemd details returned`)
				}
			})
			.catch((err) => {
				if (cancelled) return
				setError(err?.message ?? "Failed to load service details")
				setDetails(null)
			})
			.finally(() => {
				if (!cancelled) {
					setIsLoading(false)
				}
			})

		return () => {
			cancelled = true
		}
	}, [sheetOpen, service, systemId])

	if (!service) return null

	const statusLabel = ServiceStatusLabels[service.state as ServiceStatus] ?? ""
	const subStateLabel = ServiceSubStateLabels[service.sub as ServiceSubState] ?? ""

	const notAvailable = <span className="text-muted-foreground">N/A</span>

	const formatMemory = (value?: number | null) => {
		if (value === undefined || value === null) {
			return value === null ? t`Unlimited` : undefined
		}
		const { value: convertedValue, unit } = formatBytes(value, false, undefined, false)
		const digits = convertedValue >= 10 ? 1 : 2
		return `${decimalString(convertedValue, digits)} ${unit}`
	}

	const formatCpuTime = (ns?: number) => {
		if (!ns) return undefined
		const seconds = ns / 1_000_000_000
		if (seconds >= 3600) {
			const hours = Math.floor(seconds / 3600)
			const minutes = Math.floor((seconds % 3600) / 60)
			const secs = Math.floor(seconds % 60)
			return [hours ? `${hours}h` : null, minutes ? `${minutes}m` : null, secs ? `${secs}s` : null]
				.filter(Boolean)
				.join(" ")
		}
		if (seconds >= 60) {
			const minutes = Math.floor(seconds / 60)
			const secs = Math.floor(seconds % 60)
			return `${minutes}m ${secs}s`
		}
		if (seconds >= 1) {
			return `${decimalString(seconds, 2)}s`
		}
		return `${decimalString(seconds * 1000, 2)}ms`
	}

	const formatTasks = (current?: number, max?: number) => {
		const hasCurrent = typeof current === "number" && current >= 0
		const hasMax = typeof max === "number" && max > 0 && max !== null
		if (!hasCurrent && !hasMax) {
			return undefined
		}
		return (
			<>
				{hasCurrent ? current : notAvailable}
				{hasMax && (
					<span className="text-muted-foreground ms-1.5">
						{t`(limit: ${max})`}
					</span>
				)}
				{max === null && (
					<span className="text-muted-foreground ms-1.5">
						{t`(limit: unlimited)`}
					</span>
				)}
			</>
		)
	}

	const formatTimestamp = (timestamp?: number) => {
		if (!timestamp) return undefined
		// systemd timestamps are in microseconds, convert to milliseconds for JavaScript Date
		const date = new Date(timestamp / 1000)
		if (Number.isNaN(date.getTime())) return undefined
		return date.toLocaleString()
	}

	const activeStateValue = (() => {
		const stateText = details?.ActiveState
			? details.SubState
				? `${details.ActiveState} (${details.SubState})`
				: details.ActiveState
			: subStateLabel
				? `${statusLabel} (${subStateLabel})`
				: statusLabel

		for (const [index, status] of ServiceStatusLabels.entries()) {
			if (details?.ActiveState?.toLowerCase() === status.toLowerCase()) {
				service.state = index as ServiceStatus
				break
			}
		}

		return (
			<div className="flex items-center gap-2">
				<div className={cn("w-2 h-2 rounded-full flex-shrink-0", getStatusColor(service.state))} />
				{stateText}
			</div>
		)
	})()

	const statusTextValue = details?.Result

	const cpuTime = formatCpuTime(details?.CPUUsageNSec)
	const tasks = formatTasks(details?.TasksCurrent, details?.TasksMax)
	const memoryCurrent = formatMemory(details?.MemoryCurrent)
	const memoryPeak = formatMemory(details?.MemoryPeak)
	const memoryLimit = formatMemory(details?.MemoryLimit)
	const restartsValue = typeof details?.NRestarts === "number" ? details.NRestarts : undefined
	const mainPidValue = typeof details?.MainPID === "number" && details.MainPID > 0 ? details.MainPID : undefined
	const execMainPidValue =
		typeof details?.ExecMainPID === "number" && details.ExecMainPID > 0 && details.ExecMainPID !== details?.MainPID
			? details.ExecMainPID
			: undefined
	const activeEnterTimestamp = formatTimestamp(details?.ActiveEnterTimestamp)
	const activeExitTimestamp = formatTimestamp(details?.ActiveExitTimestamp)
	const inactiveEnterTimestamp = formatTimestamp(details?.InactiveEnterTimestamp)
	const execMainStartTimestamp = undefined // Property not available in current systemd interface

	const renderRow = (key: string, label: ReactNode, value?: ReactNode, alwaysShow = false) => {
		if (!alwaysShow && (value === undefined || value === null || value === "")) {
			return null
		}
		return (
			<tr key={key} className="border-b last:border-b-0">
				<td className="px-3 py-2 font-medium bg-muted dark:bg-muted/40 align-top w-35">{label}</td>
				<td className="px-3 py-2">{value ?? notAvailable}</td>
			</tr>
		)
	}

	return (
		<Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
			<SheetContent className="w-full sm:max-w-220 p-6 overflow-y-auto">
				<SheetHeader className="p-0">
					<SheetTitle>
						<Trans>Service Details</Trans>
					</SheetTitle>
				</SheetHeader>
				<div className="grid gap-6">
					{isLoading && (
						<div className="flex items-center gap-2 text-sm text-muted-foreground">
							<LoaderCircleIcon className="size-4 animate-spin" />
							<Trans>Loading...</Trans>
						</div>
					)}
					{error && (
						<Alert className="border-destructive/50 text-destructive dark:border-destructive/60 dark:text-destructive">
							<AlertTitle>
								<Trans>Error</Trans>
							</AlertTitle>
							<AlertDescription>{error}</AlertDescription>
						</Alert>
					)}

					<div>
						<div className="border rounded-md">
							<table className="w-full text-sm">
								<tbody>
									{renderRow("name", t`Name`, service.name, true)}
									{renderRow("description", t`Description`, details?.Description, true)}
									{renderRow("loadState", t`Load State`, details?.LoadState, true)}
									{renderRow(
										"bootState",
										t`Boot State`,
										<div className="flex items-center">
											{details?.UnitFileState}
											{details?.UnitFilePreset && (
												<span className="text-muted-foreground ms-1.5">(preset: {details?.UnitFilePreset})</span>
											)}
										</div>,
										true
									)}
									{renderRow("unitFile", t`Unit File`, details?.FragmentPath, true)}
									{renderRow("active", t`Active State`, activeStateValue, true)}
									{renderRow("status", t`Status`, statusTextValue, true)}
									{renderRow(
										"documentation",
										t`Documentation`,
										Array.isArray(details?.Documentation) && details.Documentation.length > 0
											? details.Documentation.join(", ")
											: undefined
									)}
								</tbody>
							</table>
						</div>
					</div>

					<div>
						<h3 className="text-sm font-medium mb-3">
							<Trans>Runtime Metrics</Trans>
						</h3>
						<div className="border rounded-md">
							<table className="w-full text-sm">
								<tbody>
									{renderRow("mainPid", t`Main PID`, mainPidValue, true)}
									{renderRow("execMainPid", t`Exec Main PID`, execMainPidValue)}
									{renderRow("tasks", t`Tasks`, tasks, true)}
									{renderRow("cpuTime", t`CPU Time`, cpuTime)}
									{renderRow("memory", t`Memory`, memoryCurrent, true)}
									{renderRow("memoryPeak", t`Memory Peak`, memoryPeak)}
									{renderRow("memoryLimit", t`Memory Limit`, memoryLimit)}
									{renderRow("restarts", t`Restarts`, restartsValue, true)}
								</tbody>
							</table>
						</div>
					</div>

					<div className="hidden has-[tr]:block">
						<h3 className="text-sm font-medium mb-3">
							<Trans>Relationships</Trans>
						</h3>
						<div className="border rounded-md">
							<table className="w-full text-sm">
								<tbody>
									{renderRow(
										"wants",
										t`Wants`,
										Array.isArray(details?.Wants) && details.Wants.length > 0 ? details.Wants.join(", ") : undefined
									)}
									{renderRow(
										"requires",
										t`Requires`,
										Array.isArray(details?.Requires) && details.Requires.length > 0
											? details.Requires.join(", ")
											: undefined
									)}
									{renderRow(
										"requiredBy",
										t`Required By`,
										Array.isArray(details?.RequiredBy) && details.RequiredBy.length > 0
											? details.RequiredBy.join(", ")
											: undefined
									)}
									{renderRow(
										"conflicts",
										t`Conflicts`,
										Array.isArray(details?.Conflicts) && details.Conflicts.length > 0
											? details.Conflicts.join(", ")
											: undefined
									)}
									{renderRow(
										"before",
										t`Before`,
										Array.isArray(details?.Before) && details.Before.length > 0 ? details.Before.join(", ") : undefined
									)}
									{renderRow(
										"after",
										t`After`,
										Array.isArray(details?.After) && details.After.length > 0 ? details.After.join(", ") : undefined
									)}
									{renderRow(
										"triggers",
										t`Triggers`,
										Array.isArray(details?.Triggers) && details.Triggers.length > 0
											? details.Triggers.join(", ")
											: undefined
									)}
									{renderRow(
										"triggeredBy",
										t`Triggered By`,
										Array.isArray(details?.TriggeredBy) && details.TriggeredBy.length > 0
											? details.TriggeredBy.join(", ")
											: undefined
									)}
								</tbody>
							</table>
						</div>
					</div>

					<div className="hidden has-[tr]:block">
						<h3 className="text-sm font-medium mb-3">
							<Trans>Lifecycle</Trans>
						</h3>
						<div className="border rounded-md">
							<table className="w-full text-sm">
								<tbody>
									{renderRow("activeSince", t`Became Active`, activeEnterTimestamp)}
									{service.state !== ServiceStatus.Active &&
										renderRow("lastActive", t`Exited Active`, activeExitTimestamp)}
									{renderRow("inactiveSince", t`Became Inactive`, inactiveEnterTimestamp)}
									{renderRow("execMainStart", t`Process Started`, execMainStartTimestamp)}
									{/* {renderRow("invocationId", t`Invocation ID`, details?.InvocationID)} */}
									{/* {renderRow("freezerState", t`Freezer State`, details?.FreezerState)} */}
								</tbody>
							</table>
						</div>
					</div>

					<div className="hidden has-[tr]:block">
						<h3 className="text-sm font-medium mb-3">
							<Trans>Capabilities</Trans>
						</h3>
						<div className="border rounded-md">
							<table className="w-full text-sm">
								<tbody>
									{renderRow("canStart", t`Can Start`, details?.CanStart ? t`Yes` : t`No`)}
									{renderRow("canStop", t`Can Stop`, details?.CanStop ? t`Yes` : t`No`)}
									{renderRow("canReload", t`Can Reload`, details?.CanReload ? t`Yes` : t`No`)}
									{/* {renderRow("refuseManualStart", t`Refuse Manual Start`, details?.RefuseManualStart ? t`Yes` : t`No`)}
									{renderRow("refuseManualStop", t`Refuse Manual Stop`, details?.RefuseManualStop ? t`Yes` : t`No`)} */}
								</tbody>
							</table>
						</div>
					</div>
				</div>
			</SheetContent>
		</Sheet>
	)
}

function SystemdTableHead({ table }: { table: TableType<SystemdRecord> }) {
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

const SystemdTableRow = memo(function SystemdTableRow({
	row,
	virtualRow,
	openSheet,
}: {
	row: Row<SystemdRecord>
	virtualRow: VirtualItem
	openSheet: (service: SystemdRecord) => void
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
					className="py-0"
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
