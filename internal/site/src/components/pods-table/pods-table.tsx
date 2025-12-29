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
import type { PodRecord } from "@/types"
import { podChartCols } from "@/components/pods-table/pods-table-columns"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { cn, useBrowserStorage } from "@/lib/utils"
import { Sheet, SheetTitle, SheetHeader, SheetContent, SheetDescription } from "../ui/sheet"
import { Dialog, DialogContent, DialogTitle } from "../ui/dialog"
import { Button } from "@/components/ui/button"
import { $allSystemsById } from "@/lib/stores"
import { LoaderCircleIcon, MaximizeIcon, RefreshCwIcon, XIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { listenKeys } from "nanostores"
import { getPagePath } from "@nanostores/router"

const syntaxTheme = "github-dark-dimmed"

export default function PodsTable({ systemId }: { systemId?: string }) {
	const loadTime = Date.now()
	const [data, setData] = useState<PodRecord[] | undefined>(undefined)
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		`sort-p-${systemId ? 1 : 0}`,
		[{ id: systemId ? "name" : "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
	const [rowSelection, setRowSelection] = useState({})
	const [globalFilter, setGlobalFilter] = useState("")

	useEffect(() => {
		function fetchData(systemId?: string) {
			pb.collection<PodRecord>("pods")
				.getList(0, 2000, {
					fields: "id,name,namespace,cpu,memory,net,status,restarts,system,updated",
					filter: systemId ? pb.filter("system={:system}", { system: systemId }) : undefined,
				})
				.then(
					({ items }) => {
						if (items.length === 0) {
							setData([]);
							return;
						}
						setData((curItems) => {
							const lastUpdated = Math.max(items[0].updated, items.at(-1)?.updated ?? 0)
							const podIds = new Set()
							const newItems = []
							for (const item of items) {
								if (Math.abs(lastUpdated - item.updated) < 70_000) {
									podIds.add(item.id)
									newItems.push(item)
								}
							}
							for (const item of curItems ?? []) {
								if (!podIds.has(item.id) && lastUpdated - item.updated < 70_000) {
									newItems.push(item)
								}
							}
							return newItems
						})
					}
				)
		}

		// initial load
		fetchData(systemId)

		// if no systemId, pull pod data after every system update
		if (!systemId) {
			return $allSystemsById.listen((_value, _oldValue, systemId) => {
				// exclude initial load of systems
				if (Date.now() - loadTime > 500) {
					fetchData(systemId)
				}
			})
		}

		// if systemId, fetch pods after the system is updated
		return listenKeys($allSystemsById, [systemId], (_newSystems) => {
			fetchData(systemId)
		})
	}, [])

	const table = useReactTable({
		data: data ?? [],
		columns: podChartCols.filter((col) => (systemId ? col.id !== "system" : true)),
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
			const pod = row.original
			const systemName = $allSystemsById.get()[pod.system]?.name ?? ""
			const id = pod.id ?? ""
			const name = pod.name ?? ""
			const namespace = pod.namespace ?? ""
			const status = pod.status ?? ""
			const searchString = `${systemName} ${id} ${name} ${namespace} ${status}`.toLowerCase()

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
							<Trans>All Pods</Trans>
						</CardTitle>
						<CardDescription className="flex">
							<Trans>Click on a pod to view more information.</Trans>
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
				</div>
			</CardHeader>
			<div className="rounded-md">
				<AllPodsTable table={table} rows={rows} colLength={visibleColumns.length} data={data} />
			</div>
		</Card>
	)
}

const AllPodsTable = memo(function AllPodsTable({
	table,
	rows,
	colLength,
	data,
}: {
	table: TableType<PodRecord>
	rows: Row<PodRecord>[]
	colLength: number
	data: PodRecord[] | undefined
}) {
	// The virtualizer will need a reference to the scrollable container element
	const scrollRef = useRef<HTMLDivElement>(null)
	const activePod = useRef<PodRecord | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const openSheet = (pod: PodRecord) => {
		activePod.current = pod
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
					<PodsTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return <PodTableRow key={row.id} row={row} virtualRow={virtualRow} openSheet={openSheet} />
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
			<PodSheet sheetOpen={sheetOpen} setSheetOpen={setSheetOpen} activePod={activePod} />
		</div>
	)
})

async function getLogsHtml(pod: PodRecord): Promise<string> {
	try {
		const [{ highlighter }, logsHtml] = await Promise.all([
			import("@/lib/shiki"),
			pb.send<{ logs: string }>("/api/beszel/pods/logs", {
				method: "GET",
				query: {
					system: pod.system,
					namespace: pod.namespace,
					pod: pod.name,
				},
			}),
		])
		return logsHtml.logs ? highlighter.codeToHtml(logsHtml.logs, { lang: "log", theme: syntaxTheme }) : t`No results.`
	} catch (error) {
		console.error(error)
		return ""
	}
}

async function getInfoHtml(pod: PodRecord): Promise<string> {
	try {
		let [{ highlighter }, { info }] = await Promise.all([
			import("@/lib/shiki"),
			pb.send<{ info: string }>("/api/beszel/pods/info", {
				method: "GET",
				query: {
					system: pod.system,
					namespace: pod.namespace,
					pod: pod.name,
				},
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

function PodSheet({
	sheetOpen,
	setSheetOpen,
	activePod,
}: {
	sheetOpen: boolean
	setSheetOpen: (open: boolean) => void
	activePod: RefObject<PodRecord | null>
}) {
	const pod = activePod.current
	if (!pod) return null

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
			const logsHtml = await getLogsHtml(pod)
			setLogsDisplay(logsHtml)
			setTimeout(scrollLogsToBottom, 20)
		} catch (error) {
			console.error(error)
		} finally {
			// Ensure minimum spin duration of 500ms
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
		if (!pod) return
			;(async () => {
				const [logsHtml, infoHtml] = await Promise.all([getLogsHtml(pod), getInfoHtml(pod)])
				setLogsDisplay(logsHtml)
				setInfoDisplay(infoHtml)
				setTimeout(scrollLogsToBottom, 20)
			})()
	}, [pod])

	return (
		<>
			<LogsFullscreenDialog
				open={logsFullscreenOpen}
				onOpenChange={setLogsFullscreenOpen}
				logsDisplay={logsDisplay}
				podName={pod.name}
				onRefresh={refreshLogs}
				isRefreshing={isRefreshingLogs}
			/>
			<InfoFullscreenDialog
				open={infoFullscreenOpen}
				onOpenChange={setInfoFullscreenOpen}
				infoDisplay={infoDisplay}
				podName={pod.name}
			/>
			<Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
				<SheetContent className="w-full sm:max-w-220 p-2">
					<SheetHeader>
						<SheetTitle>{pod.name}</SheetTitle>
						<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
							<Link className="hover:underline" href={getPagePath($router, "system", { id: pod.system })}>
								{$allSystemsById.get()[pod.system]?.name ?? ""}
							</Link>
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{pod.namespace}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{pod.status}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{t`Restarts`}: {pod.restarts}
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
									className={`size-4 transition-transform duration-300 ${isRefreshingLogs ? "animate-spin" : ""}`}
								/>
							</Button>
							<Button variant="ghost" size="sm" onClick={() => setLogsFullscreenOpen(true)} className="h-8 w-8 p-0">
								<MaximizeIcon className="size-4" />
							</Button>
						</div>
						<div
							ref={logsContainerRef}
							className={cn(
								"max-h-[calc(50dvh-10rem)] w-full overflow-auto p-3 rounded-md bg-gh-dark text-white text-sm",
								!logsDisplay && ["animate-pulse", "h-full"]
							)}
						>
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

function PodsTableHead({ table }: { table: TableType<PodRecord> }) {
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

const PodTableRow = memo(function PodTableRow({
	row,
	virtualRow,
	openSheet,
}: {
	row: Row<PodRecord>
	virtualRow: VirtualItem
	openSheet: (pod: PodRecord) => void
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

function LogsFullscreenDialog({
	open,
	onOpenChange,
	logsDisplay,
	podName,
	onRefresh,
	isRefreshing,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	logsDisplay: string
	podName: string
	onRefresh: () => void | Promise<void>
	isRefreshing: boolean
}) {
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
				<DialogTitle className="sr-only">{podName} logs</DialogTitle>
				<div ref={outerContainerRef} className="h-full overflow-auto">
					<div className="h-full w-full px-3 leading-relaxed rounded-md bg-gh-dark text-sm">
						<div className="py-3" dangerouslySetInnerHTML={{ __html: logsDisplay }} />
					</div>
				</div>
				<button
					onClick={onRefresh}
					className="absolute top-3 right-11 opacity-60 hover:opacity-100 p-1"
					disabled={isRefreshing}
					title={t`Refresh`}
					aria-label={t`Refresh`}
				>
					<RefreshCwIcon className={`size-4 transition-transform duration-300 ${isRefreshing ? "animate-spin" : ""}`} />
				</button>
			</DialogContent>
		</Dialog>
	)
}

function InfoFullscreenDialog({
	open,
	onOpenChange,
	infoDisplay,
	podName,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	infoDisplay: string
	podName: string
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="w-[calc(100vw-20px)] h-[calc(100dvh-20px)] max-w-none p-0 bg-gh-dark border-0 text-white">
				<DialogTitle className="sr-only">{podName} info</DialogTitle>
				<div className="flex-1 overflow-auto">
					<div className="h-full w-full overflow-auto p-3 rounded-md bg-gh-dark text-sm leading-relaxed">
						<div dangerouslySetInnerHTML={{ __html: infoDisplay }} />
					</div>
				</div>
			</DialogContent>
		</Dialog>
	)
}
