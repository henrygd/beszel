import { plural, t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getPaginationRowModel,
	getSortedRowModel,
	type PaginationState,
	type RowSelectionState,
	type SortingState,
	useReactTable,
} from "@tanstack/react-table"
import {
	ChevronLeftIcon,
	ChevronRightIcon,
	ChevronsLeftIcon,
	ChevronsRightIcon,
	PlusIcon,
	TagIcon,
	Trash2Icon,
} from "lucide-react"
import { useStore } from "@nanostores/react"
import { useMemo, useState } from "react"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"
import { cn, useBrowserStorage } from "@/lib/utils"
import { sendInBatches } from "@/lib/api"
import { $systems, $tags } from "@/lib/stores"
import { createTagsColumns, type TagWithSystems } from "@/components/tags/tags-columns"
import { TagEditDialog } from "@/components/tags/tag-edit-dialog"
import { groupRecordsByTag } from "@/lib/tag-utils"

export default function TagsSettings() {
	const tags = useStore($tags)
	const systems = useStore($systems)
	const [editDialog, setEditDialog] = useState<{ open: boolean; tag: TagWithSystems | null }>({
		open: false,
		tag: null,
	})
	const [globalFilter, setGlobalFilter] = useState("")
	const [sorting, setSorting] = useState<SortingState>([])
	const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
	const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
	// tag to delete from its row menu; null deletes the selected rows
	const [tagToDelete, setTagToDelete] = useState<TagWithSystems | null>(null)
	const [pagination, setPagination] = useBrowserStorage<PaginationState>("tags-pagination", {
		pageIndex: 0,
		pageSize: 10,
	})

	// Combine tags with their systems, sorted by name (both come from shared, realtime-updated stores)
	const tagsWithSystems = useMemo((): TagWithSystems[] => {
		const systemsByTag = groupRecordsByTag([...systems].sort((a, b) => a.name.localeCompare(b.name)))
		return tags.map((tag) => ({ ...tag, systems: systemsByTag[tag.id] ?? [] }))
	}, [tags, systems])

	const openEditDialog = (tag: TagWithSystems | null) => setEditDialog({ open: true, tag })

	async function handleDelete() {
		setDeleteDialogOpen(false)
		const ids = tagToDelete ? [tagToDelete.id] : table.getSelectedRowModel().rows.map((row) => row.original.id)
		try {
			await sendInBatches(ids, (batch, id) => batch.collection("tags").delete(id))
			if (!tagToDelete) {
				table.resetRowSelection()
			}
			toast({
				title: plural(ids.length, { one: "Tag deleted", other: "Tags deleted" }),
				description: plural(ids.length, {
					one: "# tag deleted successfully.",
					other: "# tags deleted successfully.",
				}),
			})
		} catch (e) {
			console.error("Failed to delete tag(s)", e)
			toast({
				title: t`Error`,
				description: t`Failed to delete tag(s).`,
				variant: "destructive",
			})
		}
		setTagToDelete(null)
	}

	// callbacks only use state setters, so the columns never need rebuilding
	const columns = useMemo(
		() =>
			createTagsColumns(openEditDialog, (tag) => {
				setTagToDelete(tag)
				setDeleteDialogOpen(true)
			}),
		[]
	)

	const table = useReactTable({
		data: tagsWithSystems,
		columns,
		getCoreRowModel: getCoreRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onGlobalFilterChange: setGlobalFilter,
		onRowSelectionChange: setRowSelection,
		onPaginationChange: setPagination,
		state: {
			sorting,
			globalFilter,
			rowSelection,
			pagination,
		},
		globalFilterFn: (row, _columnId, filterValue) => {
			const tagName = row.original.name.toLowerCase()
			const systemNames = row.original.systems.map((s) => s.name.toLowerCase()).join(" ")
			const search = String(filterValue).toLowerCase()
			return tagName.includes(search) || systemNames.includes(search)
		},
	})

	return (
		<div className="space-y-4">
			<div className="flex flex-col sm:flex-row sm:items-end gap-4">
				<div className="flex-1">
					<h3 className="text-xl font-medium mb-2">
						<Trans>Tags</Trans>
					</h3>
					<p className="text-sm text-muted-foreground">
						<Trans>Create and manage tags to organize your systems.</Trans>
					</p>
				</div>
				<div className="flex items-center gap-2">
					{table.getFilteredSelectedRowModel().rows.length > 0 && (
						<Button
							variant="destructive"
							className="h-9 shrink-0"
							onClick={() => setDeleteDialogOpen(true)}
						>
							<Trash2Icon className="size-4 shrink-0" />
							<span className="ms-1">
								<Trans>Delete ({table.getFilteredSelectedRowModel().rows.length})</Trans>
							</span>
						</Button>
					)}
					<Input
						placeholder={t`Filter tags...`}
						value={globalFilter}
						onChange={(e) => setGlobalFilter(e.target.value)}
						className="w-full sm:w-48"
					/>
					<Button onClick={() => openEditDialog(null)} className="shrink-0">
						<PlusIcon className="size-4 sm:me-2" />
						<span className="hidden sm:inline">
							<Trans>Create Tag</Trans>
						</span>
					</Button>
				</div>
			</div>

			{/* Create/Edit Tag Dialog */}
			<TagEditDialog
				open={editDialog.open}
				onOpenChange={(open) => setEditDialog((prev) => ({ ...prev, open }))}
				tag={editDialog.tag}
			/>

			{/* Delete Confirmation Dialog */}
			<AlertDialog open={deleteDialogOpen} onOpenChange={(open) => {
				setDeleteDialogOpen(open)
				if (!open) setTagToDelete(null)
			}}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Are you sure?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							{tagToDelete ? (
								<Trans>
									This will permanently delete the tag "{tagToDelete.name}" and remove it from all systems.
								</Trans>
							) : (
								<Trans>
									This will permanently delete {table.getFilteredSelectedRowModel().rows.length} tag(s) and remove them from all systems.
								</Trans>
							)}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn(buttonVariants({ variant: "destructive" }))}
							onClick={handleDelete}
						>
							<Trans>Delete</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			{/* Data Table */}
			<div className="rounded-md border">
				<Table>
					<TableHeader>
						{table.getHeaderGroups().map((headerGroup) => (
							<TableRow key={headerGroup.id}>
								{headerGroup.headers.map((header) => (
									<TableHead key={header.id} className="px-2">
										{header.isPlaceholder
											? null
											: flexRender(header.column.columnDef.header, header.getContext())}
									</TableHead>
								))}
							</TableRow>
						))}
					</TableHeader>
					<TableBody>
						{table.getRowModel().rows.length ? (
							table.getRowModel().rows.map((row) => (
								<TableRow  
									key={row.id}
									className="cursor-pointer hover:bg-muted/50"
									onClick={() => openEditDialog(row.original)}
								>
									{row.getVisibleCells().map((cell) => (
										<TableCell key={cell.id} className="py-2 px-3">
											{flexRender(cell.column.columnDef.cell, cell.getContext())}
										</TableCell>
									))}
								</TableRow>
							))
						) : (
							<TableRow>
								<TableCell colSpan={columns.length} className="h-24 text-center">
									{tags.length === 0 ? (
										<div className="flex flex-col items-center gap-2 text-muted-foreground">
											<TagIcon className="size-8 opacity-50" />
											<p>
												<Trans>No tags created yet.</Trans>
											</p>
										</div>
									) : (
										<Trans>No results.</Trans>
									)}
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</Table>
			</div>

			{/* Pagination */}
			<div className="flex items-center justify-between ps-1 tabular-nums">
				<div className="text-muted-foreground hidden flex-1 text-sm lg:flex">
					<Trans>
						{table.getFilteredSelectedRowModel().rows.length} of {table.getFilteredRowModel().rows.length} row(s)
						selected.
					</Trans>
				</div>
				<div className="flex w-full items-center gap-8 lg:w-fit">
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
							<SelectTrigger className="w-18" id="rows-per-page">
								<SelectValue placeholder={table.getState().pagination.pageSize} />
							</SelectTrigger>
							<SelectContent side="top">
								{[10, 20, 50, 100].map((pageSize) => (
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
							<span className="sr-only">
								<Trans>Go to first page</Trans>
							</span>
							<ChevronsLeftIcon className="size-5" />
						</Button>
						<Button
							variant="outline"
							className="size-9"
							size="icon"
							onClick={() => table.previousPage()}
							disabled={!table.getCanPreviousPage()}
						>
							<span className="sr-only">
								<Trans>Go to previous page</Trans>
							</span>
							<ChevronLeftIcon className="size-5" />
						</Button>
						<Button
							variant="outline"
							className="size-9"
							size="icon"
							onClick={() => table.nextPage()}
							disabled={!table.getCanNextPage()}
						>
							<span className="sr-only">
								<Trans>Go to next page</Trans>
							</span>
							<ChevronRightIcon className="size-5" />
						</Button>
						<Button
							variant="outline"
							className="hidden size-9 lg:flex"
							size="icon"
							onClick={() => table.setPageIndex(table.getPageCount() - 1)}
							disabled={!table.getCanNextPage()}
						>
							<span className="sr-only">
								<Trans>Go to last page</Trans>
							</span>
							<ChevronsRightIcon className="size-5" />
						</Button>
					</div>
				</div>
			</div>
		</div>
	)
}
