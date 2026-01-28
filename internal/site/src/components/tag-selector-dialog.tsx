import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { ChevronDownIcon, XIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { toast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import { cn } from "@/lib/utils"
import type { TagRecord } from "@/types"
import { getRandomColor, getTagColorClasses } from "@/components/tags-columns"

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
	async function createTagFromSearch() {
		const name = tagSearchQuery.trim()
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

	return (
		<div className="flex flex-col gap-2">
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="outline" className="justify-between font-normal">
						{selectedTags.length > 0 ? (
							<span className="truncate">
								{selectedTags.length === 1
									? availableTags.find((t) => t.id === selectedTags[0])?.name
									: t`${selectedTags.length} tags selected`}
							</span>
						) : (
							<span className="text-muted-foreground">
								<Trans>Select tags...</Trans>
							</span>
						)}
						<ChevronDownIcon className="ml-2 h-4 w-4 shrink-0 opacity-50" />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent className="w-80" align="start">
					<div className="px-2 py-1.5">
						<Input
							placeholder={t`Search or create tag...`}
							value={tagSearchQuery}
							onChange={(e) => onSearchQueryChange(e.target.value)}
							className="h-8"
							onClick={(e) => e.stopPropagation()}
							onKeyDown={(e) => {
								e.stopPropagation()
								if (e.key === "Enter") {
									e.preventDefault()
									createTagFromSearch()
								}
							}}
						/>
					</div>
					<DropdownMenuSeparator />
					<div className="max-h-60 overflow-y-auto">
						{availableTags
							.filter((tag) =>
								tag.name.toLowerCase().includes(tagSearchQuery.toLowerCase())
							)
							.map((tag) => {
								const isSelected = selectedTags.includes(tag.id)
								return (
									<DropdownMenuCheckboxItem
										key={tag.id}
										checked={isSelected}
										onCheckedChange={(checked) => {
											onSelectedTagsChange(
												checked
													? [...selectedTags, tag.id]
													: selectedTags.filter((id) => id !== tag.id)
											)
										}}
										onSelect={(e) => e.preventDefault()}
									>
										<Badge className={cn("text-xs", getTagColorClasses(tag.color))}>
											{tag.name}
										</Badge>
									</DropdownMenuCheckboxItem>
								)
							})}
						{tagSearchQuery &&
							!availableTags.some(
								(tag) => tag.name.toLowerCase() === tagSearchQuery.toLowerCase()
							) && (
								<div className="py-3 px-2 text-center text-sm text-muted-foreground">
									<Trans>
										Press Enter to create "{tagSearchQuery}"
									</Trans>
								</div>
							)}
						{availableTags.length === 0 && !tagSearchQuery && (
							<div className="py-4 text-center text-sm text-muted-foreground">
								<Trans>Type a name and press Enter to create a tag.</Trans>
							</div>
						)}
						{availableTags.length > 0 &&
							!tagSearchQuery &&
							availableTags.filter((tag) =>
								tag.name.toLowerCase().includes(tagSearchQuery.toLowerCase())
							).length === 0 && (
								<div className="py-4 text-center text-sm text-muted-foreground">
									<Trans>No tags found.</Trans>
								</div>
							)}
					</div>
				</DropdownMenuContent>
			</DropdownMenu>
			{selectedTags.length > 0 && (
				<div className="flex flex-wrap gap-1.5">
					{selectedTags.map((tagId) => {
						const tag = availableTags.find((t) => t.id === tagId)
						if (!tag) return null
						return (
							<Badge
								key={tag.id}
								className={cn("text-xs", getTagColorClasses(tag.color))}
							>
								{tag.name}
								<button
									type="button"
									className="ml-1 hover:bg-black/10 dark:hover:bg-white/20 rounded-full"
									onClick={(e) => {
										e.stopPropagation()
										onSelectedTagsChange(selectedTags.filter((id) => id !== tagId))
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
	)
}
