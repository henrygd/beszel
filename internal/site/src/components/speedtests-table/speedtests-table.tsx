import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type RowSelectionState,
	type SortingState,
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
import { useCallback, useMemo, useState } from "react"
import { Card, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useToast } from "@/components/ui/use-toast"
import { isReadOnlyUser, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import { $allSystemsById, $direction, getUserChartTime } from "@/lib/stores"
import { cn, formatShortDate, matchesFilterGroups, parseFilterGroups, parseSemVer } from "@/lib/utils"
import type { ChartData, ChartTimes, SpeedtestRecord } from "@/types"
import { DEFAULT_HIDDEN_SPEEDTEST_COLUMNS, getSpeedtestColumns } from "./speedtests-columns"
import { AddSpeedtestDialog, EditSpeedtestDialog } from "./speedtest-dialog"
import {
	ArrowDownIcon,
	ArrowUpDownIcon,
	ArrowUpIcon,
	EyeIcon,
	RefreshCwIcon,
	LandmarkIcon,
	LoaderCircleIcon,
	ServerIcon,
	Settings2Icon,
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
import {
	SpeedtestDownloadChart,
	SpeedtestLatencyChart,
	SpeedtestLoadedLatencyChart,
	SpeedtestLossChart,
	SpeedtestUploadChart,
} from "@/components/routes/system/charts/speedtest-charts"
import { useSpeedtestStats } from "@/lib/use-speedtests"
import { formatSpeedtestInterval, getSpeedtestServerLabel } from "@/lib/speedtest-utils"
import { useStore } from "@nanostores/react"
import { atom } from "nanostores"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { getPagePath } from "@nanostores/router"

const COLUMN_STORAGE_KEY = "besz-speedtest-cols"

function loadColumnVisibility(): VisibilityState {
	try {
		return { ...DEFAULT_HIDDEN_SPEEDTEST_COLUMNS, ...JSON.parse(localStorage.getItem(COLUMN_STORAGE_KEY) || "{}") }
	} catch {
		return { ...DEFAULT_HIDDEN_SPEEDTEST_COLUMNS }
	}
}

export default function SpeedtestsTable({
	systemId,
	speedtests,
	isLoading,
}: {
	systemId?: string
	speedtests: SpeedtestRecord[]
	isLoading: boolean
}) {
	const [sorting, setSorting] = useState<SortingState>([{ id: systemId ? "server" : "system", desc: false }])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>(loadColumnVisibility)
	const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
	const [globalFilter, setGlobalFilter] = useState("")
	const [pendingDeleteIds, setPendingDeleteIds] = useState<string[]>([])
	const [editingSpeedtest, setEditingSpeedtest] = useState<SpeedtestRecord>()
	const [activeSpeedtestId, setActiveSpeedtestId] = useState<string>()
	const [sheetOpen, setSheetOpen] = useState(false)
	const activeSpeedtest = speedtests.find((speedtest) => speedtest.id === activeSpeedtestId)

	const { toast } = useToast()
	const canManage = !isReadOnlyUser()

	const handleColumnVisibilityChange = useCallback(
		(updater: VisibilityState | ((prev: VisibilityState) => VisibilityState)) => {
			setColumnVisibility((prev) => {
				const next = typeof updater === "function" ? updater(prev) : updater
				try {
					localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify(next))
				} catch {}
				return next
			})
		},
		[]
	)

	const showError = useCallback(
		(err: unknown) => toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message }),
		[toast]
	)

	const runBatch = useCallback(
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
		async (toDelete: SpeedtestRecord[]) => {
			if (toDelete.length === 1) {
				await pb.collection("speedtests").delete(toDelete[0].id).catch(showError)
				return
			}
			setPendingDeleteIds(toDelete.map((speedtest) => speedtest.id))
		},
		[showError]
	)

	const handleBulkDelete = async () => {
		const ids = pendingDeleteIds
		setPendingDeleteIds([])
		try {
			await runBatch(ids, (batch, id) => batch.collection("speedtests").delete(id))
			setRowSelection({})
		} catch (err) {
			showError(err)
		}
	}

	const handleSetEnabled = useCallback(
		async (toUpdate: SpeedtestRecord[], enabled: boolean) => {
			const ids = toUpdate.filter((speedtest) => speedtest.enabled !== enabled).map((speedtest) => speedtest.id)
			try {
				await runBatch(ids, (batch, id) => batch.collection("speedtests").update(id, { enabled }))
				if (toUpdate.length > 1) {
					setRowSelection({})
				}
			} catch (err) {
				showError(err)
			}
		},
		[runBatch, showError]
	)

	const handleRunNow = useCallback(
		async (toRun: SpeedtestRecord[]) => {
			try {
				for (const speedtest of toRun) {
					await pb.send("/api/beszel/speedtest/run", { method: "POST", query: { id: speedtest.id } })
				}
				toast({
					title: toRun.length > 1 ? t`Speedtests started` : t`Speedtest started`,
					description: t`Results appear once the test finishes, usually within a minute or two.`,
				})
				if (toRun.length > 1) {
					setRowSelection({})
				}
			} catch (err) {
				showError(err)
			}
		},
		[showError, toast]
	)

	const columns = useMemo(() => {
		let columns = getSpeedtestColumns({
			onEdit: setEditingSpeedtest,
			onDelete: handleDeleteRequest,
			onSetEnabled: handleSetEnabled,
			onRunNow: handleRunNow,
		})
		if (systemId) columns = columns.filter((col) => col.id !== "system")
		if (!canManage) columns = columns.filter((col) => col.id !== "actions" && col.id !== "select")
		return columns
	}, [canManage, handleDeleteRequest, handleSetEnabled, handleRunNow, systemId])

	const table = useReactTable({
		data: speedtests,
		columns,
		getRowId: (row) => row.id,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onColumnVisibilityChange: handleColumnVisibilityChange,
		onRowSelectionChange: setRowSelection,
		defaultColumn: {
			sortUndefined: "last",
			size: 900,
			minSize: 0,
		},
		state: { sorting, columnVisibility, rowSelection, globalFilter },
		onGlobalFilterChange: setGlobalFilter,
		globalFilterFn: (row, _columnId, filterValue) => {
			const value = (filterValue as string).trim()
			if (!value) return true
			const speedtest = row.original
			const systemName = $allSystemsById.get()[speedtest.system]?.name ?? ""
			const searchString = `${systemName}${getSpeedtestServerLabel(speedtest)}${speedtest.isp}`.toLocaleLowerCase()
			return matchesFilterGroups(searchString, parseFilterGroups(value))
		},
	})

	const rows = table.getRowModel().rows
	const allSystems = useStore($allSystemsById)

	return (
		<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 mb-3 sm:mb-4">
				<div className="grid md-lg:flex gap-x-5 gap-y-3 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>Speedtests</Trans>
						</CardTitle>
						<div className="text-sm text-muted-foreground flex items-center flex-wrap">
							<Trans>Scheduled bandwidth tests from agents.</Trans>
						</div>
					</div>
					<div className="md-lg:ms-auto flex items-center gap-2">
						{speedtests.length > 0 && (
							<div className="relative grow">
								<Input
									placeholder={t`Filter...`}
									value={globalFilter}
									onChange={(e) => setGlobalFilter(e.target.value)}
									className="ms-auto px-4 w-full max-w-full md-lg:w-50"
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
															setSorting([{ id: column.id, desc: sorting[0]?.id === column.id && !sorting[0]?.desc }])
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
						{canManage && <AddSpeedtestDialog systemId={systemId} />}
						{canManage && (
							<EditSpeedtestDialog
								systemId={systemId}
								speedtest={editingSpeedtest}
								open={!!editingSpeedtest}
								setOpen={(open) => {
									if (!open) setEditingSpeedtest(undefined)
								}}
							/>
						)}
						<AlertDialog
							open={pendingDeleteIds.length > 0}
							onOpenChange={(open) => {
								if (!open) setPendingDeleteIds([])
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
			<div className="h-min max-h-[calc(100dvh-17rem)] max-w-full relative overflow-auto border rounded-md">
				<table className="text-sm w-full h-full text-nowrap">
					<TableHeader className="sticky top-0 z-50 w-full border-b-2">
						{table.getHeaderGroups().map((headerGroup) => (
							<tr key={headerGroup.id}>
								{headerGroup.headers.map((header) => (
									<TableHead className="px-2" key={header.id}>
										{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
									</TableHead>
								))}
							</tr>
						))}
					</TableHeader>
					<TableBody>
						{rows.length ? (
							rows.map((row) => (
								<TableRow
									key={row.id}
									data-state={row.getIsSelected() && "selected"}
									className={cn("cursor-pointer transition-opacity h-13.5", {
										"opacity-50": allSystems[row.original.system]?.status === SystemStatus.Paused,
									})}
									onClick={() => {
										setActiveSpeedtestId(row.original.id)
										setSheetOpen(true)
									}}
								>
									{row.getVisibleCells().map((cell) => (
										<TableCell key={cell.id} className="py-0" style={{ width: `${cell.column.getSize()}px` }}>
											{flexRender(cell.column.columnDef.cell, cell.getContext())}
										</TableCell>
									))}
								</TableRow>
							))
						) : (
							<TableRow>
								<TableCell
									colSpan={table.getVisibleLeafColumns().length}
									className="h-37 text-center pointer-events-none"
								>
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
			</div>
			{activeSpeedtest && (
				<SpeedtestSheet
					key={activeSpeedtest.id}
					open={sheetOpen}
					onOpenChange={setSheetOpen}
					speedtest={activeSpeedtest}
				/>
			)}
		</Card>
	)
}

function SpeedtestSheet({
	open,
	onOpenChange,
	speedtest,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	speedtest: SpeedtestRecord
}) {
	// Start from the user's default chart time, but keep it separate from the system charts' time range.
	const [chartTimeStore] = useState(() => {
		const defaultTime = getUserChartTime()
		return atom<ChartTimes>(defaultTime === "1m" ? "1h" : defaultTime)
	})
	const chartTime = useStore(chartTimeStore)
	const direction = useStore($direction)
	const system = useStore($allSystemsById)[speedtest.system]
	const stats = useSpeedtestStats({ speedtest, chartTime, enabled: open })

	const chartData = useMemo<ChartData>(
		() => ({
			agentVersion: parseSemVer(system?.info?.v),
			orientation: direction === "rtl" ? "right" : "left",
			chartTime,
		}),
		[system?.info?.v, direction, chartTime]
	)
	const empty = !stats.some((record) => record.created !== null)
	const serverLabel = getSpeedtestServerLabel(speedtest) || t`Automatic`

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-220 overflow-auto p-4 sm:p-6">
				<SheetHeader className="mb-0 border-b p-0 pb-4">
					<SheetTitle>{serverLabel}</SheetTitle>
					<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
						<ServerIcon className="size-3.5 text-muted-foreground" />
						<Link className="hover:underline" href={getPagePath($router, "system", { id: system?.id ?? "" })}>
							{system?.name ?? ""}
						</Link>
						{speedtest.isp && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<LandmarkIcon className="size-3.5 text-muted-foreground -me-0.5" />
								<span>{speedtest.isp}</span>
							</>
						)}
						<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
						<RefreshCwIcon className="size-3.5 text-muted-foreground -me-0.5" />
						<span>
							<Trans>Every {formatSpeedtestInterval(speedtest.interval)}</Trans>
						</span>
						{speedtest.last_run > 0 && (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<span>
									<Trans>Last run {formatShortDate(new Date(speedtest.last_run).toISOString())}</Trans>
								</span>
							</>
						)}
					</SheetDescription>
				</SheetHeader>
				<div className="grid xl:grid-cols-2 gap-4">
					<ChartTimeSelect
						className="bg-card col-span-full"
						agentVersion={chartData.agentVersion}
						chartTimeStore={chartTimeStore}
						allowRealtime={false}
					/>
					<SpeedtestDownloadChart stats={stats} chartData={chartData} empty={empty} />
					<SpeedtestUploadChart stats={stats} chartData={chartData} empty={empty} />
					<SpeedtestLatencyChart stats={stats} chartData={chartData} empty={empty} />
					<SpeedtestLoadedLatencyChart stats={stats} chartData={chartData} empty={empty} />
					<SpeedtestLossChart stats={stats} chartData={chartData} empty={empty} />
				</div>
			</SheetContent>
		</Sheet>
	)
}
