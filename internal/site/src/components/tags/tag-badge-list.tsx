import { TagIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { TagBadge } from "@/components/tags/tag-badge"
import { cn } from "@/lib/utils"
import type { TagRecord } from "@/types"

interface TagBadgeListProps {
	tags: TagRecord[]
	max: number
	/** "badge": show up to max, then a "+N" badge with a tooltip listing the rest.
	 *  "collapse": show all inline if <= max, otherwise only a count trigger with a tooltip listing all. */
	overflow?: "badge" | "collapse"
	badgeClassName?: string
}

/**
 * Renders only the tag badges/overflow-trigger content — no wrapping element —
 * so callers can compose it into their own container (a Link, a flex row, etc.)
 * without an extra intermediate DOM node.
 */
export function TagBadgeList({ tags, max, overflow = "badge", badgeClassName }: TagBadgeListProps) {
	if (tags.length === 0) return null

	if (overflow === "collapse") {
		if (tags.length <= max) {
			return (
				<>
					{tags.map((tag) => (
						<TagBadge key={tag.id} tag={tag} className={badgeClassName} />
					))}
				</>
			)
		}
		return (
			<Tooltip delayDuration={150}>
				<TooltipTrigger asChild>
					<div className="flex gap-1.5 items-center cursor-default">
						<TagIcon className="h-4 w-4" />
						<span>{tags.length}</span>
					</div>
				</TooltipTrigger>
				<TooltipContent>
					<div className="flex flex-wrap gap-1.5 max-w-64">
						{tags.map((tag) => (
							<TagBadge key={tag.id} tag={tag} />
						))}
					</div>
				</TooltipContent>
			</Tooltip>
		)
	}

	return (
		<>
			{tags.slice(0, max).map((tag) => (
				<TagBadge key={tag.id} tag={tag} className={badgeClassName} />
			))}
			{tags.length > max && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge variant="secondary" className={cn("text-xs px-1.5 py-0 cursor-default", badgeClassName)}>
							+{tags.length - max}
						</Badge>
					</TooltipTrigger>
					<TooltipContent>
						<div className="flex flex-wrap gap-1">
							{tags.slice(max).map((tag) => (
								<TagBadge key={tag.id} tag={tag} />
							))}
						</div>
					</TooltipContent>
				</Tooltip>
			)}
		</>
	)
}
