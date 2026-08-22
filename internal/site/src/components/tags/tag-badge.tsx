import { Badge, type BadgeProps } from "@/components/ui/badge"
import { getTagColorClasses } from "@/lib/tag-utils"
import { cn } from "@/lib/utils"
import type { TagRecord } from "@/types"

interface TagBadgeProps extends Omit<BadgeProps, "children"> {
	tag: Pick<TagRecord, "name" | "color">
	/** Allow pointer events (e.g. for previews outside a clickable row). Defaults to false. */
	interactive?: boolean
}

export function TagBadge({ tag, interactive, className, ...props }: TagBadgeProps) {
	return (
		<Badge
			className={cn("text-xs", !interactive && "pointer-events-none", getTagColorClasses(tag.color), className)}
			{...props}
		>
			{tag.name}
		</Badge>
	)
}
