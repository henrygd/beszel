import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { TagIcon } from "lucide-react"
import { type Dispatch, type SetStateAction, useEffect, useState } from "react"
import { TagBadge } from "@/components/tags/tag-badge"
import { DropdownMenuCheckboxItem, DropdownMenuLabel, DropdownMenuSeparator } from "@/components/ui/dropdown-menu"
import { $tags } from "@/lib/stores"

/** Selected tag ids for a tag filter; ids of deleted tags are dropped automatically */
export function useTagFilter(): [string[], Dispatch<SetStateAction<string[]>>] {
	const availableTags = useStore($tags)
	const [selected, setSelected] = useState<string[]>([])

	useEffect(() => {
		setSelected((prev) => {
			const next = prev.filter((id) => availableTags.some((t) => t.id === id))
			return next.length === prev.length ? prev : next
		})
	}, [availableTags])

	return [selected, setSelected]
}

interface TagFilterDropdownProps {
	selected: string[]
	onChange: Dispatch<SetStateAction<string[]>>
	/** Number of records per tag id, shown next to each tag */
	counts: Record<string, number>
}

/** Tag filter section for a table's view-options dropdown menu */
export function TagFilterDropdown({ selected, onChange, counts }: TagFilterDropdownProps) {
	const availableTags = useStore($tags)

	return (
		<>
			<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
				<TagIcon className="size-4" />
				<Trans>Tags</Trans>
			</DropdownMenuLabel>
			<DropdownMenuSeparator />
			<div className="px-1 pb-1 max-h-64 overflow-y-auto min-w-48">
				{availableTags.length === 0 ? (
					<p className="text-xs text-muted-foreground py-2 px-2">
						<Trans>No tags available</Trans>
					</p>
				) : (
					<div className="space-y-0.5">
						{availableTags.map((tag) => (
							<DropdownMenuCheckboxItem
								key={tag.id}
								checked={selected.includes(tag.id)}
								onCheckedChange={(checked) =>
									onChange((prev) => (checked ? [...prev, tag.id] : prev.filter((id) => id !== tag.id)))
								}
								// keep the menu open to allow selecting several tags
								onSelect={(e) => e.preventDefault()}
								className="justify-between gap-3"
							>
								<TagBadge tag={tag} className="px-2 py-0.5 shrink-0" />
								<span className="text-xs text-muted-foreground shrink-0">{counts[tag.id] ?? 0}</span>
							</DropdownMenuCheckboxItem>
						))}
					</div>
				)}
			</div>
		</>
	)
}
