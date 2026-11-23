/** biome-ignore-all lint/security/noDangerouslySetInnerHtml: html comes directly from docker via agent */
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
	type Table as TableType,
	useReactTable,
} from "@tanstack/react-table"
import { memo, type RefObject, useEffect, useMemo, useRef, useState } from "react"
import { DataTablePagination, usePagination } from "@/components/ui/data-table-pagination"
import { TableFilterInput, TableSettingsMenu, useTableSettings } from "@/components/ui/data-table-settings"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { pb } from "@/lib/api"
import type { ContainerRecord } from "@/types"
import { containerChartCols } from "@/components/containers-table/containers-table-columns"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { type ContainerHealth, ContainerHealthLabels } from "@/lib/enums"
import { cn, matchesTableFilter } from "@/lib/utils"
import { Sheet, SheetTitle, SheetHeader, SheetContent, SheetDescription } from "../ui/sheet"
import { Dialog, DialogContent, DialogTitle } from "../ui/dialog"
import { Button } from "@/components/ui/button"
import { $allSystemsById } from "@/lib/stores"
import { LoaderCircleIcon, MaximizeIcon, RefreshCwIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { listenKeys } from "nanostores"
import { getPagePath } from "@nanostores/router"
import { LogsDisplay, LogsFullscreenDialog, LogsIconButton, LogsTimestampToggle } from "@/components/logs-display"
import { getLogTimestampDecorations } from "@/lib/logs"

const syntaxTheme = "github-dark-dimmed"

export default function ContainersTable({ systemId }: { systemId?: string }) {
	const [data, setData] = useState<ContainerRecord[] | undefined>(undefined)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [rowSelection, setRowSelection] = useState({})
	const [globalFilter, setGlobalFilter] = useState("")
	const { pagination, onPaginationChange, resetPageIndex } = usePagination()
	const tableSettings = useTableSettings({
		colsKey: "containerCols",
		sortKey: systemId ? "containerSortModeSystem" : "containerSortMode",
		defaultSort: [{ id: systemId ? "name" : "system", desc: false }],
		onSortingChange: resetPageIndex,
	})

	// Leave out the ports column if no container has ports
	const hasPorts = !!data?.some((container) => container.ports)
	const columns = useMemo(
		() =>
			containerChartCols.filter((col) => {
				if (systemId && col.id === "system") return false
				if (!hasPorts && col.id === "ports") return false
				return true
			}),
		[systemId, hasPorts]
	)

	useEffect(() => {
		function fetchData(systemId?: string) {
			return pb
				.collection<ContainerRecord>("containers")
				.getList(0, 2000, {
					fields: "id,name,image,updatable,ports,cpu,memory,net,health,status,system,updated",
					filter: systemId ? pb.filter("system={:system}", { system: systemId }) : undefined,
					requestKey: `containers-${systemId ?? "all"}`,
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
						const containerIds = new Set()
						const newItems: ContainerRecord[] = []
						for (const item of items) {
							if (Math.abs(lastUpdated - item.updated) < 70_000) {
								containerIds.add(item.id)
								newItems.push(item)
							}
						}
						for (const item of curItems ?? []) {
							if (!containerIds.has(item.id) && lastUpdated - item.updated < 70_000) {
								newItems.push(item)
							}
						}
						return newItems
					})
				})
				.catch((e) => {
					if (!e?.isAbort) console.error(e)
				})
		}

		// initial load
		const initialLoad = fetchData(systemId)

		// if no systemId, pull system containers after every system update
		if (!systemId) {
			return $allSystemsById.listen((_value, oldValue, systemId) => {
				// skip newly added systems (initial population of the store)
				if (oldValue[systemId]) {
					// wait for initial load so its older snapshot can't overwrite newer data
					initialLoad.then(() => fetchData(systemId))
				}
			})
		}

		// if systemId, fetch containers after the system is updated
		return listenKeys($allSystemsById, [systemId], (_newSystems) => {
			fetchData(systemId)
		})
	}, [])

	const table = useReactTable({
		data: data ?? [],
		columns,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		autoResetPageIndex: false,
		onPaginationChange,
		onSortingChange: tableSettings.onSortingChange,
		onColumnFiltersChange: setColumnFilters,
		onColumnVisibilityChange: tableSettings.onColumnVisibilityChange,
		onRowSelectionChange: setRowSelection,
		defaultColumn: {
			sortUndefined: "last",
			size: 100,
			minSize: 0,
		},
		state: {
			sorting: tableSettings.sorting,
			columnFilters,
			columnVisibility: tableSettings.columnVisibility,
			rowSelection,
			globalFilter,
			pagination,
		},
		onGlobalFilterChange: (value) => {
			setGlobalFilter(value)
			resetPageIndex()
		},
		globalFilterFn: (row, _columnId, filterValue) => {
			const container = row.original
			return matchesTableFilter(
				filterValue,
				$allSystemsById.get()[container.system]?.name,
				container.id,
				container.name,
				ContainerHealthLabels[container.health as ContainerHealth],
				container.status,
				container.image,
				container.ports
			)
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
							<Trans>All Containers</Trans>
						</CardTitle>
						<CardDescription className="flex">
							<Trans>Click on a container to view more information.</Trans>
						</CardDescription>
					</div>
					<div className="flex gap-2 ms-auto w-full md:w-auto">
						<TableFilterInput value={globalFilter} onChange={table.setGlobalFilter} />
						<TableSettingsMenu table={table} />
					</div>
				</div>
			</CardHeader>
			<div className="rounded-md">
				<AllContainersTable
					table={table}
					rows={rows}
					colLength={visibleColumns.length}
					visibleColumnsKey={visibleColumns.map((column) => column.id).join(",")}
					data={data}
				/>
			</div>
			<DataTablePagination table={table} showSelected={false} />
		</Card>
	)
}

const AllContainersTable = memo(function AllContainersTable({
	table,
	rows,
	colLength,
	visibleColumnsKey,
	data,
}: {
	table: TableType<ContainerRecord>
	rows: Row<ContainerRecord>[]
	colLength: number
	visibleColumnsKey: string
	data: ContainerRecord[] | undefined
}) {
	const activeContainer = useRef<ContainerRecord | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const openSheet = (container: ContainerRecord) => {
		activeContainer.current = container
		setSheetOpen(true)
	}

	return (
		<div className="max-w-full relative overflow-auto border rounded-md">
			<table className="text-sm w-full h-full text-nowrap">
				<ContainersTableHead table={table} />
				<TableBody>
					{rows.length ? (
						rows.map((row) => (
							<ContainerTableRow
								key={row.id}
								row={row}
								openSheet={openSheet}
								visibleColumnsKey={visibleColumnsKey}
							/>
						))
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
			<ContainerSheet sheetOpen={sheetOpen} setSheetOpen={setSheetOpen} activeContainer={activeContainer} />
		</div>
	)
})

async function getLogsHtml(container: ContainerRecord): Promise<string> {
	try {
		const [{ highlighter }, logsHtml] = await Promise.all([
			import("@/lib/shiki"),
			pb.send<{ logs: string }>("/api/beszel/containers/logs", {
				system: container.system,
				container: container.id,
			}),
		])
		return logsHtml.logs
			? highlighter.codeToHtml(logsHtml.logs, {
					lang: "log",
					theme: syntaxTheme,
					decorations: getLogTimestampDecorations(logsHtml.logs),
				})
			: t`No results.`
	} catch (error) {
		console.error(error)
		return ""
	}
}

async function getInfoHtml(container: ContainerRecord): Promise<string> {
	try {
		let [{ highlighter }, { info }] = await Promise.all([
			import("@/lib/shiki"),
			pb.send<{ info: string }>("/api/beszel/containers/info", {
				system: container.system,
				container: container.id,
			}),
		])
		try {
			info = JSON.stringify(JSON.parse(info), null, 2)
		} catch (_) {}
		return info ? highlighter.codeToHtml(info, { lang: "json", theme: syntaxTheme }) : t`No results.`
	} catch (error) {
		console.error(error)
		return ""
	}
}

function ContainerSheet({
	sheetOpen,
	setSheetOpen,
	activeContainer,
}: {
	sheetOpen: boolean
	setSheetOpen: (open: boolean) => void
	activeContainer: RefObject<ContainerRecord | null>
}) {
	const [logsDisplay, setLogsDisplay] = useState<string>("")
	const [infoDisplay, setInfoDisplay] = useState<string>("")
	const [logsFullscreenOpen, setLogsFullscreenOpen] = useState<boolean>(false)
	const [infoFullscreenOpen, setInfoFullscreenOpen] = useState<boolean>(false)
	const [isRefreshingLogs, setIsRefreshingLogs] = useState<boolean>(false)
	const logsContainerRef = useRef<HTMLDivElement>(null)

	const container = activeContainer.current

	function scrollLogsToBottom() {
		if (logsContainerRef.current) {
			logsContainerRef.current.scrollTo({ top: logsContainerRef.current.scrollHeight })
		}
	}

	const refreshLogs = async () => {
		if (!container) return
		setIsRefreshingLogs(true)
		const startTime = Date.now()

		try {
			const logsHtml = await getLogsHtml(container)
			setLogsDisplay(logsHtml)
			setTimeout(scrollLogsToBottom, 20)
		} catch (error) {
			console.error(error)
		} finally {
			// Ensure minimum spin duration of 800ms
			const elapsed = Date.now() - startTime
			const remaining = Math.max(0, 500 - elapsed)
			setTimeout(() => {
				setIsRefreshingLogs(false)
			}, remaining)
		}
	}

	useEffect(() => {
		setLogsDisplay("")
		setInfoDisplay("")
		if (!container) return
		;(async () => {
			const [logsHtml, infoHtml] = await Promise.all([getLogsHtml(container), getInfoHtml(container)])
			setLogsDisplay(logsHtml)
			setInfoDisplay(infoHtml)
			setTimeout(scrollLogsToBottom, 20)
		})()
	}, [container])

	if (!container) return null

	return (
		<>
			<LogsFullscreenDialog
				open={logsFullscreenOpen}
				onOpenChange={setLogsFullscreenOpen}
				logsDisplay={logsDisplay}
				name={container.name}
				onRefresh={refreshLogs}
				isRefreshing={isRefreshingLogs}
			/>
			<InfoFullscreenDialog
				open={infoFullscreenOpen}
				onOpenChange={setInfoFullscreenOpen}
				infoDisplay={infoDisplay}
				containerName={container.name}
			/>
			<Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
				<SheetContent className="w-full sm:max-w-220 p-2">
					<SheetHeader>
						<SheetTitle>{container.name}</SheetTitle>
						<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
							<Link className="hover:underline" href={getPagePath($router, "system", { id: container.system })}>
								{$allSystemsById.get()[container.system]?.name ?? ""}
							</Link>
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.status}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.image}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.id}
							{/* {container.ports && (
								<>
									<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
									{container.ports}
								</>
							)} */}
							{/* <Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{ContainerHealthLabels[container.health as ContainerHealth]} */}
						</SheetDescription>
					</SheetHeader>
					<div className="px-3 pb-3 -mt-4 flex flex-col gap-3 h-full items-start">
						<div className="flex items-center w-full">
							<h3>{t`Logs`}</h3>
							<LogsTimestampToggle className="ms-auto" />
							<LogsIconButton label={t`Refresh`} onClick={refreshLogs} disabled={isRefreshingLogs}>
								<RefreshCwIcon
									className={`size-4 transition-transform duration-300 ${isRefreshingLogs ? "animate-spin" : ""}`}
								/>
							</LogsIconButton>
							<LogsIconButton label={t`Fullscreen`} onClick={() => setLogsFullscreenOpen(true)}>
								<MaximizeIcon className="size-4" />
							</LogsIconButton>
						</div>
						<LogsDisplay logsDisplay={logsDisplay} containerRef={logsContainerRef} />
						<div className="flex items-center w-full">
							<h3>{t`Detail`}</h3>
							<Button
								variant="ghost"
								size="sm"
								onClick={() => setInfoFullscreenOpen(true)}
								className="h-8 w-8 p-0 ms-auto"
							>
								<MaximizeIcon className="size-4" />
							</Button>
						</div>
						<div
							className={cn(
								"grow h-[calc(50dvh-4rem)] w-full overflow-auto p-3 rounded-md bg-gh-dark text-white text-sm",
								!infoDisplay && "animate-pulse"
							)}
						>
							<div dangerouslySetInnerHTML={{ __html: infoDisplay }} />
						</div>
					</div>
				</SheetContent>
			</Sheet>
		</>
	)
}

