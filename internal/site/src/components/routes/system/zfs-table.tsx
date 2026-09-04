import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { isReadOnlyUser, pb } from "@/lib/api"
import { cn, formatBytes, formatShortDate, hourWithSeconds, toFixedFloat } from "@/lib/utils"
import type { ZfsDataset, ZfsPoolRecord, ZfsVdev } from "@/types"
import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import type { Column, ColumnDef } from "@tanstack/react-table"
import {
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table"
import {
	ActivityIcon,
	BinaryIcon,
	CheckCircleIcon,
	CircleAlertIcon,
	ClockIcon,
	HardDriveDownloadIcon,
	HardDriveIcon,
	HardDriveUploadIcon,
	LoaderCircleIcon,
	MoreHorizontalIcon,
	RefreshCwIcon,
	RotateCwIcon,
	XCircleIcon,
	XIcon,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useState } from "react"

const ZFS_POOL_FIELDS = "id,system,name,health,size,alloc,free,scrub,details_updated,updated"

/** Maps a zpool health string to a Badge variant. */
function healthVariant(health: string): "success" | "warning" | "danger" | "outline" {
	switch (health) {
		case "ONLINE":
			return "success"
		case "DEGRADED":
			return "warning"
		case "FAULTED":
		case "OFFLINE":
		case "UNAVAIL":
		case "REMOVED":
		case "SUSPENDED":
			return "danger"
		default:
			return "outline"
	}
}

function formatCapacity(bytes: number): string {
	if (!bytes) return "-"
	const { value, unit } = formatBytes(bytes)
	return `${toFixedFloat(value, value >= 10 ? 1 : 2)} ${unit}`
}

function HeaderButton<T>({ column, name, Icon }: { column: Column<T>; name: string; Icon: React.ElementType }) {
	const isSorted = column.getIsSorted()
	return (
		<Button
			className={cn(
				"h-9 px-3 flex items-center gap-2 duration-50",
				isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90"
			)}
			variant="ghost"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			<Icon className="size-4" />
			{name}
		</Button>
	)
}

const columns: ColumnDef<ZfsPoolRecord>[] = [
	{
		accessorKey: "name",
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		header: ({ column }) => <HeaderButton column={column} name={`Pool`} Icon={HardDriveIcon} />,
		cell: ({ getValue }) => <span className="font-medium ms-1.5">{getValue() as string}</span>,
	},
	{
		accessorKey: "health",
		sortingFn: (a, b) => a.original.health.localeCompare(b.original.health),
		header: ({ column }) => <HeaderButton column={column} name={t`Health`} Icon={ActivityIcon} />,
		cell: ({ getValue }) => {
			const health = (getValue() as string) || ""
			return <Badge variant={healthVariant(health)}>{health || t`Unknown`}</Badge>
		},
	},
	{
		id: "size",
		accessorFn: (record) => record.size,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Capacity`} Icon={BinaryIcon} />,
		cell: ({ getValue }) => <span className="ms-1.5 tabular-nums">{formatCapacity(getValue() as number)}</span>,
	},
	{
		id: "used",
		accessorFn: (record) => record.alloc,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Used`} Icon={HardDriveDownloadIcon} />,
		cell: ({ getValue }) => <span className="ms-1.5 tabular-nums">{formatCapacity(getValue() as number)}</span>,
	},
	{
		id: "free",
		accessorFn: (record) => record.free,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t({ message: `Free`, context: "Free space" })} Icon={HardDriveUploadIcon} />,
		cell: ({ getValue }) => <span className="ms-1.5 tabular-nums">{formatCapacity(getValue() as number)}</span>,
	},
	{
		id: "scrub",
		accessorFn: (record) => record.scrub?.state ?? "",
		header: ({ column }) => <HeaderButton column={column} name={`Scrub`} Icon={RotateCwIcon} />,
		cell: ({ row }) => {
			const scrub = row.original.scrub
			if (!scrub?.state) return <span className="ms-1.5 text-muted-foreground">{t`None`}</span>
			return (
				<span className="ms-1.5 tabular-nums">
					{scrub.state}
					{scrub.progress ? ` (${scrub.progress})` : ""}
				</span>
			)
		},
	},
	{
		id: "updated",
		invertSorting: true,
		accessorFn: (record) => record.details_updated || record.updated,
		header: ({ column }) => <HeaderButton column={column} name={t`Updated`} Icon={ClockIcon} />,
		cell: ({ getValue }) => {
			const timestamp = getValue() as string
			if (!timestamp) return null
			const formatter =
				new Date(timestamp).toDateString() === new Date().toDateString() ? hourWithSeconds : formatShortDate
			return <span className="ms-1 tabular-nums">{formatter(timestamp)}</span>
		},
	},
]

