import { Trans } from "@lingui/react/macro"
import type { ColumnDef } from "@tanstack/react-table"
import {
	MoreHorizontalIcon,
	PencilIcon,
	ServerIcon,
	TagIcon,
	TrashIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { TagBadge } from "@/components/tags/tag-badge"
import type { SystemRecord, TagRecord } from "@/types"

export interface TagWithSystems extends TagRecord {
	systems: SystemRecord[]
}

// Create columns generator function that accepts callbacks
export function createTagsColumns(
	onEditTag: (tag: TagWithSystems) => void,
	onDeleteTag: (id: string, name: string) => void
): ColumnDef<TagWithSystems>[] {
	return [
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
  					onClick={(e) => e.stopPropagation()}
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
			cell: ({ row }) => <TagBadge tag={row.original} />,
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
						onClick={() => onEditTag(row.original)}
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
						<DropdownMenuItem onSelect={() => onEditTag(row.original)}>
							<PencilIcon className="me-2.5 size-4" />
							<Trans>Edit tag</Trans>
						</DropdownMenuItem>
						<DropdownMenuSeparator />
						<DropdownMenuItem
							className="text-destructive focus:text-destructive"
							onSelect={() => onDeleteTag(row.original.id, row.original.name)}
							onClick={(e) => e.stopPropagation()}
						>
							<TrashIcon className="me-2.5 size-4" />
							<Trans>Delete tag</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			),
		},
	]
}
