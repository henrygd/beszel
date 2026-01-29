import { Trans } from "@lingui/react/macro"
import type { ColumnDef } from "@tanstack/react-table"
import {
	MoreHorizontalIcon,
	PencilIcon,
	ServerIcon,
	TagIcon,
	TrashIcon,
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"
import type { SystemRecord, TagRecord } from "@/types"

export interface TagWithSystems extends TagRecord {
	systems: SystemRecord[]
}

// Tag color names (Tailwind colors)
export const tagColors = ["red", "orange", "amber", "yellow", "lime", "green", "emerald", "teal", "cyan", "sky", "blue", "indigo", "violet", "purple", "fuchsia", "pink", "rose"]

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
			cell: ({ row }) => (
				<Badge className={cn("pointer-events-none", getTagColorClasses(row.original.color))}>
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
