import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import {
	type ColumnDef,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type RowSelectionState,
	type SortingState,
	useReactTable,
} from "@tanstack/react-table"
import { ChevronDownIcon, MoreHorizontalIcon, PencilIcon, PlusIcon, ServerIcon, TagIcon, Trash2Icon, TrashIcon, XIcon } from "lucide-react"
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
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"
import { cn } from "@/lib/utils"
import { pb } from "@/lib/api"
import type { SystemRecord, TagRecord } from "@/types"

// Tag color names (Tailwind colors)
const tagColors = ["red", "orange", "amber", "yellow", "lime", "green", "emerald", "teal", "cyan", "sky", "blue", "indigo", "violet", "purple", "fuchsia", "pink", "rose"]

// Tag color classes mapping
export const tagColorClasses: Record<string, string> = {
	red: "bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300",
	orange: "bg-orange-100 text-orange-700 dark:bg-orange-950 dark:text-orange-300",
	amber: "bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300",
	yellow: "bg-yellow-100 text-yellow-700 dark:bg-yellow-950 dark:text-yellow-300",
	lime: "bg-lime-100 text-lime-700 dark:bg-lime-950 dark:text-lime-300",
	green: "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300",
	emerald: "bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300",
	teal: "bg-teal-100 text-teal-700 dark:bg-teal-950 dark:text-teal-300",
	cyan: "bg-cyan-100 text-cyan-700 dark:bg-cyan-950 dark:text-cyan-300",
	sky: "bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-300",
	blue: "bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300",
	indigo: "bg-indigo-100 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300",
	violet: "bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300",
	purple: "bg-purple-100 text-purple-700 dark:bg-purple-950 dark:text-purple-300",
	fuchsia: "bg-fuchsia-100 text-fuchsia-700 dark:bg-fuchsia-950 dark:text-fuchsia-300",
	pink: "bg-pink-100 text-pink-700 dark:bg-pink-950 dark:text-pink-300",
	rose: "bg-rose-100 text-rose-700 dark:bg-rose-950 dark:text-rose-300",
}

// Generate a random color name
export function getRandomColor(): string {
	return tagColors[Math.floor(Math.random() * tagColors.length)]
}

// Get classes for a tag color
export function getTagColorClasses(color?: string): string {
	return tagColorClasses[color || "blue"] || tagColorClasses.blue
}

