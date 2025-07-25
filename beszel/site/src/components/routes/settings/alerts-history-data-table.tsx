import { pb } from "@/lib/stores"
import { alertInfo, cn, formatDuration, formatShortDate } from "@/lib/utils"
import { AlertsHistoryRecord } from "@/types"
import {
	getCoreRowModel,
	getPaginationRowModel,
	getSortedRowModel,
	getFilteredRowModel,
	useReactTable,
	flexRender,
	ColumnFiltersState,
	SortingState,
	VisibilityState,
} from "@tanstack/react-table"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { alertsHistoryColumns } from "../../alerts-history-columns"
import { Checkbox } from "@/components/ui/checkbox"
import { memo, useEffect, useState } from "react"
import { Label } from "@/components/ui/label"
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select"
import {
	ChevronLeftIcon,
	ChevronRightIcon,
	ChevronsLeftIcon,
	ChevronsRightIcon,
	DownloadIcon,
	Trash2Icon,
} from "lucide-react"
import { Trans } from "@lingui/react/macro"
import { t } from "@lingui/core/macro"
import { useToast } from "@/components/ui/use-toast"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
	AlertDialogTrigger,
} from "@/components/ui/alert-dialog"

const SectionIntro = memo(() => {
	return (
		<div>
			<h3 className="text-xl font-medium mb-2">
				<Trans>Alert History</Trans>
			</h3>
			<p className="text-sm text-muted-foreground leading-relaxed">
				<Trans>View your 200 most recent alerts.</Trans>
			</p>
		</div>
	)
})

