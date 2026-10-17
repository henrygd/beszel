import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { toast } from "@/components/ui/use-toast"
import { SearchableDropdown } from "@/components/ui/searchable-dropdown"
import { SelectedBadgeList } from "@/components/ui/selected-badge-list"
import { TagBadge } from "@/components/tags/tag-badge"
import { pb } from "@/lib/api"
import { $tags, $tagsById } from "@/lib/stores"
import type { TagRecord } from "@/types"
import { getRandomColor, getTagColorClasses, useTags } from "@/lib/tag-utils"

interface TagSelectorDialogProps {
	selectedTags: string[]
	onSelectedTagsChange: (tags: string[]) => void
}

export function TagSelectorDialog({ selectedTags, onSelectedTagsChange }: TagSelectorDialogProps) {
	const availableTags = useStore($tags)
	const selected = useTags(selectedTags)

	async function createTagFromSearch(query: string) {
		const name = query.trim()
		// Tag names are unique case-insensitively, so select the existing tag instead
		const existing = availableTags.find((tag) => tag.name.toLowerCase() === name.toLowerCase())
		if (existing) {
			if (!selectedTags.includes(existing.id)) onSelectedTagsChange([...selectedTags, existing.id])
			return
		}
		try {
			const record = await pb.collection("tags").create<TagRecord>({
				name,
				color: getRandomColor(),
			})
			$tagsById.setKey(record.id, record)
			onSelectedTagsChange([...selectedTags, record.id])
		} catch (e: any) {
			console.error("Failed to create tag", e)
			toast({
				title: t`Failed to create tag`,
				description: e.message || t`Check logs for more details.`,
				variant: "destructive",
			})
			return false
		}
	}

	return (
		<div className="flex flex-col gap-2">
			<SearchableDropdown
				items={availableTags}
				selectedIds={selectedTags}
				onSelectionChange={onSelectedTagsChange}
				getItemId={(tag) => tag.id}
				getItemLabel={(tag) => tag.name}
				renderItem={(tag) => <TagBadge tag={tag} />}
				displayText={
					selected.length === 0
						? t`Select tags...`
						: selected.length === 1
							? selected[0].name
							: t`${selected.length} tags selected`
				}
				placeholder={t`Search or create tag...`}
				onCreateItem={createTagFromSearch}
				createMessage={(query) => (
					<div className="py-3 px-2 text-center text-sm text-muted-foreground">
						<Trans>Press Enter to create "{query}"</Trans>
					</div>
				)}
				emptyMessage={
					<div className="py-4 text-center text-sm text-muted-foreground">
						<Trans>Type a name and press Enter to create a tag.</Trans>
					</div>
				}
				noResultsMessage={
					<div className="py-4 text-center text-sm text-muted-foreground">
						<Trans>No tags found.</Trans>
					</div>
				}
			/>

			<SelectedBadgeList
				items={selected}
				getItemId={(tag) => tag.id}
				getItemLabel={(tag) => tag.name}
				getItemClasses={(tag) => getTagColorClasses(tag.color)}
				onRemove={(id) => onSelectedTagsChange(selectedTags.filter((tagId) => tagId !== id))}
			/>
		</div>
	)
}
