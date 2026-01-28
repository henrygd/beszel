import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
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
import { useEffect, useMemo, useState } from "react"
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
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"
import { cn, useBrowserStorage } from "@/lib/utils"
import { pb } from "@/lib/api"
import type { SystemRecord, TagRecord } from "@/types"
import { createTagsColumns, getRandomColor, type TagWithSystems } from "@/components/tags-columns"
import { TagEditDialog } from "@/components/tag-edit-dialog"

export default function TagsSettings() {
	const { t: tFunc } = useLingui()
	const [tags, setTags] = useState<TagRecord[]>([])
	const [systems, setSystems] = useState<SystemRecord[]>([])
	const [loading, setLoading] = useState(true)
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editingTag, setEditingTag] = useState<TagWithSystems | null>(null)
	const [selectedSystems, setSelectedSystems] = useState<string[]>([])
	const [newTagName, setNewTagName] = useState("")
	const [newTagColor, setNewTagColor] = useState("#3b82f6")
	const [systemSearchQuery, setSystemSearchQuery] = useState("")
	const [globalFilter, setGlobalFilter] = useState("")
	const [sorting, setSorting] = useState<SortingState>([])
	const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
	const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
	const [pagination, setPagination] = useBrowserStorage<PaginationState>("tags-pagination", {
		pageIndex: 0,
		pageSize: 10,
	})

	// Initial data load
	useEffect(() => {
		Promise.all([
			pb.collection("tags").getFullList<TagRecord>({ sort: "name", requestKey: null }),
			pb.collection("systems").getFullList<SystemRecord>({ sort: "name", fields: "id,name,tags", requestKey: null }),
		]).then(([tagsRecords, systemsRecords]) => {
			setTags(tagsRecords)
			setSystems(systemsRecords)
			setLoading(false)
		})
	}, [])

	// Subscribe to tag updates
	useEffect(() => {
		let unsubscribe: (() => void) | undefined
		;(async () => {
			unsubscribe = await pb.collection("tags").subscribe(
				"*",
				(e) => {
					setTags((current) => {
						if (e.action === "create") {
							return [...current, e.record as TagRecord].sort((a, b) => a.name.localeCompare(b.name))
						}
						if (e.action === "update") {
							return current.map((tag) =>
								tag.id === e.record.id ? (e.record as TagRecord) : tag
							)
						}
						if (e.action === "delete") {
							return current.filter((tag) => tag.id !== e.record.id)
						}
						return current
					})
				}
			)
		})()
		return () => unsubscribe?.()
	}, [])

	// Combine tags with their systems
	const tagsWithSystems = useMemo((): TagWithSystems[] => {
		return tags.map((tag) => ({
			...tag,
			systems: systems.filter((s) => s.tags?.includes(tag.id)),
		}))
	}, [tags, systems])

	function openCreateDialog() {
		setEditingTag(null)
		setNewTagName("")
		setNewTagColor(getRandomColor())
		setSelectedSystems([])
		setSystemSearchQuery("")
		setDialogOpen(true)
	}

	function openEditDialog(tag: TagWithSystems) {
		setEditingTag(tag)
		setNewTagName(tag.name)
		setNewTagColor(tag.color || "#3b82f6")
		setSelectedSystems(tag.systems.map((s) => s.id))
		setSystemSearchQuery("")
		setDialogOpen(true)
	}

	async function saveTag() {
		if (!newTagName.trim()) {
			toast({
				title: tFunc`Tag name required`,
				description: tFunc`Please enter a tag name.`,
				variant: "destructive",
			})
			return
		}

		try {
			let tagId: string
			if (editingTag) {
				// Update tag
				const record = await pb.collection("tags").update<TagRecord>(editingTag.id, {
					name: newTagName.trim(),
					color: newTagColor,
				})
				setTags(tags.map((t) => (t.id === record.id ? record : t)).sort((a, b) => a.name.localeCompare(b.name)))
				tagId = editingTag.id

				// Update system assignments
				const currentSystems = systems.filter((s) => s.tags?.includes(tagId)).map((s) => s.id)
				const toAdd = selectedSystems.filter((id) => !currentSystems.includes(id))
				const toRemove = currentSystems.filter((id) => !selectedSystems.includes(id))

				const updates = [
					...toAdd.map((systemId) => {
						const system = systems.find((s) => s.id === systemId)
						const newTags = [...(system?.tags || []), tagId]
						return pb.collection("systems").update(systemId, { tags: newTags })
					}),
					...toRemove.map((systemId) => {
						const system = systems.find((s) => s.id === systemId)
						const newTags = (system?.tags || []).filter((t) => t !== tagId)
						return pb.collection("systems").update(systemId, { tags: newTags })
					}),
				]

				if (updates.length > 0) {
					await Promise.all(updates)
					setSystems((prev) =>
						prev.map((s) => {
							if (toAdd.includes(s.id)) {
								return { ...s, tags: [...(s.tags || []), tagId] }
							}
							if (toRemove.includes(s.id)) {
								return { ...s, tags: (s.tags || []).filter((t) => t !== tagId) }
							}
							return s
						})
					)
				}

				toast({
					title: tFunc`Tag updated`,
					description: tFunc`The tag has been updated successfully.`,
				})
			} else {
				// Create new tag
				const record = await pb.collection("tags").create<TagRecord>({
					name: newTagName.trim(),
					color: newTagColor,
				})
				setTags([...tags, record].sort((a, b) => a.name.localeCompare(b.name)))
				tagId = record.id

				// Assign to selected systems
				if (selectedSystems.length > 0) {
					const updates = selectedSystems.map((systemId) => {
						const system = systems.find((s) => s.id === systemId)
						const newTags = [...(system?.tags || []), tagId]
						return pb.collection("systems").update(systemId, { tags: newTags })
					})
					await Promise.all(updates)
					setSystems((prev) =>
						prev.map((s) => {
							if (selectedSystems.includes(s.id)) {
								return { ...s, tags: [...(s.tags || []), tagId] }
							}
							return s
						})
					)
				}

				toast({
					title: tFunc`Tag created`,
					description: tFunc`The tag has been created successfully.`,
				})
			}

			setDialogOpen(false)
		} catch (e: any) {
			console.error("Failed to save tag", e)
			toast({
				title: editingTag ? tFunc`Failed to update tag` : tFunc`Failed to create tag`,
				description: e.message || tFunc`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	async function deleteTag(id: string, name: string) {
		if (!confirm(tFunc`Are you sure you want to delete the tag "${name}"? This will remove it from all systems.`)) {
			return
		}

		try {
			await pb.collection("tags").delete(id)
			setTags(tags.filter((tag) => tag.id !== id))
			toast({
				title: tFunc`Tag deleted`,
				description: tFunc`The tag has been deleted successfully.`,
			})
		} catch (e) {
			console.error("Failed to delete tag", e)
			toast({
				title: tFunc`Failed to delete tag`,
				description: tFunc`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	async function handleBulkDelete() {
		setDeleteDialogOpen(false)
		const selectedIds = table.getSelectedRowModel().rows.map((row) => row.original.id)
		try {
			let batch = pb.createBatch()
			let inBatch = 0
			for (const id of selectedIds) {
				batch.collection("tags").delete(id)
				inBatch++
				if (inBatch > 20) {
					await batch.send()
					batch = pb.createBatch()
					inBatch = 0
				}
			}
			inBatch && (await batch.send())
			setTags((prev) => prev.filter((tag) => !selectedIds.includes(tag.id)))
			table.resetRowSelection()
			toast({
				title: tFunc`Tags deleted`,
				description: tFunc`${selectedIds.length} tag(s) deleted successfully.`,
			})
		} catch (e) {
			console.error("Failed to delete tags", e)
			toast({
				title: tFunc`Error`,
				description: tFunc`Failed to delete tags.`,
				variant: "destructive",
			})
		}
	}

	const columns = createTagsColumns(openEditDialog, deleteTag)

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

	if (loading) {
		return (
			<div className="py-8 text-center text-muted-foreground">
				<Trans>Loading...</Trans>
			</div>
		)
	}

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
						<AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
							<AlertDialogTrigger asChild>
								<Button variant="destructive" className="h-9 shrink-0">
									<Trash2Icon className="size-4 shrink-0" />
									<span className="ms-1">
										<Trans>Delete ({table.getFilteredSelectedRowModel().rows.length})</Trans>
									</span>
								</Button>
							</AlertDialogTrigger>
							<AlertDialogContent>
								<AlertDialogHeader>
									<AlertDialogTitle>
										<Trans>Are you sure?</Trans>
									</AlertDialogTitle>
									<AlertDialogDescription>
										<Trans>
											This will permanently delete {table.getFilteredSelectedRowModel().rows.length} tag(s) and remove them from all systems.
										</Trans>
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
										<Trans>Delete</Trans>
									</AlertDialogAction>
								</AlertDialogFooter>
							</AlertDialogContent>
						</AlertDialog>
					)}
					<Input
						placeholder={t`Filter tags...`}
						value={globalFilter}
						onChange={(e) => setGlobalFilter(e.target.value)}
						className="w-full sm:w-48"
					/>
					<Button onClick={openCreateDialog} className="shrink-0">
						<PlusIcon className="size-4 sm:me-2" />
						<span className="hidden sm:inline">
							<Trans>Create Tag</Trans>
						</span>
					</Button>
				</div>
			</div>

			{/* Create/Edit Tag Dialog */}
			<TagEditDialog
				open={dialogOpen}
				onOpenChange={setDialogOpen}
				editingTag={editingTag}
				tagName={newTagName}
				tagColor={newTagColor}
				selectedSystems={selectedSystems}
				systemSearchQuery={systemSearchQuery}
				systems={systems}
				onTagNameChange={setNewTagName}
				onTagColorChange={setNewTagColor}
				onSelectedSystemsChange={setSelectedSystems}
				onSystemSearchQueryChange={setSystemSearchQuery}
				onSave={saveTag}
			/>

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
