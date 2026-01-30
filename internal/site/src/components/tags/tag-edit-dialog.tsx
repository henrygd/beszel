import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { SearchableDropdown } from "@/components/ui/searchable-dropdown"
import { SelectedBadgeList } from "@/components/ui/selected-badge-list"
import { cn } from "@/lib/utils"
import type { SystemRecord, TagRecord } from "@/types"
import { tagColors, tagColorClasses, getTagColorClasses, type TagWithSystems } from "./tags-columns"

interface TagEditDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	editingTag: TagWithSystems | null
	tagName: string
	tagColor: string
	selectedSystems: string[]
	systemSearchQuery: string
	systems: SystemRecord[]
	onTagNameChange: (name: string) => void
	onTagColorChange: (color: string) => void
	onSelectedSystemsChange: (systems: string[]) => void
	onSystemSearchQueryChange: (query: string) => void
	onSave: () => void
}

export function TagEditDialog({
	open,
	onOpenChange,
	editingTag,
	tagName,
	tagColor,
	selectedSystems,
	systemSearchQuery,
	systems,
	onTagNameChange,
	onTagColorChange,
	onSelectedSystemsChange,
	onSystemSearchQueryChange,
	onSave,
}: TagEditDialogProps) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>
						{editingTag ? <Trans>Edit Tag</Trans> : <Trans>Create New Tag</Trans>}
					</DialogTitle>
					<DialogDescription>
						{editingTag ? (
							<Trans>Update the tag name, color, and assigned systems.</Trans>
						) : (
							<Trans>Create a new tag and assign it to systems.</Trans>
						)}
					</DialogDescription>
				</DialogHeader>
				<div className="grid xs:grid-cols-[auto_1fr] gap-y-3 gap-x-4 items-center mt-1">
					<Label htmlFor="tag-name" className="xs:text-end">
						<Trans>Name</Trans>
					</Label>
					<Input
						id="tag-name"
						value={tagName}
						onChange={(e) => onTagNameChange(e.target.value)}
						placeholder={t`e.g., Production, Development`}
						maxLength={50}
					/>
					<Label className="xs:text-end self-start pt-2">
						<Trans>Color</Trans>
					</Label>
					<div className="flex flex-col gap-2">
						<div className="flex flex-wrap gap-1.5">
							{tagColors.map((color) => (
								<button
									key={color}
									type="button"
									onClick={() => onTagColorChange(color)}
									className={cn(
										"w-6 h-6 rounded-full transition-all",
										tagColorClasses[color].split(" ").filter(c => c.startsWith("bg-") || c.startsWith("dark:bg-")).join(" "),
										tagColor === color && "ring-2 ring-offset-2 ring-primary"
									)}
								/>
							))}
						</div>
						<Badge className={cn(getTagColorClasses(tagColor), "self-start")}>
							{tagName || t`Preview`}
						</Badge>
					</div>
					<Label className="xs:text-end self-start pt-2">
						<Trans>Systems</Trans>
					</Label>
					<div className="flex flex-col gap-2">
						<SearchableDropdown
							items={systems}
							selectedIds={selectedSystems}
							searchQuery={systemSearchQuery}
							onSearchChange={onSystemSearchQueryChange}
							onSelectionChange={onSelectedSystemsChange}
							getItemId={(system) => system.id}
							getItemLabel={(system) => system.name}
							renderItem={(system) => system.name}
							placeholder={t`Search systems...`}
							getDisplayText={(count) => (count === 0 ? t`Select systems...` : count === 1 ? systems.find((s) => s.id === selectedSystems[0])?.name || t`1 system selected` : t`${count} systems selected`)}
							emptyMessage={
								<div className="py-4 text-center text-sm text-muted-foreground">
									<Trans>No systems found.</Trans>
								</div>
							}
							noResultsMessage={
								<div className="py-4 text-center text-sm text-muted-foreground">
									<Trans>No systems found.</Trans>
								</div>
							}
						/>
						<SelectedBadgeList
							items={systems}
							selectedIds={selectedSystems}
							getItemId={(system) => system.id}
							getItemLabel={(system) => system.name}
							onRemove={(id) => onSelectedSystemsChange(selectedSystems.filter((sId) => sId !== id))}
							variant="secondary"
						/>
					</div>
				</div>
				<DialogFooter className="mt-4">
					<Button variant="outline" onClick={() => onOpenChange(false)}>
						<Trans>Cancel</Trans>
					</Button>
					<Button onClick={onSave}>
						{editingTag ? <Trans>Save</Trans> : <Trans>Create</Trans>}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