export default function AlertsHistoryDataTable() {
	const [data, setData] = useState<AlertsHistoryRecord[]>([])
	const [sorting, setSorting] = useState<SortingState>([])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
	const [rowSelection, setRowSelection] = useState({})
	const [globalFilter, setGlobalFilter] = useState("")
	const { toast } = useToast()
	const [deleteOpen, setDeleteDialogOpen] = useState(false)

	useEffect(() => {
		let unsubscribe: (() => void) | undefined
		const pbOptions = {
			expand: "system",
			fields: "id,name,value,state,created,resolved,expand.system.name",
		}
		// Initial load
		pb.collection<AlertsHistoryRecord>("alerts_history")
			.getList(0, 200, {
				...pbOptions,
				sort: "-created",
			})
			.then(({ items }) => setData(items))

		// Subscribe to changes
		;(async () => {
			unsubscribe = await pb.collection("alerts_history").subscribe(
				"*",
				(e) => {
					if (e.action === "create") {
						setData((current) => [e.record as AlertsHistoryRecord, ...current])
					}
					if (e.action === "update") {
						setData((current) => current.map((r) => (r.id === e.record.id ? (e.record as AlertsHistoryRecord) : r)))
					}
					if (e.action === "delete") {
						setData((current) => current.filter((r) => r.id !== e.record.id))
					}
				},
				pbOptions
			)
		})()
		// Unsubscribe on unmount
		return () => unsubscribe?.()
	}, [])

	const table = useReactTable({
		data,
		columns: [
			{
				id: "select",
				header: ({ table }) => (
					<Checkbox
						className="ms-2"
						checked={table.getIsAllPageRowsSelected() || (table.getIsSomePageRowsSelected() && "indeterminate")}
						onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
						aria-label="Select all"
					/>
				),
				cell: ({ row }) => (
					<Checkbox
						checked={row.getIsSelected()}
						onCheckedChange={(value) => row.toggleSelected(!!value)}
						aria-label="Select row"
					/>
				),
				enableSorting: false,
				enableHiding: false,
			},
			...alertsHistoryColumns,
		],
		getCoreRowModel: getCoreRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		onColumnVisibilityChange: setColumnVisibility,
		onRowSelectionChange: setRowSelection,
		state: {
			sorting,
			columnFilters,
			columnVisibility,
			rowSelection,
			globalFilter,
		},
		onGlobalFilterChange: setGlobalFilter,
		globalFilterFn: (row, _columnId, filterValue) => {
			const system = row.original.expand?.system?.name ?? ""
			const name = row.getValue("name") ?? ""
			const created = row.getValue("created") ?? ""
			const search = String(filterValue).toLowerCase()
			return (
				system.toLowerCase().includes(search) ||
				(name as string).toLowerCase().includes(search) ||
				(created as string).toLowerCase().includes(search)
			)
		},
	})

	// Bulk delete handler
	const handleBulkDelete = async () => {
		setDeleteDialogOpen(false)
		const selectedIds = table.getSelectedRowModel().rows.map((row) => row.original.id)
		try {
			let batch = pb.createBatch()
			let inBatch = 0
			for (const id of selectedIds) {
				batch.collection("alerts_history").delete(id)
				inBatch++
				if (inBatch > 20) {
					await batch.send()
					batch = pb.createBatch()
					inBatch = 0
				}
			}
			inBatch && (await batch.send())
			table.resetRowSelection()
		} catch (e) {
			toast({
				variant: "destructive",
				title: t`Error`,
				description: `Failed to delete records.`,
			})
		}
	}

	// Export to CSV handler
	const handleExportCSV = () => {
		const selectedRows = table.getSelectedRowModel().rows
		if (!selectedRows.length) return
		const cells: Record<string, (record: AlertsHistoryRecord) => string> = {
			system: (record) => record.expand?.system?.name || record.system,
			name: (record) => alertInfo[record.name]?.name() || record.name,
			value: (record) => record.value + (alertInfo[record.name]?.unit ?? ""),
			state: (record) => (record.resolved ? t`Resolved` : t`Active`),
			created: (record) => formatShortDate(record.created),
			resolved: (record) => (record.resolved ? formatShortDate(record.resolved) : ""),
			duration: (record) => (record.resolved ? formatDuration(record.created, record.resolved) : ""),
		}
		const csvRows = [Object.keys(cells).join(",")]
		for (const row of selectedRows) {
			const r = row.original
			csvRows.push(
				Object.values(cells)
					.map((val) => val(r))
					.join(",")
			)
		}
		const blob = new Blob([csvRows.join("\n")], { type: "text/csv" })
		const url = URL.createObjectURL(blob)
		const a = document.createElement("a")
		a.href = url
		a.download = "alerts_history.csv"
		a.click()
		URL.revokeObjectURL(url)
	}

	return (
		<div className="@container w-full">
			<div className="@3xl:flex items-end mb-4 gap-4">
				<SectionIntro />
				<div className="flex items-center gap-2 ms-auto mt-3 @3xl:mt-0">
					{table.getFilteredSelectedRowModel().rows.length > 0 && (
						<div className="fixed bottom-0 left-0 w-full p-4 grid grid-cols-2 items-center gap-4 z-50 backdrop-blur-md shrink-0 @lg:static @lg:p-0 @lg:w-auto @lg:gap-3">
							<AlertDialog open={deleteOpen} onOpenChange={(open) => setDeleteDialogOpen(open)}>
								<AlertDialogTrigger asChild>
									<Button variant="destructive" className="h-9 shrink-0">
										<Trash2Icon className="size-4 shrink-0" />
										<span className="ms-1">
											<Trans>Delete</Trans>
										</span>
									</Button>
								</AlertDialogTrigger>
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
							<Button variant="outline" className="h-10" onClick={handleExportCSV}>
								<DownloadIcon className="size-4" />
								<span className="ms-1">
									<Trans>Export</Trans>
								</span>
							</Button>
						</div>
					)}
					<Input
						placeholder={t`Filter...`}
						value={globalFilter}
						onChange={(e) => setGlobalFilter(e.target.value)}
						className="px-4 w-full max-w-full @3xl:w-64"
					/>
				</div>
			</div>
			<div className="rounded-md border overflow-x-auto whitespace-nowrap">
				<Table>
					<TableHeader>
						{table.getHeaderGroups().map((headerGroup) => (
							<TableRow key={headerGroup.id}>
								{headerGroup.headers.map((header) => (
									<TableHead className="px-2" key={header.id}>
										{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
									</TableHead>
								))}
							</TableRow>
						))}
					</TableHeader>
					<TableBody>
						{table.getRowModel().rows.length ? (
							table.getRowModel().rows.map((row) => (
								<TableRow key={row.id} data-state={row.getIsSelected() && "selected"}>
									{row.getVisibleCells().map((cell) => (
										<TableCell key={cell.id} className="py-3">
											{flexRender(cell.column.columnDef.cell, cell.getContext())}
										</TableCell>
									))}
								</TableRow>
							))
						) : (
							<TableRow>
								<TableCell colSpan={table.getAllColumns().length} className="h-24 text-center">
									<Trans>No results.</Trans>
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</Table>
			</div>
			<div className="flex items-center justify-between ps-1 tabular-nums">
				<div className="text-muted-foreground hidden flex-1 text-sm lg:flex">
					<Trans>
						{table.getFilteredSelectedRowModel().rows.length} of {table.getFilteredRowModel().rows.length} row(s)
						selected.
					</Trans>
				</div>
				<div className="flex w-full items-center gap-8 lg:w-fit my-3">
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
							<SelectTrigger className="w-[4.8em]" id="rows-per-page">
								<SelectValue placeholder={table.getState().pagination.pageSize} />
							</SelectTrigger>
							<SelectContent side="top">
								{[10, 20, 50, 100, 200].map((pageSize) => (
									<SelectItem key={pageSize} value={`${pageSize}`}>
										{pageSize}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
					<div className="flex w-fit items-center justify-center text-sm font-medium">
						<Trans>
							Page {table.getState().pagination.pageIndex + 1} of {table.getPageCount()}
						</Trans>
					</div>
					<div className="ms-auto flex items-center gap-2 lg:ms-0">
						<Button
							variant="outline"
							className="hidden size-9 p-0 lg:flex"
							onClick={() => table.setPageIndex(0)}
							disabled={!table.getCanPreviousPage()}
						>
							<span className="sr-only">Go to first page</span>
							<ChevronsLeftIcon className="size-5" />
						</Button>
						<Button
							variant="outline"
							className="size-9"
							size="icon"
							onClick={() => table.previousPage()}
							disabled={!table.getCanPreviousPage()}
						>
							<span className="sr-only">Go to previous page</span>
							<ChevronLeftIcon className="size-5" />
						</Button>
						<Button
							variant="outline"
							className="size-9"
							size="icon"
							onClick={() => table.nextPage()}
							disabled={!table.getCanNextPage()}
						>
							<span className="sr-only">Go to next page</span>
							<ChevronRightIcon className="size-5" />
						</Button>
						<Button
							variant="outline"
							className="hidden size-9 lg:flex"
							size="icon"
							onClick={() => table.setPageIndex(table.getPageCount() - 1)}
							disabled={!table.getCanNextPage()}
						>
							<span className="sr-only">Go to last page</span>
							<ChevronsRightIcon className="size-5" />
						</Button>
					</div>
				</div>
			</div>
		</div>
	)
}
