import {
	CellContext,
	ColumnDef,
	ColumnFiltersState,
	getFilteredRowModel,
	SortingState,
	getSortedRowModel,
	flexRender,
	VisibilityState,
	getCoreRowModel,
	getPaginationRowModel,
	useReactTable,
	HeaderContext,
	Row,
	Table as TableType,
} from "@tanstack/react-table"

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

import { Button, buttonVariants } from "@/components/ui/button"

import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"

import { Collapsible } from "@/components/ui/collapsible"

import { TailscaleNode } from "@/types"
import {
	MoreHorizontalIcon,
	ArrowUpDownIcon,
	CopyIcon,
	WifiIcon,
	ServerIcon,
	MonitorIcon,
	SmartphoneIcon,
	ClockIcon,
	TagIcon,
	RefreshCwIcon as UpdateIcon,
	LayoutGridIcon,
	LayoutListIcon,
	Settings2Icon,
	AppleIcon,
	ZapIcon,
	RouterIcon,
	CalendarIcon,
	UserIcon,
	TimerIcon,
	ArrowDownIcon,
	ArrowUpIcon,
	EyeIcon,
	ChevronDownIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	ChevronsLeftIcon,
	ChevronsRightIcon,
} from "lucide-react"
import { memo, useEffect, useMemo, useState } from "react"
import { $tailscaleNodes, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import {
	cn,
	copyToClipboard,
	useLocalStorage,
} from "@/lib/utils"
import { useLingui, Trans } from "@lingui/react/macro"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { EthernetIcon, WindowsIcon, TuxIcon } from "../ui/icons"
import { Input } from "../ui/input"
import { Label } from "../ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select"
import { ClassValue } from "clsx"

type ViewMode = "table" | "grid"

function getOSIcon(os: string) {
	const lowerOS = os.toLowerCase()
	if (lowerOS.includes("linux")) return <TuxIcon className="h-4 w-4" />
	if (lowerOS.includes("macos") || lowerOS.includes("darwin")) return <AppleIcon className="h-4 w-4" />
	if (lowerOS.includes("windows")) return <WindowsIcon className="h-4 w-4" />
	if (lowerOS.includes("ios")) return <AppleIcon className="h-4 w-4" />
	if (lowerOS.includes("android")) return <SmartphoneIcon className="h-4 w-4" />
	if (lowerOS.includes("tvos")) return <AppleIcon className="h-4 w-4" />
	return <ServerIcon className="h-4 w-4" />
}

function formatLastSeen(lastSeen: string) {
	const date = new Date(lastSeen)
	const now = new Date()
	const diffMs = now.getTime() - date.getTime()
	const diffMins = Math.floor(diffMs / 60000)
	const diffHours = Math.floor(diffMins / 60)
	const diffDays = Math.floor(diffHours / 24)

	if (diffMins < 1) return "Just now"
	if (diffMins < 60) return `${diffMins}m ago`
	if (diffHours < 24) return `${diffHours}h ago`
	return `${diffDays}d ago`
}

function truncateTailnetName(name: string) {
	// If the name contains a tailnet domain (e.g., "apprise.tail43c135.ts.net")
	// truncate it to just the hostname part (e.g., "apprise")
	if (name.includes(".")) {
		return name.split(".")[0]
	}
	return name
}

function truncateVersion(version: string) {
	// If the version contains a dash, truncate it to just the part before the dash
	// e.g., "1.54.0-1234567890abcdef" becomes "1.54.0"
	if (version.includes("-")) {
		return version.split("-")[0]
	}
	return version
}

function sortableHeader(context: HeaderContext<TailscaleNode, unknown>) {
	const { column } = context
	// @ts-ignore
	const { Icon, hideSort, name }: { Icon: React.ElementType; name: () => string; hideSort: boolean } = column.columnDef
	return (
		<Button
			variant="ghost"
			className="h-9 px-3 flex"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{Icon && <Icon className="me-2 size-4" />}
			{name()}
			{hideSort || <ArrowUpDownIcon className="ms-2 size-4" />}
		</Button>
	)
}

export default function TailscaleTable() {
	const nodes = useStore($tailscaleNodes)
	const { t } = useLingui()
	const [sorting, setSorting] = useState<SortingState>([])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
	const [rowSelection, setRowSelection] = useState({})
	const [globalFilter, setGlobalFilter] = useState("")
	const [viewMode, setViewMode] = useLocalStorage<ViewMode>("tailscale-view-mode", "table")
	const [statusFilter, setStatusFilter] = useState<string>("all")

	const [selectedNode, setSelectedNode] = useState<TailscaleNode | null>(null)
	const [dialogOpen, setDialogOpen] = useState(false)
	const [pagination, setPagination] = useState({
		pageIndex: 0,
		pageSize: 10,
	})

	const handleNodeClick = (node: TailscaleNode) => {
		setSelectedNode(node)
		setDialogOpen(true)
	}

	// Fetch Tailscale data on component mount
	useEffect(() => {
		const fetchTailscaleData = async () => {
			try {
				const apiUrl = `${window.location.origin}/api/beszel/tailscale/nodes`
				const response = await fetch(apiUrl, {
					headers: {
						Authorization: pb.authStore.token,
					},
				})
				if (response.ok) {
					const data = await response.json()
					$tailscaleNodes.set(data)
				}
			} catch (error) {
				console.error("Failed to fetch Tailscale data:", error)
			}
		}

		fetchTailscaleData()
		// Refresh every 30 seconds
		const interval = setInterval(fetchTailscaleData, 30000)
		return () => clearInterval(interval)
	}, [])

			const columns = useMemo<ColumnDef<TailscaleNode>[]>(
		() => [
			{
				size: 200,
				minSize: 0,
				accessorKey: "name",
				id: "name",
				name: () => "Name",
				enableHiding: false,
				invertSorting: false,
				Icon: ServerIcon,
				cell: (info: CellContext<TailscaleNode, unknown>) => {
					const node = info.row.original
					return (
						<span className="flex gap-2 items-center md:ps-1 md:pe-5">
							<TailscaleIndicatorDot node={node} />
							<span className="font-medium text-sm" title={node.name}>
								{truncateTailnetName(node.name)}
							</span>
						</span>
					)
				},
				header: sortableHeader,
			},
			{
				accessorKey: "ip",
				id: "ip",
				name: () => "IP",
				cell: (info: CellContext<TailscaleNode, unknown>) => {
					const node = info.row.original
					return (
						<div className="flex flex-col">
							<span className="text-muted-foreground tabular-nums">{node.ip}</span>
							{node.ipv6 && <span className="text-muted-foreground text-xs text-muted-foreground tabular-nums">{node.ipv6}</span>}
						</div>
					)
				},
				Icon: EthernetIcon,
				header: sortableHeader,
			},
			{
				accessorKey: "os",
				id: "os",
				name: () => "OS",
				cell: (info: CellContext<TailscaleNode, unknown>) => {
					const node = info.row.original
					return (
						<div className="flex items-center gap-2">
							{getOSIcon(node.os)}
							<span className="text-sm tabular-nums">{node.os}</span>
						</div>
					)
				},
				Icon: MonitorIcon,
				header: sortableHeader,
			},
						{
				accessorKey: "version",
				id: "version",
				name: () => "Version",
				cell: (info: CellContext<TailscaleNode, unknown>) => {
					const node = info.row.original
					return (
						<span className="flex gap-2 items-center md:pe-5 tabular-nums">
							<TailscaleIndicatorDot
								node={node}
								className={
									(!node.online && "bg-primary/30") ||
									(node.updateAvailable && "bg-yellow-500") ||
									"bg-green-500"
								}
							/>
							<span className="truncate max-w-14 tabular-nums" title={node.version}>{truncateVersion(node.version)}</span>
						</span>
					)
				},
				Icon: WifiIcon,
				header: sortableHeader,
			},
			{
				accessorKey: "lastSeen",
				id: "lastSeen",
				name: () => "Last Seen",
				cell: (info: CellContext<TailscaleNode, unknown>) => {
					const node = info.row.original
					return (
						<div className="flex items-center gap-2">
							<span className="text-sm tabular-nums">
								{node.online ? "Online" : formatLastSeen(node.lastSeen)}
							</span>
						</div>
					)
				},
				Icon: ClockIcon,
				header: sortableHeader,
			},
			{
				accessorKey: "tags",
				id: "tags",
				name: () => "Tags",
				cell: (info: CellContext<TailscaleNode, unknown>) => {
					const node = info.row.original
					if (!node.tags || node.tags.length === 0) return null
					return (
						<div className="flex flex-wrap gap-1">
							{node.tags.slice(0, 2).map((tag, index) => (
								<span
									key={index}
									className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-1 text-xs"
								>
									<TagIcon className="h-3 w-3" />
									{tag.replace("tag:", "")}
								</span>
							))}
							{node.tags.length > 2 && (
								<span className="text-xs text-muted-foreground">+{node.tags.length - 2}</span>
							)}
						</div>
					)
				},
				Icon: TagIcon,
				header: sortableHeader,
			},
			{
				id: "actions",
				enableHiding: false,
				cell: (info: CellContext<TailscaleNode, unknown>) => {
					const node = info.row.original
					return (
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="ghost" className="h-8 w-8 p-0" data-nolink>
									<span className="sr-only">Open menu</span>
									<MoreHorizontalIcon className="h-4 w-4" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuLabel>Actions</DropdownMenuLabel>
								<DropdownMenuItem
									onClick={() => copyToClipboard(node.ip)}
									className="cursor-pointer"
								>
									<CopyIcon className="mr-2 h-4 w-4" />
									<Trans>Copy IP</Trans>
								</DropdownMenuItem>
								<DropdownMenuItem
									onClick={() => copyToClipboard(node.name)}
									className="cursor-pointer"
								>
									<CopyIcon className="mr-2 h-4 w-4" />
									<Trans>Copy Name</Trans>
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					)
				},
			},
		],
		[]
	)



	// Filter nodes based on status filter
	const filteredNodes = useMemo(() => {
		return nodes.filter((node) => {
			if (statusFilter === "all") return true
			if (statusFilter === "online") return node.online
			if (statusFilter === "offline") return !node.online
			return true
		})
	}, [nodes, statusFilter])

	const table = useReactTable({
		data: filteredNodes,
		columns,
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		onColumnVisibilityChange: setColumnVisibility,
		onRowSelectionChange: setRowSelection,
		onGlobalFilterChange: setGlobalFilter,
		onPaginationChange: setPagination,
		globalFilterFn: (row, columnId, filterValue) => {
		const searchValue = filterValue.toLowerCase()
		
		// Search across all columns in the row
		for (const column of row.getAllCells()) {
			const value = column.getValue()
			
			// Special handling for tags column
			if (column.column.id === "tags" && Array.isArray(value)) {
				const hasMatchingTag = value.some(tag => 
					tag.toLowerCase().includes(searchValue) || 
					tag.replace("tag:", "").toLowerCase().includes(searchValue)
				)
				if (hasMatchingTag) return true
			}
			
			// Default string search for other columns
			if (typeof value === "string") {
				if (value.toLowerCase().includes(searchValue)) return true
			}
			
			// For other types, convert to string and search
			if (String(value).toLowerCase().includes(searchValue)) return true
		}
		
		return false
	},
		state: {
			sorting,
			columnFilters,
			columnVisibility,
			rowSelection,
			globalFilter,
			pagination,
		},
	})

	const visibleColumns = table.getVisibleLeafColumns()
	const rows = table.getRowModel().rows
	const allColumns = table.getAllColumns()

	const CardHead = useMemo(() => {
		return (
			<CardHeader className="pb-5 px-2 sm:px-6 max-sm:pt-5 max-sm:pb-1">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2.5">
							<Trans>Tailscale Network</Trans>
						</CardTitle>
						<CardDescription>
							<Trans>Updated in real time. Click on a node to view information.</Trans>
						</CardDescription>
					</div>
					

					
					<div className="flex gap-2 ms-auto w-full md:w-80">
						<Input placeholder={t`Filter...`} onChange={(e) => setGlobalFilter(e.target.value)} className="px-4" />
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline">
									<Settings2Icon className="me-1.5 size-4 opacity-80" />
									<Trans>View</Trans>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
								<div className="grid grid-cols-1 md:grid-cols-4 divide-y md:divide-s md:divide-y-0">
									<div>
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<LayoutGridIcon className="size-4" />
											<Trans>Layout</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<DropdownMenuRadioGroup
											className="px-1 pb-1"
											value={viewMode}
											onValueChange={(view) => setViewMode(view as ViewMode)}
										>
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
											<ArrowUpDownIcon className="size-4" />
											<Trans>Sort By</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<div className="px-1 pb-1">
											{allColumns.map((column) => {
												if (!column.getCanSort()) return null
												let Icon = <span className="w-6"></span>
												// if current sort column, show sort direction
												if (sorting[0]?.id === column.id) {
													if (sorting[0]?.desc) {
														Icon = <ArrowUpIcon className="me-2 size-4" />
													} else {
														Icon = <ArrowDownIcon className="me-2 size-4" />
													}
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
														{/* @ts-ignore */}
														{column.columnDef.name()}
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
											{allColumns
												.filter((column) => column.getCanHide())
												.map((column) => {
													return (
														<DropdownMenuCheckboxItem
															key={column.id}
															onSelect={(e) => e.preventDefault()}
															checked={column.getIsVisible()}
															onCheckedChange={(value) => column.toggleVisibility(!!value)}
														>
															{/* @ts-ignore */}
															{column.columnDef.name()}
														</DropdownMenuCheckboxItem>
													)
												})}
										</div>
									</div>

									<div>
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<WifiIcon className="size-4" />
											<Trans>Status</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<DropdownMenuRadioGroup
											className="px-1 pb-1"
											value={statusFilter}
											onValueChange={(value) => setStatusFilter(value)}
										>
											<DropdownMenuRadioItem value="all" className="gap-2">
												<Trans>All</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="online" className="gap-2">
												<Trans>Online</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="offline" className="gap-2">
												<Trans>Offline</Trans>
											</DropdownMenuRadioItem>
										</DropdownMenuRadioGroup>
									</div>
								</div>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</CardHeader>
		)
	}, [visibleColumns.length, sorting, viewMode, allColumns, statusFilter])

	return (
		<Card>
			{CardHead}
			<div className="p-6 pt-0 max-sm:py-3 max-sm:px-2">
				{viewMode === "table" ? (
					// table layout
					<div className="rounded-md border overflow-hidden">
						<AllTailscaleTable table={table} rows={rows} colLength={visibleColumns.length} onNodeClick={handleNodeClick} />
					</div>
				) : (
					// grid layout
					<div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
						{rows?.length ? (
							rows.map((row) => {
								return <TailscaleCard key={row.original.id} row={row} table={table} colLength={visibleColumns.length} onNodeClick={handleNodeClick} />
							})
						) : (
							<div className="col-span-full text-center py-8">
								<Trans>No nodes found.</Trans>
							</div>
						)}
					</div>
				)}
			</div>
			
			{/* Pagination Controls */}
			{viewMode === "table" && (
				<div className="flex items-center justify-end px-4 pb-4">
					<div className="flex items-center gap-8">
						<div className="hidden items-center gap-2 lg:flex">
							<Label htmlFor="rows-per-page" className="text-sm font-medium">
								<Trans>Rows per page</Trans>
							</Label>
							<Select
								value={`${table.getState().pagination.pageSize}`}
								onValueChange={(value) => {
									table.setPageSize(Number(value))
								}}
							>
								<SelectTrigger className="w-20" id="rows-per-page">
									<SelectValue
										placeholder={table.getState().pagination.pageSize}
									/>
								</SelectTrigger>
								<SelectContent side="top">
									{[10, 20, 30, 40, 50].map((pageSize) => (
										<SelectItem key={pageSize} value={`${pageSize}`}>
											{pageSize}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						<div className="flex w-fit items-center justify-center text-sm font-medium">
							<Trans>Page {table.getState().pagination.pageIndex + 1} of {table.getPageCount()}</Trans>
						</div>
						<div className="ml-auto flex items-center gap-2 lg:ml-0">
							<Button
								variant="outline"
								className="hidden h-8 w-8 p-0 lg:flex"
								onClick={() => table.setPageIndex(0)}
								disabled={!table.getCanPreviousPage()}
							>
								<span className="sr-only">Go to first page</span>
								<ChevronsLeftIcon className="h-4 w-4" />
							</Button>
							<Button
								variant="outline"
								className="size-8"
								size="icon"
								onClick={() => table.previousPage()}
								disabled={!table.getCanPreviousPage()}
							>
								<span className="sr-only">Go to previous page</span>
								<ChevronLeftIcon className="h-4 w-4" />
							</Button>
							<Button
								variant="outline"
								className="size-8"
								size="icon"
								onClick={() => table.nextPage()}
								disabled={!table.getCanNextPage()}
							>
								<span className="sr-only">Go to next page</span>
								<ChevronRightIcon className="h-4 w-4" />
							</Button>
							<Button
								variant="outline"
								className="hidden size-8 lg:flex"
								size="icon"
								onClick={() => table.setPageIndex(table.getPageCount() - 1)}
								disabled={!table.getCanNextPage()}
							>
								<span className="sr-only">Go to last page</span>
								<ChevronsRightIcon className="h-4 w-4" />
							</Button>
						</div>
					</div>
				</div>
			)}
			
			{/* Node Details Dialog */}
			{selectedNode && (
				<TailscaleNodeDialog 
					node={selectedNode} 
					open={dialogOpen} 
					setOpen={setDialogOpen} 
				/>
			)}
		</Card>
	)
}

const AllTailscaleTable = memo(
	({ table, rows, colLength, onNodeClick }: { table: TableType<TailscaleNode>; rows: Row<TailscaleNode>[]; colLength: number; onNodeClick: (node: TailscaleNode) => void }) => {
		return (
			<Table>
				<TailscaleTableHead table={table} colLength={colLength} />
				<TableBody>
					{rows.length ? (
						rows.map((row) => (
							<TailscaleTableRow key={row.original.id} row={row} length={rows.length} colLength={colLength} onNodeClick={onNodeClick} />
						))
					) : (
						<TableRow>
							<TableCell colSpan={colLength} className="h-24 text-center">
								<Trans>No nodes found.</Trans>
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
		)
	}
)

function TailscaleTableHead({ table, colLength }: { table: TableType<TailscaleNode>; colLength: number }) {
	const { i18n } = useLingui()
	return useMemo(() => {
		return (
			<TableHeader>
				{table.getHeaderGroups().map((headerGroup) => (
					<TableRow key={headerGroup.id}>
						{headerGroup.headers.map((header) => {
							return (
								<TableHead className="px-1.5" key={header.id}>
									{flexRender(header.column.columnDef.header, header.getContext())}
								</TableHead>
							)
						})}
					</TableRow>
				))}
			</TableHeader>
		)
	}, [i18n.locale, colLength])
}

const TailscaleTableRow = memo(
	({ row, length, colLength, onNodeClick }: { row: Row<TailscaleNode>; length: number; colLength: number; onNodeClick: (node: TailscaleNode) => void }) => {
		const node = row.original
		const { t } = useLingui()
		return useMemo(() => {
			return (
				<TableRow
					className={cn("cursor-pointer transition-opacity")}
					onClick={() => onNodeClick(node)}
				>
					{row.getVisibleCells().map((cell) => (
						<TableCell
							key={cell.id}
							style={{
								width: cell.column.getSize(),
							}}
							className={cn("overflow-hidden relative", length > 10 ? "py-2" : "py-2.5")}
						>
							{flexRender(cell.column.columnDef.cell, cell.getContext())}
						</TableCell>
					))}
				</TableRow>
			)
		}, [node, colLength, t])
	}
)

const TailscaleCard = memo(
	({ row, table, colLength, onNodeClick }: { row: Row<TailscaleNode>; table: TableType<TailscaleNode>; colLength: number; onNodeClick: (node: TailscaleNode) => void }) => {
		const node = row.original
		const { t } = useLingui()

		return useMemo(() => {
			return (
				<Card
					key={node.id}
					className="cursor-pointer hover:shadow-md transition-all bg-transparent w-full dark:border-border duration-200 relative"
					onClick={() => onNodeClick(node)}
				>
					<CardHeader className="py-1 ps-5 pe-3 bg-muted/30 border-b border-border/60">
						<div className="flex items-center justify-between gap-2">
							<CardTitle className="text-base tracking-normal shrink-1 text-primary/90 flex items-center min-h-10 gap-2.5 min-w-0">
								<div className="flex items-center gap-2.5 min-w-0">
									<TailscaleIndicatorDot node={node} />
									<CardTitle className="text-[.95em]/normal tracking-normal truncate text-primary/90">
										{truncateTailnetName(node.name)}
									</CardTitle>
								</div>
							</CardTitle>
							{table.getColumn("actions")?.getIsVisible() && (
								<div className="flex gap-1 flex-shrink-0 relative z-10">
									<ActionsButton node={node} />
								</div>
							)}
						</div>
					</CardHeader>
					<CardContent className="space-y-2.5 text-sm px-5 pt-3.5 pb-4">
						{table.getAllColumns().map((column) => {
							if (!column.getIsVisible() || column.id === "name" || column.id === "actions") return null
							const cell = row.getAllCells().find((cell) => cell.column.id === column.id)
							if (!cell) return null
							// @ts-ignore
							const { Icon, name } = column.columnDef as ColumnDef<TailscaleNode, unknown>
							return (
								<div key={column.id} className="flex items-center gap-3">
									{Icon && <Icon className="size-4 text-muted-foreground" />}
									<div className="flex items-center gap-3 flex-1">
										<span className="text-muted-foreground min-w-16">{name()}:</span>
										<div className="flex-1">{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>
									</div>
								</div>
							)
						})}
					</CardContent>
				</Card>
			)
		}, [node, colLength, t])
	}
)

const ActionsButton = memo(({ node }: { node: TailscaleNode }) => {
	const { t } = useLingui()
	const { id, name, ip } = node

	return useMemo(() => {
		return (
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="ghost" size={"icon"} data-nolink>
						<span className="sr-only">
							<Trans>Open menu</Trans>
						</span>
						<MoreHorizontalIcon className="w-5" />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem onClick={() => copyToClipboard(ip)}>
						<CopyIcon className="me-2.5 size-4" />
						<Trans>Copy IP</Trans>
					</DropdownMenuItem>
					<DropdownMenuItem onClick={() => copyToClipboard(name)}>
						<CopyIcon className="me-2.5 size-4" />
						<Trans>Copy name</Trans>
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		)
	}, [id, name, ip, t])
})

function TailscaleIndicatorDot({ node, className }: { node: TailscaleNode; className?: ClassValue }) {
	className ||= {
		"bg-green-500": node.online,
		"bg-red-500": !node.online,
	}
	return (
		<span
			className={cn("flex-shrink-0 size-2 rounded-full", className)}
		/>
	)
}

function TailscaleNodeDialog({ node, open, setOpen }: { node: TailscaleNode; open: boolean; setOpen: (open: boolean) => void }) {
	const { t } = useLingui()
	
	const formatDate = (dateString: string) => {
		return new Date(dateString).toLocaleString()
	}
	
	const getStatusColor = (online: boolean) => {
		return "text-foreground"
	}
	
	const getStatusText = (online: boolean) => {
		return online ? "Online" : "Offline"
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						<TailscaleIndicatorDot node={node} />
						<span>{node.name}</span>
					</DialogTitle>
					<DialogDescription>
						Detailed information about this Tailscale node
					</DialogDescription>
				</DialogHeader>
				
				<div className="space-y-4">
					{/* Basic Information */}
					<Collapsible title="Basic Information" icon={<ServerIcon className="h-4 w-4" />} defaultOpen={true}>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Status:</span>
							<span className={getStatusColor(node.online)}>{getStatusText(node.online)}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Hostname:</span>
							<span className="text-muted-foreground">{node.hostname}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Computed Name:</span>
							<span className="text-muted-foreground">{node.computedName}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Operating System:</span>
							<span>{node.os}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Version:</span>
							<span className="text-muted-foreground">{node.version}</span>
						</div>
					</Collapsible>

					{/* Network Information */}
					<Collapsible title="Network Information" icon={<WifiIcon className="h-4 w-4" />}>
						<div className="flex justify-between">
							<span className="text-muted-foreground">IP Address:</span>
							<span className="text-muted-foreground">{node.ip}</span>
						</div>
						{node.ipv6 && (
							<div className="flex justify-between">
								<span className="text-muted-foreground">IPv6 Address:</span>
								<span className="text-muted-foreground">{node.ipv6}</span>
							</div>
						)}
						<div className="flex justify-between">
							<span className="text-muted-foreground">DERP Server:</span>
							<span className="text-muted-foreground">{node.derp}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Last Seen:</span>
							<span>{formatDate(node.lastSeen)}</span>
						</div>
					</Collapsible>

					{/* Node Properties */}
					<Collapsible title="Node Properties" icon={<Settings2Icon className="h-4 w-4" />}>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Exit Node:</span>
							<span className="text-foreground">
								{node.isExitNode ? "Yes" : "No"}
							</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Subnet Router:</span>
							<span className="text-foreground">
								{node.isSubnetRouter ? "Yes" : "No"}
							</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Ephemeral:</span>
							<span className="text-foreground">
								{node.isEphemeral ? "Yes" : "No"}
							</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Connected to Control:</span>
							<span className="text-foreground">
								{node.connectedToControl ? "Yes" : "No"}
							</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Update Available:</span>
							<span className="text-foreground">
								{node.updateAvailable ? "Yes" : "No"}
							</span>
						</div>
					</Collapsible>

					{/* Security & Keys */}
					<Collapsible title="Security & Keys" icon={<CalendarIcon className="h-4 w-4" />}>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Key Expiry:</span>
							<span className="text-muted-foreground">{formatDate(node.keyExpiry)}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Expired:</span>
							<span className="text-foreground">
								{node.expired ? "Yes" : "No"}
							</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Key Expired:</span>
							<span className="text-foreground">
								{node.keyExpired ? "Yes" : "No"}
							</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Created:</span>
							<span>{formatDate(node.created)}</span>
						</div>
					</Collapsible>

					{/* Tags */}
					{node.tags && node.tags.length > 0 && (
						<Collapsible title="Tags" icon={<TagIcon className="h-4 w-4" />}>
							<div className="flex flex-wrap gap-2">
								{node.tags.map((tag, index) => (
									<span
										key={index}
										className="inline-flex items-center gap-1 rounded-full bg-muted px-3 py-1 text-sm"
									>
										<TagIcon className="h-3 w-3" />
										{tag.replace("tag:", "")}
									</span>
								))}
							</div>
						</Collapsible>
					)}

					{/* Advanced Information */}
					<Collapsible title="Advanced Information" icon={<EyeIcon className="h-4 w-4" />}>
						<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
							<div className="space-y-3">
								<div className="flex justify-between">
									<span className="text-muted-foreground">In Network Map:</span>
									<span className="text-foreground">
										{node.inNetworkMap ? "Yes" : "No"}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-muted-foreground">In Magic Sock:</span>
									<span className="text-foreground">
										{node.inMagicSock ? "Yes" : "No"}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-muted-foreground">In Engine:</span>
									<span className="text-foreground">
										{node.inEngine ? "Yes" : "No"}
									</span>
								</div>
							</div>
							<div className="space-y-3">
								{node.endpoints && node.endpoints.length > 0 && (
									<div>
										<span className="text-muted-foreground">Endpoints:</span>
										<div className="mt-1 space-y-1">
											{node.endpoints.map((endpoint, index) => (
												<div key={index} className="text-muted-foreground text-sm">{endpoint}</div>
											))}
										</div>
									</div>
								)}
							</div>
						</div>
					</Collapsible>
				</div>
			</DialogContent>
		</Dialog>
	)
} 