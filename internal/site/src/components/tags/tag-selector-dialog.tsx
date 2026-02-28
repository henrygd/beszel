import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { Badge } from "@/components/ui/badge"
import { toast } from "@/components/ui/use-toast"
import { SearchableDropdown } from "@/components/ui/searchable-dropdown"
import { SelectedBadgeList } from "@/components/ui/selected-badge-list"
import { pb } from "@/lib/api"
import { cn } from "@/lib/utils"
import type { TagRecord } from "@/types"
import { getRandomColor, getTagColorClasses } from "./tags-columns"

interface TagSelectorDialogProps {
	availableTags: TagRecord[]
	selectedTags: string[]
	tagSearchQuery: string
	onAvailableTagsChange: (tags: TagRecord[]) => void
	onSelectedTagsChange: (tags: string[]) => void
	onSearchQueryChange: (query: string) => void
}

export function TagSelectorDialog({
	availableTags,
	selectedTags,
	tagSearchQuery,
	onAvailableTagsChange,
	onSelectedTagsChange,
	onSearchQueryChange,
}: TagSelectorDialogProps) {
	async function createTagFromSearch(query: string) {
		const name = query.trim()
		if (!name) return
		// Check if tag with this name already exists
		if (availableTags.some((tag) => tag.name.toLowerCase() === name.toLowerCase())) return
		try {
			const record = await pb.collection("tags").create<TagRecord>({
				name,
				color: getRandomColor(),
			})
			onAvailableTagsChange([...availableTags, record].sort((a, b) => a.name.localeCompare(b.name)))
			onSelectedTagsChange([...selectedTags, record.id])
			onSearchQueryChange("")
		} catch (e: any) {
			console.error("Failed to create tag", e)
			toast({
				title: t`Failed to create tag`,
				description: e.message || t`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	const displayText = (selectedCount: number) => {
		if (selectedCount === 0) return t`Select tags...`
		if (selectedCount === 1) {
			const tag = availableTags.find((t) => t.id === selectedTags[0])
			return tag?.name || t`1 tag selected`
		}
		return t`${selectedCount} tags selected`
	}

	return (
		<div className="flex flex-col gap-2">
			<SearchableDropdown
				items={availableTags}
				selectedIds={selectedTags}
				searchQuery={tagSearchQuery}
				onSearchChange={onSearchQueryChange}
				onSelectionChange={onSelectedTagsChange}
				getItemId={(tag) => tag.id}
				getItemLabel={(tag) => tag.name}
				renderItem={(tag) => (
					<Badge className={cn("text-xs pointer-events-none", getTagColorClasses(tag.color))}>
						{tag.name}
					</Badge>
				)}
				placeholder={t`Search or create tag...`}
				getDisplayText={(count) => (count === 1 ? availableTags.find((t) => t.id === selectedTags[0])?.name || t`1 tag selected` : displayText(count))}
				stopPropagation
				onCreateItem={createTagFromSearch}
				createMessage={
					<div className="py-3 px-2 text-center text-sm text-muted-foreground">
						<Trans>Press Enter to create "{tagSearchQuery}"</Trans>
					</div>
				}
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
				items={availableTags}
				selectedIds={selectedTags}
				getItemId={(tag) => tag.id}
				getItemLabel={(tag) => tag.name}
				getItemClasses={(tag) => getTagColorClasses(tag.color)}
				onRemove={(id) => onSelectedTagsChange(selectedTags.filter((tagId) => tagId !== id))}
			/>
		</div>
	)
}