interface TagWithSystems extends TagRecord {
	systems: SystemRecord[]
}

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

	const columns = useMemo<ColumnDef<TagWithSystems>[]>(
		() => [
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
						className="ms-2"
						checked={row.getIsSelected()}
						onCheckedChange={(value) => row.toggleSelected(!!value)}
						aria-label="Select row"
					/>
				),
				enableSorting: false,
				enableHiding: false,
			},
			{
				accessorKey: "name",
				header: () => (
					<span className="flex items-center gap-2">
						<TagIcon className="size-4" />
						<Trans>Tag</Trans>
					</span>
				),
				cell: ({ row }) => (
					<Badge className={getTagColorClasses(row.original.color)}>
						{row.original.name}
					</Badge>
				),
			},
			{
				id: "systems",
				accessorFn: (row) => row.systems.length,
				header: () => (
					<span className="flex items-center gap-2">
						<ServerIcon className="size-4" />
						<Trans>Systems</Trans>
					</span>
				),
				cell: ({ row }) => {
					const systemsList = row.original.systems
					return (
						<Button
							variant="ghost"
							size="sm"
							className="h-8 px-3 gap-2"
							onClick={() => openEditDialog(row.original)}
						>
							<span>{systemsList.length}</span>
							{systemsList.length > 0 && (
								<span className="text-muted-foreground text-xs truncate max-w-40 hidden sm:inline">
									({systemsList.map((s) => s.name).join(", ")})
								</span>
							)}
						</Button>
					)
				},
			},
			{
				id: "actions",
				header: () => (
					<span className="sr-only">
						<Trans>Actions</Trans>
					</span>
				),
				cell: ({ row }) => (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button variant="ghost" size="icon">
								<span className="sr-only">
									<Trans>Open menu</Trans>
								</span>
								<MoreHorizontalIcon className="size-5" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem onSelect={() => openEditDialog(row.original)}>
								<PencilIcon className="me-2.5 size-4" />
								<Trans>Edit tag</Trans>
							</DropdownMenuItem>
							<DropdownMenuSeparator />
							<DropdownMenuItem
								className="text-destructive focus:text-destructive"
								onSelect={() => deleteTag(row.original.id, row.original.name)}
							>
								<TrashIcon className="me-2.5 size-4" />
								<Trans>Delete tag</Trans>
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				),
			},
		],
		[systems]
	)

	const table = useReactTable({
		data: tagsWithSystems,
		columns,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onGlobalFilterChange: setGlobalFilter,
		onRowSelectionChange: setRowSelection,
		state: {
			sorting,
			globalFilter,
			rowSelection,
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
			<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
				<DialogContent className="sm:max-w-lg">
					<DialogHeader>
						<DialogTitle>
							{editingTag ? <Trans>Edit Tag</Trans> : <Trans>Create New Tag</Trans>}
						</DialogTitle>
						<DialogDescription>
							{editingTag ? (
								<Trans>Update the tag name, color, and assigned systems.</Trans>
							) : (
								<Trans>Create a new tag and assign it to systems.</Trans>
							)}
						</DialogDescription>
					</DialogHeader>
					<div className="grid xs:grid-cols-[auto_1fr] gap-y-3 gap-x-4 items-center mt-1">
						<Label htmlFor="tag-name" className="xs:text-end">
							<Trans>Name</Trans>
						</Label>
						<Input
							id="tag-name"
							value={newTagName}
							onChange={(e) => setNewTagName(e.target.value)}
							placeholder={t`e.g., Production, Development`}
							maxLength={50}
						/>
						<Label className="xs:text-end self-start pt-2">
							<Trans>Color</Trans>
						</Label>
						<div className="flex flex-col gap-2">
							<div className="flex flex-wrap gap-1.5">
								{tagColors.map((color) => (
									<button
										key={color}
										type="button"
										onClick={() => setNewTagColor(color)}
										className={cn(
											"w-6 h-6 rounded-full transition-all",
											tagColorClasses[color].split(" ").filter(c => c.startsWith("bg-") && !c.includes("dark")).join(" "),
											newTagColor === color && "ring-2 ring-offset-2 ring-primary"
										)}
									/>
								))}
							</div>
							<Badge className={cn(getTagColorClasses(newTagColor), "self-start")}>
								{newTagName || t`Preview`}
							</Badge>
						</div>
						<Label className="xs:text-end self-start pt-2">
							<Trans>Systems</Trans>
						</Label>
						<div className="flex flex-col gap-2">
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" className="justify-between font-normal">
										{selectedSystems.length > 0 ? (
											<span className="truncate">
												{selectedSystems.length === 1
													? systems.find((s) => s.id === selectedSystems[0])?.name
													: t`${selectedSystems.length} systems selected`}
											</span>
										) : (
											<span className="text-muted-foreground">
												<Trans>Select systems...</Trans>
											</span>
										)}
										<ChevronDownIcon className="ml-2 h-4 w-4 shrink-0 opacity-50" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent className="w-80" align="start">
									<div className="px-2 py-1.5">
										<Input
											placeholder={t`Search systems...`}
											value={systemSearchQuery}
											onChange={(e) => setSystemSearchQuery(e.target.value)}
											className="h-8"
										/>
									</div>
									<DropdownMenuSeparator />
									<div className="max-h-60 overflow-y-auto">
										{systems
											.filter((system) =>
												system.name.toLowerCase().includes(systemSearchQuery.toLowerCase())
											)
											.map((system) => {
												const isSelected = selectedSystems.includes(system.id)
												return (
													<DropdownMenuCheckboxItem
														key={system.id}
														checked={isSelected}
														onCheckedChange={(checked) => {
															setSelectedSystems((prev) =>
																checked ? [...prev, system.id] : prev.filter((id) => id !== system.id)
															)
														}}
														onSelect={(e) => e.preventDefault()}
													>
														{system.name}
													</DropdownMenuCheckboxItem>
												)
											})}
										{systems.length === 0 && (
											<div className="py-4 text-center text-sm text-muted-foreground">
												<Trans>No systems found.</Trans>
											</div>
										)}
										{systems.length > 0 &&
											systemSearchQuery &&
											systems.filter((s) =>
												s.name.toLowerCase().includes(systemSearchQuery.toLowerCase())
											).length === 0 && (
												<div className="py-4 text-center text-sm text-muted-foreground">
													<Trans>No systems found.</Trans>
												</div>
											)}
									</div>
								</DropdownMenuContent>
							</DropdownMenu>
							{selectedSystems.length > 0 && (
								<div className="flex flex-wrap gap-1.5">
									{selectedSystems.map((systemId) => {
										const system = systems.find((s) => s.id === systemId)
										if (!system) return null
										return (
											<Badge
												key={system.id}
												variant="secondary"
												className="text-xs"
											>
												{system.name}
												<button
													type="button"
													className="ml-1 hover:bg-muted-foreground/20 rounded-full"
													onClick={() => {
														setSelectedSystems((prev) => prev.filter((id) => id !== systemId))
													}}
												>
													<XIcon className="h-3 w-3" />
												</button>
											</Badge>
										)
									})}
								</div>
							)}
						</div>
					</div>
					<DialogFooter className="mt-4">
						<Button variant="outline" onClick={() => setDialogOpen(false)}>
							<Trans>Cancel</Trans>
						</Button>
						<Button onClick={saveTag}>
							{editingTag ? <Trans>Save</Trans> : <Trans>Create</Trans>}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

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
								<TableRow key={row.id}>
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
		</div>
	)
}
