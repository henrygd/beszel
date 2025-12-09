import { Trans, useLingui } from "@lingui/react/macro"
import { PencilIcon, PlusIcon, TagIcon, TrashIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { toast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import type { TagRecord } from "@/types"
import { Badge } from "@/components/ui/badge"

export default function TagsSettings() {
	const { t } = useLingui()
	const [tags, setTags] = useState<TagRecord[]>([])
	const [loading, setLoading] = useState(true)
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editingTag, setEditingTag] = useState<TagRecord | null>(null)
	const [newTagName, setNewTagName] = useState("")
	const [newTagColor, setNewTagColor] = useState("#3b82f6")

	useEffect(() => {
		loadTags()
	}, [])

	async function loadTags() {
		try {
			const records = await pb.collection("tags").getFullList<TagRecord>({
				sort: "name",
			})
			setTags(records)
		} catch (e) {
			console.error("Failed to load tags", e)
			toast({
				title: t`Failed to load tags`,
				description: t`Check logs for more details.`,
				variant: "destructive",
			})
		} finally {
			setLoading(false)
		}
	}

	function openCreateDialog() {
		setEditingTag(null)
		setNewTagName("")
		setNewTagColor("#3b82f6")
		setDialogOpen(true)
	}

	function openEditDialog(tag: TagRecord) {
		setEditingTag(tag)
		setNewTagName(tag.name)
		setNewTagColor(tag.color || "#3b82f6")
		setDialogOpen(true)
	}

	async function saveTag() {
		if (!newTagName.trim()) {
			toast({
				title: t`Tag name required`,
				description: t`Please enter a tag name.`,
				variant: "destructive",
			})
			return
		}

		try {
			if (editingTag) {
				// Update existing tag
				const record = await pb.collection("tags").update<TagRecord>(editingTag.id, {
					name: newTagName.trim(),
					color: newTagColor,
				})
				setTags(tags.map((t) => (t.id === record.id ? record : t)).sort((a, b) => a.name.localeCompare(b.name)))
				toast({
					title: t`Tag updated`,
					description: t`The tag has been updated successfully.`,
				})
			} else {
				// Create new tag
				const record = await pb.collection("tags").create<TagRecord>({
					name: newTagName.trim(),
					color: newTagColor,
				})
				setTags([...tags, record].sort((a, b) => a.name.localeCompare(b.name)))
				toast({
					title: t`Tag created`,
					description: t`The tag has been created successfully.`,
				})
			}
			setNewTagName("")
			setNewTagColor("#3b82f6")
			setEditingTag(null)
			setDialogOpen(false)
		} catch (e: any) {
			console.error("Failed to save tag", e)
			toast({
				title: editingTag ? t`Failed to update tag` : t`Failed to create tag`,
				description: e.message || t`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	async function deleteTag(id: string, name: string) {
		if (!confirm(t`Are you sure you want to delete the tag "${name}"? This will remove it from all systems.`)) {
			return
		}

		try {
			await pb.collection("tags").delete(id)
			setTags(tags.filter((tag) => tag.id !== id))
			toast({
				title: t`Tag deleted`,
				description: t`The tag has been deleted successfully.`,
			})
		} catch (e) {
			console.error("Failed to delete tag", e)
			toast({
				title: t`Failed to delete tag`,
				description: t`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	if (loading) {
		return (
			<div className="py-8 text-center text-muted-foreground">
				<Trans>Loading...</Trans>
			</div>
		)
	}

	return (
		<div className="space-y-6">
			<div>
				<h3 className="text-lg font-medium mb-2">
					<Trans>Tags</Trans>
				</h3>
				<p className="text-sm text-muted-foreground mb-4">
					<Trans>Create and manage tags to organize your systems.</Trans>
				</p>
			</div>

			<Button onClick={openCreateDialog}>
				<PlusIcon className="me-2 size-4" />
				<Trans>Create Tag</Trans>
			</Button>

			<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>
							{editingTag ? <Trans>Edit Tag</Trans> : <Trans>Create New Tag</Trans>}
						</DialogTitle>
						<DialogDescription>
							{editingTag ? (
								<Trans>Update the name and color for this tag.</Trans>
							) : (
								<Trans>Enter a name and color for the new tag.</Trans>
							)}
						</DialogDescription>
					</DialogHeader>
					<div className="grid gap-4 py-4">
						<div className="grid gap-2">
							<Label htmlFor="tag-name">
								<Trans>Name</Trans>
							</Label>
							<Input
								id="tag-name"
								value={newTagName}
								onChange={(e) => setNewTagName(e.target.value)}
								placeholder={t`e.g., Production, Development, etc.`}
								maxLength={50}
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										saveTag()
									}
								}}
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="tag-color">
								<Trans>Color</Trans>
							</Label>
							<div className="flex gap-2">
								<Input
									id="tag-color"
									type="color"
									value={newTagColor}
									onChange={(e) => setNewTagColor(e.target.value)}
									className="w-20 h-10 p-1 cursor-pointer"
								/>
								<Input
									type="text"
									value={newTagColor}
									onChange={(e) => setNewTagColor(e.target.value)}
									placeholder="#3b82f6"
									maxLength={7}
									className="flex-1"
								/>
							</div>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setDialogOpen(false)}>
							<Trans>Cancel</Trans>
						</Button>
						<Button onClick={saveTag}>
							{editingTag ? <Trans>Save</Trans> : <Trans>Create</Trans>}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<div className="space-y-2">
				{tags.length === 0 ? (
					<div className="py-8 text-center text-muted-foreground">
						<TagIcon className="mx-auto mb-2 size-8 opacity-50" />
						<p>
							<Trans>No tags created yet.</Trans>
						</p>
					</div>
				) : (
					<div className="grid gap-2">
						{tags.map((tag) => (
							<div
								key={tag.id}
								className="flex items-center justify-between p-3 border rounded-lg hover:bg-accent/50 transition-colors"
							>
								<div className="flex items-center gap-3">
									<Badge style={{ backgroundColor: tag.color || "#3b82f6" }} className="text-white">
										{tag.name}
									</Badge>
								</div>
								<div className="flex gap-1">
									<Button
										variant="ghost"
										size="sm"
										onClick={() => openEditDialog(tag)}
										className="hover:text-foreground"
									>
										<PencilIcon className="size-4" />
									</Button>
									<Button
										variant="ghost"
										size="sm"
										onClick={() => deleteTag(tag.id, tag.name)}
										className="text-destructive hover:text-destructive"
									>
										<TrashIcon className="size-4" />
									</Button>
								</div>
							</div>
						))}
					</div>
				)}
			</div>
		</div>
	)
}
