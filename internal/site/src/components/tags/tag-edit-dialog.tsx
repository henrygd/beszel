import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { useMemo, useState } from "react"
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
import { toast } from "@/components/ui/use-toast"
import { TagBadge } from "@/components/tags/tag-badge"
import { pb } from "@/lib/api"
import { $allSystemsById, $systems } from "@/lib/stores"
import { cn } from "@/lib/utils"
import type { SystemRecord, TagRecord } from "@/types"
import type { TagWithSystems } from "./tags-columns"
import { getRandomColor, getTagColorClasses, syncTagAssignments, tagColors } from "@/lib/tag-utils"

interface TagEditDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	/** Tag to edit, or null to create a new one */
	tag: TagWithSystems | null
}

export function TagEditDialog({ open, onOpenChange, tag }: TagEditDialogProps) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-md">
				{/* DialogContent only mounts while open, so the form state starts fresh each time */}
				<TagEditForm tag={tag} onDone={() => onOpenChange(false)} />
			</DialogContent>
		</Dialog>
	)
}

function TagEditForm({ tag, onDone }: { tag: TagWithSystems | null; onDone: () => void }) {
	const systems = useStore($systems)
	const systemsById = useStore($allSystemsById)
	const [name, setName] = useState(tag?.name ?? "")
	const [color, setColor] = useState(() => tag?.color || (tag ? "blue" : getRandomColor()))
	const [selectedSystems, setSelectedSystems] = useState(() => tag?.systems.map((s) => s.id) ?? [])
	const [saving, setSaving] = useState(false)

	const sortedSystems = useMemo(() => [...systems].sort((a, b) => a.name.localeCompare(b.name)), [systems])
	const selected = selectedSystems.map((id) => systemsById[id]).filter(Boolean) as SystemRecord[]

	async function save() {
		const trimmedName = name.trim()
		if (!trimmedName) {
			toast({
				title: t`Tag name required`,
				description: t`Please enter a tag name.`,
				variant: "destructive",
			})
			return
		}

		setSaving(true)
		try {
			const data = { name: trimmedName, color }
			const tagId = tag
				? (await pb.collection("tags").update<TagRecord>(tag.id, data)).id
				: (await pb.collection("tags").create<TagRecord>(data)).id
			// diff against the live store rather than the snapshot taken when the dialog opened
			const currentSystems = tag ? $systems.get().filter((s) => s.tags?.includes(tagId)).map((s) => s.id) : []
			await syncTagAssignments("systems", tagId, currentSystems, selectedSystems)

			toast(
				tag
					? { title: t`Tag updated`, description: t`The tag has been updated successfully.` }
					: { title: t`Tag created`, description: t`The tag has been created successfully.` }
			)
			onDone()
		} catch (e: any) {
			console.error("Failed to save tag", e)
			toast({
				title: tag ? t`Failed to update tag` : t`Failed to create tag`,
				description: e.message || t`Check logs for more details.`,
				variant: "destructive",
			})
		} finally {
			setSaving(false)
		}
	}

	return (
		<>
			<DialogHeader>
				<DialogTitle>{tag ? <Trans>Edit Tag</Trans> : <Trans>Create New Tag</Trans>}</DialogTitle>
				<DialogDescription>
					{tag ? (
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
					value={name}
					onChange={(e) => setName(e.target.value)}
					placeholder={t`e.g., Production, Development`}
					maxLength={50}
				/>
				<Label className="xs:text-end self-start pt-2">
					<Trans>Color</Trans>
				</Label>
				<div className="flex flex-col gap-2">
					<div className="flex flex-wrap gap-1.5">
						{tagColors.map((c) => (
							<button
								key={c}
								type="button"
								aria-label={c}
								aria-pressed={color === c}
								onClick={() => setColor(c)}
								className={cn(
									"w-6 h-6 rounded-full transition-all",
									getTagColorClasses(c),
									color === c && "ring-2 ring-offset-2 ring-primary"
								)}
							/>
						))}
					</div>
					<TagBadge tag={{ name: name || t`Preview`, color }} interactive className="self-start" />
				</div>
				<Label className="xs:text-end self-start pt-2">
					<Trans>Systems</Trans>
				</Label>
				<div className="flex flex-col gap-2">
					<SearchableDropdown
						items={sortedSystems}
						selectedIds={selectedSystems}
						onSelectionChange={setSelectedSystems}
						getItemId={(system) => system.id}
						getItemLabel={(system) => system.name}
						renderItem={(system) => system.name}
						displayText={
							selected.length === 0
								? t`Select systems...`
								: selected.length === 1
									? selected[0].name
									: t`${selected.length} systems selected`
						}
						placeholder={t`Search systems...`}
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
						items={selected}
						getItemId={(system) => system.id}
						getItemLabel={(system) => system.name}
						onRemove={(id) => setSelectedSystems(selectedSystems.filter((sId) => sId !== id))}
						variant="secondary"
					/>
				</div>
			</div>
			<DialogFooter className="mt-4">
				<Button variant="outline" onClick={onDone}>
					<Trans>Cancel</Trans>
				</Button>
				<Button onClick={save} disabled={saving}>
					{tag ? <Trans>Save</Trans> : <Trans>Create</Trans>}
				</Button>
			</DialogFooter>
		</>
	)
}