function ContainersTableHead({ table }: { table: TableType<ContainerRecord> }) {
	return (
		<TableHeader className="sticky top-0 z-50 w-full border-b-2">
			{table.getHeaderGroups().map((headerGroup) => (
				<tr key={headerGroup.id}>
					{headerGroup.headers.map((header) => {
						return (
							<TableHead className="px-2" key={header.id} style={{ width: header.getSize() }}>
								{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
							</TableHead>
						)
					})}
				</tr>
			))}
		</TableHeader>
	)
}

const ContainerTableRow = memo(function ContainerTableRow({
	row,
	openSheet,
	visibleColumnsKey: _visibleColumnsKey,
}: {
	row: Row<ContainerRecord>
	openSheet: (container: ContainerRecord) => void
	// Column visibility doesn't change the row object identity, so this prop exists only
	// to force a re-render (and a fresh row.getVisibleCells() read) when columns are toggled.
	visibleColumnsKey: string
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
						height: 54,
						width: cell.column.getSize(),
					}}
				>
					{flexRender(cell.column.columnDef.cell, cell.getContext())}
				</TableCell>
			))}
		</TableRow>
	)
})

function InfoFullscreenDialog({
	open,
	onOpenChange,
	infoDisplay,
	containerName,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	infoDisplay: string
	containerName: string
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="w-[calc(100vw-20px)] h-[calc(100dvh-20px)] max-w-none p-0 bg-gh-dark border-0 text-white">
				<DialogTitle className="sr-only">{containerName} info</DialogTitle>
				<div className="flex-1 overflow-auto">
					<div className="h-full w-full overflow-auto p-3 rounded-md bg-gh-dark text-sm leading-relaxed">
						<div dangerouslySetInnerHTML={{ __html: infoDisplay }} />
					</div>
				</div>
			</DialogContent>
		</Dialog>
	)
}
