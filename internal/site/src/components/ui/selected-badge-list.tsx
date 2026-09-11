import { XIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

interface SelectedBadgeListProps<T> {
	items: T[]
	selectedIds: string[]
	getItemLabel: (item: T) => string
	getItemId: (item: T) => string
	getItemClasses?: (item: T) => string
	onRemove: (id: string) => void
	renderBadge?: (item: T, label: string, onRemove: () => void) => React.ReactNode
	variant?: "default" | "secondary"
}

export function SelectedBadgeList<T>({
	items,
	selectedIds,
	getItemLabel,
	getItemId,
	getItemClasses,
	onRemove,
	renderBadge,
	variant = "default",
}: SelectedBadgeListProps<T>) {
	if (selectedIds.length === 0) return null

	return (
		<div className="flex flex-wrap gap-1.5">
			{selectedIds.map((id) => {
				const item = items.find((i) => getItemId(i) === id)
				if (!item) return null
				const label = getItemLabel(item)

				if (renderBadge) {
					return (
						<div key={id}>
							{renderBadge(item, label, () => onRemove(id))}
						</div>
					)
				}

				const customClasses = getItemClasses?.(item)

				return (
					<Badge
						key={id}
						variant={variant}
						className={cn("text-xs pointer-events-none", customClasses)}
					>
						{label}
						<button
							type="button"
							className="ml-1 hover:bg-black/10 dark:hover:bg-white/20 rounded-full pointer-events-auto"
							onClick={(e) => {
								e.stopPropagation()
								onRemove(id)
							}}
						>
							<XIcon className="h-3 w-3" />
						</button>
					</Badge>
				)
			})}
		</div>
	)
}
