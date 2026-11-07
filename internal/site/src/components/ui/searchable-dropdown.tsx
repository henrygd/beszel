import { ChevronDownIcon } from "lucide-react"
import { type ReactNode, useState } from "react"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"

interface SearchableDropdownProps<T> {
	items: T[]
	selectedIds: string[]
	onSelectionChange: (ids: string[]) => void
	getItemLabel: (item: T) => string
	getItemId: (item: T) => string
	renderItem: (item: T) => ReactNode
	/** Trigger button text */
	displayText: string
	placeholder?: string
	emptyMessage?: ReactNode
	noResultsMessage?: ReactNode
	/** Shown below the results when `onCreateItem` is set and no item has exactly the searched label */
	createMessage?: (query: string) => ReactNode
	/** Called on Enter with the search text; should also handle an exact match (e.g. by selecting it).
	 *  Return false to keep the search text (e.g. when creation failed). */
	onCreateItem?: (query: string) => Promise<boolean | void> | boolean | void
}

export function SearchableDropdown<T>({
	items,
	selectedIds,
	onSelectionChange,
	getItemLabel,
	getItemId,
	renderItem,
	displayText,
	placeholder = "Search...",
	emptyMessage,
	noResultsMessage,
	createMessage,
	onCreateItem,
}: SearchableDropdownProps<T>) {
	const [searchQuery, setSearchQuery] = useState("")

	const query = searchQuery.trim().toLowerCase()
	const filteredItems = items.filter((item) => getItemLabel(item).toLowerCase().includes(query))
	// offer creation unless an item with exactly this label exists (a partial match shouldn't block it)
	const canCreate = !!onCreateItem && !!query && !items.some((item) => getItemLabel(item).toLowerCase() === query)

	const handleKeyDown = async (e: React.KeyboardEvent<HTMLInputElement>) => {
		// keep keys in the input instead of triggering the menu's typeahead / item focus
		e.stopPropagation()
		if (e.key === "Enter" && query && onCreateItem) {
			e.preventDefault()
			if ((await onCreateItem(searchQuery)) !== false) {
				setSearchQuery("")
			}
		}
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" className="justify-between font-normal">
					<span className="truncate">{displayText}</span>
					<ChevronDownIcon className="ml-2 h-4 w-4 shrink-0 opacity-50" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent className="w-80" align="start">
				<div className="px-2 py-1.5">
					<Input
						placeholder={placeholder}
						value={searchQuery}
						onChange={(e) => setSearchQuery(e.target.value)}
						className="h-8"
						onClick={(e) => e.stopPropagation()}
						onKeyDown={handleKeyDown}
					/>
				</div>
				<DropdownMenuSeparator />
				<div className="max-h-60 overflow-y-auto">
					{filteredItems.length > 0 ? (
						filteredItems.map((item) => {
							const itemId = getItemId(item)
							return (
								<DropdownMenuCheckboxItem
									key={itemId}
									checked={selectedIds.includes(itemId)}
									onCheckedChange={(checked) => {
										onSelectionChange(
											checked ? [...selectedIds, itemId] : selectedIds.filter((id) => id !== itemId)
										)
									}}
									onSelect={(e) => e.preventDefault()}
								>
									{renderItem(item)}
								</DropdownMenuCheckboxItem>
							)
						})
					) : canCreate && createMessage ? null : query ? (
						noResultsMessage || <div className="py-4 text-center text-sm text-muted-foreground">No items found.</div>
					) : (
						emptyMessage || <div className="py-4 text-center text-sm text-muted-foreground">No items available.</div>
					)}
					{canCreate && createMessage?.(searchQuery.trim())}
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