function VdevTable({ vdevs }: { vdevs: ZfsVdev[] }) {
	if (!vdevs?.length) return null
	return (
		<div className="overflow-x-auto rounded-md border">
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Vdev</TableHead>
						<TableHead>{t`State`}</TableHead>
						<TableHead className="text-right">{t`Read errors`}</TableHead>
						<TableHead className="text-right">{t`Write errors`}</TableHead>
						<TableHead className="text-right">{t`Checksum errors`}</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{vdevs.map((vdev) => (
						<TableRow key={vdev.name}>
							<TableCell className="font-mono text-xs">{vdev.name}</TableCell>
							<TableCell>
								<Badge variant={healthVariant(vdev.state ?? "")} className="font-normal">
									{vdev.state ?? "-"}
								</Badge>
							</TableCell>
							<TableCell
								className={cn("text-right tabular-nums", (vdev.readErrs ?? 0) > 0 && "text-red-600 dark:text-red-400")}
							>
								{vdev.readErrs ?? 0}
							</TableCell>
							<TableCell
								className={cn("text-right tabular-nums", (vdev.writeErrs ?? 0) > 0 && "text-red-600 dark:text-red-400")}
							>
								{vdev.writeErrs ?? 0}
							</TableCell>
							<TableCell
								className={cn(
									"text-right tabular-nums",
									(vdev.checksumErrs ?? 0) > 0 && "text-red-600 dark:text-red-400"
								)}
							>
								{vdev.checksumErrs ?? 0}
							</TableCell>
						</TableRow>
					))}
				</TableBody>
			</Table>
		</div>
	)
}

