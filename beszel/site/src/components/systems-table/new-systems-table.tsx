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
	useReactTable,
	HeaderContext,
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
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"

import { AddSystemRecord, SystemRecord } from "@/types"
import {
	MoreHorizontalIcon,
	ArrowUpDownIcon,
	NetworkIcon,
	CopyIcon,
	PauseCircleIcon,
	PlayCircleIcon,
	Trash2Icon,
	ServerIcon,
	FingerprintIcon,
	LayoutGridIcon,
	LayoutListIcon,
	ArrowDownIcon,
	ArrowUpIcon,
	Settings2Icon,
	EyeIcon,
	PenBoxIcon,
} from "lucide-react"
import { memo, useEffect, useMemo, useRef, useState } from "react"
import { $hubVersion, $newSystems, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { cn, copyToClipboard, decimalString, isReadOnlyUser, useLocalStorage } from "@/lib/utils"
import { $router, Link, navigate } from "../router"
import { Trans, t } from "@lingui/macro"
import { useLingui } from "@lingui/react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { Input } from "../ui/input"
import { getPagePath } from "@nanostores/router"
import { SystemDialog } from "../add-system"
import { Dialog } from "../ui/dialog"

type ViewMode = "table" | "grid"

function sortableHeader(context: HeaderContext<SystemRecord, unknown>, hideSortIcon = false) {
	const { column } = context
	return (
		<Button
			variant="ghost"
			className="h-9 px-3 flex"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{/* @ts-ignore */}
			{column.columnDef?.icon && <column.columnDef.icon className="me-2 size-4" />}
			{column.id}
			{!hideSortIcon && <ArrowUpDownIcon className="ms-2 size-4" />}
		</Button>
	)
}

export default function SystemsTable() {
	const data = useStore($newSystems)
	const hubVersion = useStore($hubVersion)
	const [filter, setFilter] = useState<string>()
	const [sorting, setSorting] = useState<SortingState>([{ id: t`Hostname`, desc: false }])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useLocalStorage<VisibilityState>("cols", {})
	const [viewMode, setViewMode] = useLocalStorage<ViewMode>("viewMode", window.innerWidth > 1024 ? "table" : "grid")
	const { i18n } = useLingui()

	useEffect(() => {
		if (filter !== undefined) {
			table.getColumn(t`Hostname`)?.setFilterValue(filter)
		}
	}, [filter])

	const columns = useMemo(() => {
		return [
			{
				size: 200,
				accessorKey: "hostname",
				id: t`Hostname`,
				enableHiding: false,
				icon: ServerIcon,
				cell: (host) => {
					return host.getValue() as string
				},
				header: sortableHeader,
			},
			{
				accessorKey: "address",
				id: t`Address`,
				invertSorting: true,
				cell: (addr) => {
					return addr.getValue() as string
				},
				icon: NetworkIcon,
				header: sortableHeader,
			},
			{
				accessorKey: "fingerprint",
				id: t`Fingerprint`,
				invertSorting: true,
				cell: (fp) => (
					<span className="flex gap-0.5 items-center text-base md:pe-5">
						<Button
							data-nolink
							variant={"ghost"}
							className="text-primary/90 h-7 px-1.5 gap-1.5"
							onClick={() => copyToClipboard(fp.getValue() as string)}
						>
							{fp.getValue() as string}
							<CopyIcon className="h-2.5 w-2.5" />
						</Button>
					</span>
				),
				icon: FingerprintIcon,
				header: sortableHeader,
			},
			{
				id: t({ message: "Actions", comment: "Table column" }),
				size: 120,
				cell: ({ row }) => (
					<div className="flex justify-end items-center gap-1">
						test
						{/* <!-<ActionsButton system={row.original} />-> */}
					</div>
				),
			},
		] as ColumnDef<AddSystemRecord>[]
	}, [hubVersion, i18n.locale])

	const table = useReactTable({
		data,
		columns,
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
			minSize: 0,
			size: Number.MAX_SAFE_INTEGER,
			maxSize: Number.MAX_SAFE_INTEGER,
		},
	})

	return (
		<Card>
			<CardHeader className="pb-5 px-2 sm:px-6 max-sm:pt-5 max-sm:pb-1">
				<div className="grid md:flex gap-5 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2.5">
							<Trans>New Systems</Trans>
						</CardTitle>
						<CardDescription>
							<Trans>Allow or deny new systems for monitoring.</Trans>
						</CardDescription>
					</div>
					<div className="flex gap-2 ms-auto w-full md:w-80">
						<Input placeholder={t`Filter...`} onChange={(e) => setFilter(e.target.value)} className="px-4" />
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline">
									<Settings2Icon className="me-1.5 size-4 opacity-80" />
									<Trans>View</Trans>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
								<div className="grid grid-cols-1 md:grid-cols-3 divide-y md:divide-s md:divide-y-0">
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
											{table.getAllColumns().map((column) => {
												if (column.id === t`Actions` || !column.getCanSort()) return null
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
														{column.id}
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
												.map((column) => {
													return (
														<DropdownMenuCheckboxItem
															key={column.id}
															onSelect={(e) => e.preventDefault()}
															checked={column.getIsVisible()}
															onCheckedChange={(value) => column.toggleVisibility(!!value)}
														>
															{column.id}
														</DropdownMenuCheckboxItem>
													)
												})}
										</div>
									</div>
								</div>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</CardHeader>
			<div className="p-6 pt-0 max-sm:py-3 max-sm:px-2">
				{viewMode === "table" ? (
					// table layout
					<div className="rounded-md border overflow-hidden">
						<Table>
							<TableHeader>
								{table.getHeaderGroups().map((headerGroup) => (
									<TableRow key={headerGroup.id}>
										{headerGroup.headers.map((header) => {
											return (
												<TableHead className="px-2" key={header.id}>
													{header.isPlaceholder
														? null
														: flexRender(header.column.columnDef.header, header.getContext())}
												</TableHead>
											)
										})}
									</TableRow>
								))}
							</TableHeader>
							<TableBody>
								{table.getRowModel().rows?.length ? (
									table.getRowModel().rows.map((row) => (
										<TableRow
											key={row.original.id}
											data-state={row.getIsSelected() && "selected"}
											className={cn("cursor-pointer transition-opacity", {
												"opacity-50": row.original.status === "paused",
											})}
											onClick={(e) => {
												const target = e.target as HTMLElement
												if (!target.closest("[data-nolink]") && e.currentTarget.contains(target)) {
													navigate(getPagePath($router, "system", { name: row.original.name }))
												}
											}}
										>
											{row.getVisibleCells().map((cell) => (
												<TableCell
													key={cell.id}
													style={{
														width: cell.column.getSize() === Number.MAX_SAFE_INTEGER ? "auto" : cell.column.getSize(),
													}}
													className={cn("overflow-hidden relative", data.length > 10 ? "py-2" : "py-2.5")}
												>
													{flexRender(cell.column.columnDef.cell, cell.getContext())}
												</TableCell>
											))}
										</TableRow>
									))
								) : (
									<TableRow>
										<TableCell colSpan={columns.length} className="h-24 text-center">
											<Trans>No systems found.</Trans>
										</TableCell>
									</TableRow>
								)}
							</TableBody>
						</Table>
					</div>
				) : (
					// grid layout
					<div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
						{table.getRowModel().rows?.length ? (
							table.getRowModel().rows.map((row) => {
								const system = row.original
								const { status } = system
								return (
									<Card
										key={system.id}
										className={cn(
											"cursor-pointer hover:shadow-md transition-all bg-transparent w-full dark:border-border duration-200 relative",
											{
												"opacity-50": status === "paused",
											}
										)}
									>
										<CardHeader className="py-1 ps-5 pe-3 bg-muted/30 border-b border-border/60">
											<div className="flex items-center justify-between gap-2">
												<CardTitle className="text-base tracking-normal shrink-1 text-primary/90 flex items-center min-h-10 gap-2.5 min-w-0">
													<div className="flex items-center gap-2.5 min-w-0">
														<CardTitle className="text-[.95em]/normal tracking-normal truncate text-primary/90">
															{system.name}
														</CardTitle>
													</div>
												</CardTitle>
												{table.getColumn(t`Actions`)?.getIsVisible() && (
													<div className="flex gap-1 flex-shrink-0 relative z-10">
														test
														{/* <ActionsButton system={system} /> */}
													</div>
												)}
											</div>
										</CardHeader>
										<CardContent className="space-y-2.5 text-sm px-5 pt-3.5 pb-4">
											{table.getAllColumns().map((column) => {
												if (!column.getIsVisible() || column.id === t`System` || column.id === t`Actions`) return null
												const cell = row.getAllCells().find((cell) => cell.column.id === column.id)
												if (!cell) return null
												return (
													<div key={column.id} className="flex items-center gap-3">
														{/* @ts-ignore */}
														{column.columnDef?.icon && (
															// @ts-ignore
															<column.columnDef.icon className="size-4 text-muted-foreground" />
														)}
														<div className="flex items-center gap-3 flex-1">
															<span className="text-muted-foreground min-w-16">{column.id}:</span>
															<div className="flex-1">{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>
														</div>
													</div>
												)
											})}
										</CardContent>
										<Link
											href={getPagePath($router, "system", { name: row.original.name })}
											className="inset-0 absolute w-full h-full"
										>
											<span className="sr-only">{row.original.name}</span>
										</Link>
									</Card>
								)
							})
						) : (
							<div className="col-span-full text-center py-8">
								<Trans>No new systems found.</Trans>
							</div>
						)}
					</div>
				)}
			</div>
		</Card>
	)
}

const ActionsButton = memo(({ system }: { system: SystemRecord }) => {
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [editOpen, setEditOpen] = useState(false)
	let editOpened = useRef(false)

	const { id, status, host, name } = system

	return (
		<>
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
					{!isReadOnlyUser() && (
						<DropdownMenuItem
							onSelect={() => {
								editOpened.current = true
								setEditOpen(true)
							}}
						>
							<PenBoxIcon className="me-2.5 size-4" />
							<Trans>Edit</Trans>
						</DropdownMenuItem>
					)}
					<DropdownMenuItem
						className={cn(isReadOnlyUser() && "hidden")}
						onClick={() => {
							pb.collection("systems").update(id, {
								status: status === "paused" ? "pending" : "paused",
							})
						}}
					>
						{status === "paused" ? (
							<>
								<PlayCircleIcon className="me-2.5 size-4" />
								<Trans>Resume</Trans>
							</>
						) : (
							<>
								<PauseCircleIcon className="me-2.5 size-4" />
								<Trans>Pause</Trans>
							</>
						)}
					</DropdownMenuItem>
					<DropdownMenuItem onClick={() => copyToClipboard(host)}>
						<CopyIcon className="me-2.5 size-4" />
						<Trans>Copy host</Trans>
					</DropdownMenuItem>
					<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
					<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")} onSelect={() => setDeleteOpen(true)}>
						<Trash2Icon className="me-2.5 size-4" />
						<Trans>Delete</Trans>
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
			{/* edit dialog */}
			<Dialog open={editOpen} onOpenChange={setEditOpen}>
				{editOpened.current && <SystemDialog system={system} setOpen={setEditOpen} />}
			</Dialog>
			{/* deletion dialog */}
			<AlertDialog open={deleteOpen} onOpenChange={(open) => setDeleteOpen(open)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Are you sure you want to delete {name}?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>
								This action cannot be undone. This will permanently delete all current records for {name} from the
								database.
							</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn(buttonVariants({ variant: "destructive" }))}
							onClick={() => pb.collection("systems").delete(id)}
						>
							<Trans>Continue</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	)
})
