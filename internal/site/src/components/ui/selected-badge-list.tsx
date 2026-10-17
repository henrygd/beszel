import { XIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

interface SelectedBadgeListProps<T> {
	/** The selected items, in display order */
	items: T[]
	getItemLabel: (item: T) => string
	getItemId: (item: T) => string
	getItemClasses?: (item: T) => string
	onRemove: (id: string) => void
	variant?: "default" | "secondary"
}

export function SelectedBadgeList<T>({
	items,
	getItemLabel,
	getItemId,
	getItemClasses,
	onRemove,
	variant = "default",
}: SelectedBadgeListProps<T>) {
	if (items.length === 0) return null

	return (
		<div className="flex flex-wrap gap-1.5">
			{items.map((item) => {
				const id = getItemId(item)
				return (
					<Badge key={id} variant={variant} className={cn("text-xs pointer-events-none", getItemClasses?.(item))}>
						{getItemLabel(item)}
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
