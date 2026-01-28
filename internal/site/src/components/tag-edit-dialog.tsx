import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { ChevronDownIcon, XIcon } from "lucide-react"
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
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import type { SystemRecord, TagRecord } from "@/types"
import { tagColors, tagColorClasses, getTagColorClasses, type TagWithSystems } from "@/components/tags-columns"

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
										tagColorClasses[color].split(" ").filter(c => c.startsWith("bg-") && !c.includes("dark")).join(" "),
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
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline" className="justify-between font-normal">
									{selectedSystems.length > 0 ? (
										<span className="truncate">
											{selectedSystems.length === 1
												? systems.find((s) => s.id === selectedSystems[0])?.name
												: t`${selectedSystems.length} systems selected`}
										</span>
									) : (
										<span className="text-muted-foreground">
											<Trans>Select systems...</Trans>
										</span>
									)}
									<ChevronDownIcon className="ml-2 h-4 w-4 shrink-0 opacity-50" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent className="w-80" align="start">
								<div className="px-2 py-1.5">
									<Input
										placeholder={t`Search systems...`}
										value={systemSearchQuery}
										onChange={(e) => onSystemSearchQueryChange(e.target.value)}
										className="h-8"
									/>
								</div>
								<DropdownMenuSeparator />
								<div className="max-h-60 overflow-y-auto">
									{systems
										.filter((system) =>
											system.name.toLowerCase().includes(systemSearchQuery.toLowerCase())
										)
										.map((system) => {
											const isSelected = selectedSystems.includes(system.id)
											return (
												<DropdownMenuCheckboxItem
													key={system.id}
													checked={isSelected}
													onCheckedChange={(checked) => {
														onSelectedSystemsChange(
															checked
																? [...selectedSystems, system.id]
																: selectedSystems.filter((id) => id !== system.id)
														)
													}}
													onSelect={(e) => e.preventDefault()}
												>
													{system.name}
												</DropdownMenuCheckboxItem>
											)
										})}
									{systems.length === 0 && (
										<div className="py-4 text-center text-sm text-muted-foreground">
											<Trans>No systems found.</Trans>
										</div>
									)}
									{systems.length > 0 &&
										systemSearchQuery &&
										systems.filter((s) =>
											s.name.toLowerCase().includes(systemSearchQuery.toLowerCase())
										).length === 0 && (
											<div className="py-4 text-center text-sm text-muted-foreground">
												<Trans>No systems found.</Trans>
											</div>
										)}
								</div>
							</DropdownMenuContent>
						</DropdownMenu>
						{selectedSystems.length > 0 && (
							<div className="flex flex-wrap gap-1.5">
								{selectedSystems.map((systemId) => {
									const system = systems.find((s) => s.id === systemId)
									if (!system) return null
									return (
										<Badge
											key={system.id}
											variant="secondary"
											className="text-xs pointer-events-none"
										>
											{system.name}
											<button
												type="button"
												className="ml-1 hover:bg-muted-foreground/20 rounded-full pointer-events-auto"
												onClick={() => {
													onSelectedSystemsChange(selectedSystems.filter((id) => id !== systemId))
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
