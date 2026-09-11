import { ChevronDownIcon } from "lucide-react"
import { ReactNode, useRef } from "react"
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
	searchQuery: string
	onSearchChange: (query: string) => void
	onSelectionChange: (ids: string[]) => void
	getItemLabel: (item: T) => string
	getItemId: (item: T) => string
	renderItem: (item: T, isSelected: boolean) => ReactNode
	placeholder?: string
	getDisplayText?: (selectedCount: number, selectedLabel?: string) => string
	emptyMessage?: ReactNode
	noResultsMessage?: ReactNode
	createMessage?: ReactNode
	stopPropagation?: boolean
	onCreateItem?: (query: string) => Promise<void>
}

export function SearchableDropdown<T>({
	items,
	selectedIds,
	searchQuery,
	onSearchChange,
	onSelectionChange,
	getItemLabel,
	getItemId,
	renderItem,
	placeholder = "Search...",
	getDisplayText,
	emptyMessage,
	noResultsMessage,
	createMessage,
	stopPropagation = false,
	onCreateItem,
}: SearchableDropdownProps<T>) {
	const inputRef = useRef<HTMLInputElement>(null)

	const filteredItems = items.filter((item) =>
		getItemLabel(item).toLowerCase().includes(searchQuery.toLowerCase())
	)

	const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
		if (stopPropagation) {
			e.stopPropagation()
		}
		if (e.key === "Enter" && searchQuery && onCreateItem && !filteredItems.length) {
			e.preventDefault()
			onCreateItem(searchQuery)
		}
	}

	const defaultDisplayText = (selectedCount: number, selectedLabel?: string) => {
		if (selectedCount === 0) return "Select..."
		if (selectedCount === 1) return selectedLabel || "1 selected"
		return `${selectedCount} selected`
	}

	const displayText = getDisplayText ? getDisplayText(selectedIds.length) : defaultDisplayText(selectedIds.length)

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
						ref={inputRef}
						placeholder={placeholder}
						value={searchQuery}
						onChange={(e) => onSearchChange(e.target.value)}
						className="h-8"
						onClick={(e) => stopPropagation && e.stopPropagation()}
						onKeyDown={handleKeyDown}
					/>
				</div>
				<DropdownMenuSeparator />
				<div className="max-h-60 overflow-y-auto">
					{filteredItems.length > 0 ? (
						filteredItems.map((item) => {
							const itemId = getItemId(item)
							const isSelected = selectedIds.includes(itemId)
							return (
								<DropdownMenuCheckboxItem
									key={itemId}
									checked={isSelected}
									onCheckedChange={(checked) => {
										onSelectionChange(
											checked
												? [...selectedIds, itemId]
												: selectedIds.filter((id) => id !== itemId)
										)
									}}
									onSelect={(e) => e.preventDefault()}
								>
									{renderItem(item, isSelected)}
								</DropdownMenuCheckboxItem>
							)
						})
					) : searchQuery && onCreateItem && createMessage ? (
						createMessage
					) : searchQuery ? (
						noResultsMessage || <div className="py-4 text-center text-sm text-muted-foreground">No items found.</div>
					) : (
						emptyMessage || <div className="py-4 text-center text-sm text-muted-foreground">No items available.</div>
					)}
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