const datasetColumns: ColumnDef<ZfsDataset>[] = [
	{
		accessorKey: "name",
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		header: ({ column }) => <HeaderButton column={column} name={`Dataset`} Icon={HardDriveIcon} />,
		cell: ({ getValue }) => <span className="font-mono text-xs">{getValue() as string}</span>,
	},
	{
		id: "used",
		accessorFn: (ds) => ds.used ?? 0,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Used`} Icon={HardDriveDownloadIcon} />,
		cell: ({ getValue }) => <span className="text-right tabular-nums">{formatCapacity(getValue() as number)}</span>,
	},
	{
		id: "avail",
		accessorFn: (ds) => ds.avail ?? 0,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t({message:`Available`, context: "Disk space available"})} Icon={HardDriveUploadIcon} />,
		cell: ({ getValue }) => <span className="text-right tabular-nums">{formatCapacity(getValue() as number)}</span>,
	},
	{
		accessorKey: "mount",
		sortingFn: (a, b) => (a.original.mount ?? "").localeCompare(b.original.mount ?? ""),
		header: ({ column }) => <HeaderButton column={column} name={t`Mountpoint`} Icon={HardDriveIcon} />,
		cell: ({ getValue }) => (
			<span className="font-mono text-xs text-muted-foreground">{(getValue() as string) || "-"}</span>
		),
	},
]

function DatasetTable({ datasets }: { datasets: ZfsDataset[] }) {
	const [filter, setFilter] = useState("")
	const filtered = useMemo(() => {
		if (!datasets) return []
		if (!filter) return datasets
		const needle = filter.toLowerCase()
		return datasets.filter((ds) => ds.name.toLowerCase().includes(needle))
	}, [datasets, filter])

	const table = useReactTable({
		data: filtered,
		columns: datasetColumns,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
	})

	if (!datasets?.length) return null
	return (
		<div>
			<div className="mb-2 flex items-center justify-between gap-4">
				<h3 className="text-base font-semibold">Datasets</h3>
				<div className="relative w-64 max-w-full">
					<Input
						placeholder={t`Filter...`}
						value={filter}
						onChange={(e) => setFilter(e.target.value)}
						className="px-4 w-full"
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
			</div>
			<div className="max-h-80 overflow-auto rounded-md border">
				<Table>
					<TableHeader className="sticky top-0 z-10">
						{table.getHeaderGroups().map((headerGroup) => (
							<TableRow key={headerGroup.id}>
								{headerGroup.headers.map((header) => (
									<TableHead key={header.id} className="px-2">
										{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
									</TableHead>
								))}
							</TableRow>
						))}
					</TableHeader>
					<TableBody>
						{table.getRowModel().rows.map((row) => (
							<TableRow key={row.id}>
								{row.getVisibleCells().map((cell) => (
									<TableCell key={cell.id} className="ps-5 whitespace-pre">
										{flexRender(cell.column.columnDef.cell, cell.getContext())}
									</TableCell>
								))}
							</TableRow>
						))}
					</TableBody>
				</Table>
			</div>
		</div>
	)
}

function PoolSheet({
	poolId,
	open,
	onOpenChange,
}: {
	poolId: string | null
	open: boolean
	onOpenChange: (open: boolean) => void
}) {
	const [pool, setPool] = useState<ZfsPoolRecord | null>(null)
	const [isLoading, setIsLoading] = useState(false)

	useEffect(() => {
		let active = true
		if (!poolId) {
			setPool(null)
			return
		}
		// Only fetch when opening, not when closing (keeps data visible during close animation)
		if (!open) return
		setIsLoading(true)
		pb.collection("zfs_pools")
			.getOne(poolId)
			.then((record) => active && setPool(record as ZfsPoolRecord))
			.catch(() => active && setPool(null))
			.finally(() => active && setIsLoading(false))
		return () => {
			active = false
		}
	}, [open, poolId])

	const health = pool?.health || ""
	const healthVariantValue = healthVariant(health)
	const HealthIcon =
		healthVariantValue === "success"
			? CheckCircleIcon
			: healthVariantValue === "warning"
				? CircleAlertIcon
				: XCircleIcon

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-220 gap-0 overflow-y-auto">
				<SheetHeader className="mb-0 border-b">
					<SheetTitle className="flex items-center gap-2">
						{pool ? pool.name : `ZFS Pool`}
						{pool && <Badge variant={healthVariantValue}>{health}</Badge>}
					</SheetTitle>
					<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
						{pool?.size ? formatCapacity(pool.size) : null}
						{pool?.alloc ? (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<span>
									<Trans>Used</Trans>: {formatCapacity(pool.alloc)}
								</span>
							</>
						) : null}
						{pool?.free ? (
							<>
								<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
								<span>
									<Trans context="Free space">Free</Trans>: {formatCapacity(pool.free)}
								</span>
							</>
						) : null}
					</SheetDescription>
				</SheetHeader>
				<div className="flex-1 p-4 flex flex-col gap-4">
					{isLoading ? (
						<div className="flex justify-center py-8">
							<LoaderCircleIcon className="animate-spin size-10 opacity-60" />
						</div>
					) : (
						<>
							{pool && health && (
								<Alert className="pb-3 shrink-0">
									<HealthIcon className="size-4" />
									<AlertTitle>
										<Trans>Pool Health</Trans>: {health}
									</AlertTitle>
									{pool.scrub?.state && (
										<AlertDescription>
											Scrub: {pool.scrub.state}
											{pool.scrub.progress ? ` (${pool.scrub.progress})` : ""}
											{pool.scrub.errors ? `, ${pool.scrub.errors} errors` : ""}
										</AlertDescription>
									)}
								</Alert>
							)}
							{pool?.vdevs?.length || pool?.datasets?.length ? (
								<>
									{pool.vdevs?.length ? <VdevTable vdevs={pool.vdevs} /> : null}
									<DatasetTable datasets={pool.datasets ?? []} />
								</>
							) : (
								!isLoading && (
									<div className="py-8 text-center text-sm text-muted-foreground">
										<Trans>No detail data for this pool.</Trans>
									</div>
								)
							)}
						</>
					)}
				</div>
			</SheetContent>
		</Sheet>
	)
}

export default function ZfsTable({ systemId }: { systemId?: string }) {
	const [zfsPools, setZfsPools] = useState<ZfsPoolRecord[]>()
	const [globalFilter, setGlobalFilter] = useState("")
	const [activePoolId, setActivePoolId] = useState<string | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const [refreshingId, setRefreshingId] = useState<string | null>(null)

	useEffect(() => {
		let disposed = false
		let unsubscribe: () => void = () => {}
		// fetch initial records
		pb.collection<ZfsPoolRecord>("zfs_pools")
			.getFullList({
				filter: systemId ? pb.filter("system={:id}", { id: systemId }) : "",
				sort: "name",
				fields: ZFS_POOL_FIELDS,
			})
			.then((records) => !disposed && setZfsPools(records))
			.catch((error) => console.error("Failed to fetch ZFS pools:", error))

		// subscribe to realtime updates
		const pbOptions = systemId ? { filter: `system="${systemId}"` } : undefined
		;(async () => {
			try {
				const unsubscribeNow = await pb.collection<ZfsPoolRecord>("zfs_pools").subscribe(
					"*",
					(event) => {
						const record = event.record as ZfsPoolRecord
						setZfsPools((current) => {
							const pools = current ?? []
							const matchesSystemScope = !systemId || record.system === systemId
							if (event.action === "delete") {
								return pools.filter((pool) => pool.id !== record.id)
							}
							if (!matchesSystemScope) {
								return pools.filter((pool) => pool.id !== record.id)
							}
							const existingIndex = pools.findIndex((pool) => pool.id === record.id)
							if (existingIndex === -1) {
								return [record, ...pools]
							}
							const next = [...pools]
							next[existingIndex] = record
							return next
						})
					},
					pbOptions
				)
				if (disposed) {
					unsubscribeNow()
				} else {
					unsubscribe = unsubscribeNow
				}
			} catch (error) {
				console.error("Failed to subscribe to ZFS pool updates:", error)
			}
		})()

		return () => {
			disposed = true
			unsubscribe?.()
		}
	}, [systemId])

	const refreshSystem = useCallback(async (systemId: string) => {
		try {
			await pb.send("/api/beszel/zfs/refresh", {
				method: "POST",
				query: { system: systemId },
			})
		} catch (error) {
			console.error("Failed to refresh ZFS pools:", error)
		}
	}, [])

	const handleRowRefresh = useCallback(
		async (pool: ZfsPoolRecord) => {
			if (!pool.system) return
			setRefreshingId(pool.id)
			try {
				await refreshSystem(pool.system)
			} finally {
				setRefreshingId((id) => (id === pool.id ? null : id))
			}
		},
		[refreshSystem]
	)

	const actionColumn = useMemo<ColumnDef<ZfsPoolRecord>>(
		() => ({
			id: "actions",
			enableSorting: false,
			header: () => (
				<span className="sr-only">
					<Trans>Actions</Trans>
				</span>
			),
			cell: ({ row }) => {
				const pool = row.original
				const isRowRefreshing = refreshingId === pool.id

				return (
					<div className="flex justify-end">
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									variant="ghost"
									size="icon"
									className="size-10"
									onClick={(event) => event.stopPropagation()}
									onMouseDown={(event) => event.stopPropagation()}
								>
									<span className="sr-only">
										<Trans>Open menu</Trans>
									</span>
									<MoreHorizontalIcon className="w-5" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" onClick={(event) => event.stopPropagation()}>
								<DropdownMenuItem
									onClick={(event) => {
										event.stopPropagation()
										handleRowRefresh(pool)
									}}
									disabled={isRowRefreshing}
								>
									<RefreshCwIcon className={cn("me-2.5 size-4", isRowRefreshing && "animate-spin")} />
									<Trans>Refresh</Trans>
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				)
			},
		}),
		[refreshingId, handleRowRefresh]
	)

	const tableColumns = useMemo(() => {
		return isReadOnlyUser() ? columns : [...columns, actionColumn]
	}, [actionColumn])

	const table = useReactTable({
		data: zfsPools || ([] as ZfsPoolRecord[]),
		columns: tableColumns,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		state: { globalFilter },
		onGlobalFilterChange: setGlobalFilter,
		globalFilterFn: (row, _columnId, filterValue) => {
			const pool = row.original
			const searchString = `${pool.name} ${pool.health ?? ""}`.toLowerCase()
			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})
	const rows = table.getRowModel().rows

	// Hide the table on system pages if there's no data
	if (systemId && !zfsPools?.length && !globalFilter) {
		return null
	}

	const openSheet = (pool: ZfsPoolRecord) => {
		setActivePoolId(pool.id)
		setSheetOpen(true)
	}

	return (
		<div>
			<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
				<CardHeader className="p-0 mb-3 sm:mb-4">
					<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
						<div className="px-2 sm:px-1">
							<CardTitle className="mb-2">ZFS</CardTitle>
							<CardDescription className="flex">
								<Trans>Click on a pool to view vdev and dataset details.</Trans>
							</CardDescription>
						</div>
						<div className="relative ms-auto w-full max-w-full md:w-64">
							<Input
								placeholder={t`Filter...`}
								value={globalFilter}
								onChange={(event) => setGlobalFilter(event.target.value)}
								className="px-4 w-full max-w-full md:w-64"
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
				<div className="h-min max-h-[calc(100dvh-17rem)] max-w-full relative overflow-auto rounded-md border">
					<Table>
						<TableHeader className="sticky top-0 z-50 w-full border-b-2">
							{table.getHeaderGroups().map((headerGroup) => (
								<TableRow key={headerGroup.id}>
									{headerGroup.headers.map((header) => (
										<TableHead key={header.id} className="px-2">
											{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
										</TableHead>
									))}
								</TableRow>
							))}
						</TableHeader>
						<TableBody>
							{rows.map((row) => (
								<TableRow
									key={row.id}
									data-state={row.getIsSelected() && "selected"}
									className="cursor-pointer"
									onClick={() => openSheet(row.original)}
								>
									{row.getVisibleCells().map((cell) => (
										<TableCell key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>
									))}
								</TableRow>
							))}
						</TableBody>
					</Table>
				</div>
			</Card>
			<PoolSheet poolId={activePoolId} open={sheetOpen} onOpenChange={setSheetOpen} />
		</div>
	)
}
