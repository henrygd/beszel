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
import type { ContainerRecord } from "@/types"
import { containerChartCols } from "@/components/containers-table/containers-table-columns"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { type ContainerHealth, ContainerHealthLabels } from "@/lib/enums"
import { cn, useBrowserStorage } from "@/lib/utils"
import { Sheet, SheetTitle, SheetHeader, SheetContent, SheetDescription } from "../ui/sheet"
import { Dialog, DialogContent, DialogTitle } from "../ui/dialog"
import { Button } from "@/components/ui/button"
import { $allSystemsById } from "@/lib/stores"
import { MaximizeIcon, RefreshCwIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { listenKeys } from "nanostores"
import { getPagePath } from "@nanostores/router"

const syntaxTheme = "github-dark-dimmed"

export default function ContainersTable({ systemId }: { systemId?: string }) {
	const [data, setData] = useState<ContainerRecord[]>([])
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		`sort-c-${systemId ? 1 : 0}`,
		[{ id: systemId ? "name" : "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
	const [rowSelection, setRowSelection] = useState({})
	const [globalFilter, setGlobalFilter] = useState("")

	useEffect(() => {
		const pbOptions = {
			fields: "id,name,cpu,memory,net,health,status,system,updated",
		}

		const fetchData = (lastXMs: number) => {
			const updated = Date.now() - lastXMs
			let filter: string
			if (systemId) {
				filter = pb.filter("system={:system} && updated > {:updated}", { system: systemId, updated })
			} else {
				filter = pb.filter("updated > {:updated}", { updated })
			}
			pb.collection<ContainerRecord>("containers")
				.getList(0, 2000, {
					...pbOptions,
					filter,
				})
				.then(({ items }) => setData((curItems) => {
					const containerIds = new Set(items.map(item => item.id))
					const now = Date.now()
					for (const item of curItems) {
						if (!containerIds.has(item.id) && now - item.updated < 70_000) {
							items.push(item)
						}
					}
					return items
				}))
		}

		// initial load
		fetchData(70_000)

		// if no systemId, poll every 10 seconds
		if (!systemId) {
			// poll every 10 seconds
			const intervalId = setInterval(() => fetchData(10_500), 10_000)
			// clear interval on unmount
			return () => clearInterval(intervalId)
		}

		// if systemId, fetch containers after the system is updated
		return listenKeys($allSystemsById, [systemId], (_newSystems) => {
			setTimeout(() => fetchData(1000), 100)
		})
	}, [])

	const table = useReactTable({
		data,
		columns: containerChartCols.filter(col => systemId ? col.id !== "system" : true),
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
			const container = row.original
			const systemName = $allSystemsById.get()[container.system]?.name ?? ""
			const id = container.id ?? ""
			const name = container.name ?? ""
			const status = container.status ?? ""
			const healthLabel = ContainerHealthLabels[container.health as ContainerHealth] ?? ""
			const searchString = `${systemName} ${id} ${name} ${healthLabel} ${status}`.toLowerCase()

			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})

	const rows = table.getRowModel().rows
	const visibleColumns = table.getVisibleLeafColumns()

	if (!rows.length) return null

	return (
		<Card className="p-6 @container w-full">
			<CardHeader className="p-0 mb-4">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>All Containers</Trans>
						</CardTitle>
						<CardDescription className="flex">
							<Trans>Click on a container to view more information.</Trans>
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
				<AllContainersTable table={table} rows={rows} colLength={visibleColumns.length} />
			</div>
		</Card>
	)
}

const AllContainersTable = memo(
	function AllContainersTable({ table, rows, colLength }: { table: TableType<ContainerRecord>; rows: Row<ContainerRecord>[]; colLength: number }) {
		// The virtualizer will need a reference to the scrollable container element
		const scrollRef = useRef<HTMLDivElement>(null)
		const activeContainer = useRef<ContainerRecord | null>(null)
		const [sheetOpen, setSheetOpen] = useState(false)
		const openSheet = (container: ContainerRecord) => {
			activeContainer.current = container
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
				<div style={{ height: `${virtualizer.getTotalSize() + 50}px`, paddingTop, paddingBottom }}>
					<table className="text-sm w-full h-full">
						<ContainersTableHead table={table} />
						<TableBody>
							{rows.length ? (
								virtualRows.map((virtualRow) => {
									const row = rows[virtualRow.index]
									return (
										<ContainerTableRow
											key={row.id}
											row={row}
											virtualRow={virtualRow}
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
				<ContainerSheet sheetOpen={sheetOpen} setSheetOpen={setSheetOpen} activeContainer={activeContainer} />
			</div>
		)
	}
)


async function getLogsHtml(container: ContainerRecord): Promise<string> {
	try {
		const [{ highlighter }, logsHtml] = await Promise.all([import('@/lib/shiki'), pb.send<{ logs: string }>("/api/beszel/containers/logs", {
			system: container.system,
			container: container.id,
		})])
		return highlighter.codeToHtml(logsHtml.logs, { lang: "log", theme: syntaxTheme })
	} catch (error) {
		console.error(error)
		return ""
	}
}

async function getInfoHtml(container: ContainerRecord): Promise<string> {
	try {
		let [{ highlighter }, { info }] = await Promise.all([import('@/lib/shiki'), pb.send<{ info: string }>("/api/beszel/containers/info", {
			system: container.system,
			container: container.id,
		})])
		try {
			info = JSON.stringify(JSON.parse(info), null, 2)
		} catch (_) { }
		return highlighter.codeToHtml(info, { lang: "json", theme: syntaxTheme })
	} catch (error) {
		console.error(error)
		return ""
	}
}

function ContainerSheet({ sheetOpen, setSheetOpen, activeContainer }: { sheetOpen: boolean, setSheetOpen: (open: boolean) => void, activeContainer: RefObject<ContainerRecord | null> }) {
	const container = activeContainer.current
	if (!container) return null

	const [logsDisplay, setLogsDisplay] = useState<string>("")
	const [infoDisplay, setInfoDisplay] = useState<string>("")
	const [logsFullscreenOpen, setLogsFullscreenOpen] = useState<boolean>(false)
	const [infoFullscreenOpen, setInfoFullscreenOpen] = useState<boolean>(false)
	const [isRefreshingLogs, setIsRefreshingLogs] = useState<boolean>(false)
	const logsContainerRef = useRef<HTMLDivElement>(null)

	function scrollLogsToBottom() {
		if (logsContainerRef.current) {
			logsContainerRef.current.scrollTo({ top: logsContainerRef.current.scrollHeight })
		}
	}

	const refreshLogs = async () => {
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
		setInfoDisplay("");
		if (!container) return
		(async () => {
			const [logsHtml, infoHtml] = await Promise.all([getLogsHtml(container), getInfoHtml(container)])
			setLogsDisplay(logsHtml)
			setInfoDisplay(infoHtml)
			setTimeout(scrollLogsToBottom, 20)
		})()
	}, [container])

	return (
		<>
			<LogsFullscreenDialog
				open={logsFullscreenOpen}
				onOpenChange={setLogsFullscreenOpen}
				logsDisplay={logsDisplay}
				containerName={container.name}
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
						<SheetDescription className="flex items-center gap-2">
							<Link className="hover:underline" href={getPagePath($router, "system", { id: container.system })}>{$allSystemsById.get()[container.system]?.name ?? ""}</Link>
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.status}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.id}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{ContainerHealthLabels[container.health as ContainerHealth]}
						</SheetDescription>
					</SheetHeader>
					<div className="px-3 pb-3 -mt-4 flex flex-col gap-3 h-full items-start">
						<div className="flex items-center w-full">
							<h3>{t`Logs`}</h3>
							<Button
								variant="ghost"
								size="sm"
								onClick={refreshLogs}
								className="h-8 w-8 p-0 ms-auto"
								disabled={isRefreshingLogs}
							>
								<RefreshCwIcon
									className={`size-4 transition-transform duration-300 ${isRefreshingLogs ? 'animate-spin' : ''}`}
								/>
							</Button>
							<Button
								variant="ghost"
								size="sm"
								onClick={() => setLogsFullscreenOpen(true)}
								className="h-8 w-8 p-0"
							>
								<MaximizeIcon className="size-4" />
							</Button>
						</div>
						<div ref={logsContainerRef} className={cn("max-h-[calc(50dvh-10rem)] w-full overflow-auto p-3 rounded-md bg-gh-dark text-sm", !logsDisplay && ["animate-pulse", "h-full"])}>
							<div dangerouslySetInnerHTML={{ __html: logsDisplay }} />
						</div>
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
						<div className={cn("grow h-[calc(50dvh-4rem)] w-full overflow-auto p-3 rounded-md bg-gh-dark text-sm", !infoDisplay && "animate-pulse")}>
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

const ContainerTableRow = memo(
	function ContainerTableRow({
		row,
		virtualRow,
		openSheet,
	}: {
		row: Row<ContainerRecord>
		virtualRow: VirtualItem
		openSheet: (container: ContainerRecord) => void
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
						style={{
							height: virtualRow.size,
						}}
					>
						{flexRender(cell.column.columnDef.cell, cell.getContext())}
					</TableCell>
				))}
			</TableRow>
		)
	}
)

function LogsFullscreenDialog({ open, onOpenChange, logsDisplay, containerName, onRefresh, isRefreshing }: { open: boolean, onOpenChange: (open: boolean) => void, logsDisplay: string, containerName: string, onRefresh: () => void | Promise<void>, isRefreshing: boolean }) {
	const outerContainerRef = useRef<HTMLDivElement>(null)

	useEffect(() => {
		if (open && logsDisplay) {
			// Scroll the outer container to bottom
			const scrollToBottom = () => {
				if (outerContainerRef.current) {
					outerContainerRef.current.scrollTop = outerContainerRef.current.scrollHeight
				}
			}
			setTimeout(scrollToBottom, 50)
		}
	}, [open, logsDisplay])

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="w-[calc(100vw-20px)] h-[calc(100dvh-20px)] max-w-none p-0 bg-gh-dark border-0 text-white">
				<DialogTitle className="sr-only">{containerName} logs</DialogTitle>
				<div ref={outerContainerRef} className="h-full overflow-auto">
					<div className="h-full w-full px-3 leading-relaxed rounded-md bg-gh-dark text-sm">
						<div className="py-3" dangerouslySetInnerHTML={{ __html: logsDisplay }} />
					</div>
				</div>
				<button
					onClick={() => {
						void onRefresh()
					}}
					className="absolute top-3 right-11 opacity-60 hover:opacity-100 p-1"
					disabled={isRefreshing}
					title={t`Refresh`}
					aria-label={t`Refresh`}
				>
					<RefreshCwIcon
						className={`size-4 transition-transform duration-300 ${isRefreshing ? 'animate-spin' : ''}`}
					/>
				</button>
			</DialogContent>
		</Dialog>
	)
}

function InfoFullscreenDialog({ open, onOpenChange, infoDisplay, containerName }: { open: boolean, onOpenChange: (open: boolean) => void, infoDisplay: string, containerName: string }) {
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
