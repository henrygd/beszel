import { type ReactNode, useCallback, useRef, useState } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ChevronDownIcon, type LucideIcon, SearchIcon, ServerIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { $systems } from "@/lib/stores"
import { cn, supportsNetworkMonitors } from "@/lib/utils"

export function SystemMultiSelect({
	id,
	selectedSystemIds,
	onChange,
	disabled,
	className,
	systemIds,
	placeholder,
	canSelectMore,
}: {
	id: string
	selectedSystemIds: Set<string>
	onChange: (ids: Set<string>) => void
	disabled?: boolean
	className?: string
	/** Limit the options to these systems. Defaults to all systems that support network monitors. */
	systemIds?: string[]
	placeholder?: string
	canSelectMore?: boolean
}) {
	const systems = useStore($systems)
	const { t } = useLingui()
	const options = systems
		.filter((system) => (systemIds ? systemIds.includes(system.id) : supportsNetworkMonitors(system)))
		.map((system) => ({ id: system.id, label: system.name }))
	return (
		<MultiSelect
			id={id}
			options={options}
			selectedIds={selectedSystemIds}
			onChange={onChange}
			disabled={disabled}
			className={className}
			icon={ServerIcon}
			canSelectMore={canSelectMore}
			placeholder={placeholder ?? t`Select systems`}
			searchPlaceholder={t`Search systems`}
			emptyText={<Trans>No systems found.</Trans>}
		/>
	)
}

type MultiSelectOption = { id: string; label: string }

export function MultiSelect<T extends MultiSelectOption>({
	id,
	options,
	selectedIds,
	onChange,
	disabled,
	className,
	icon: Icon,
	placeholder,
	searchPlaceholder,
	emptyText,
	renderOption = (option) => <span className="truncate">{option.label}</span>,
	canSelectMore = true,
}: {
	id: string
	options: T[]
	selectedIds: Set<string>
	onChange: (ids: Set<string>) => void
	disabled?: boolean
	className?: string
	icon: LucideIcon
	placeholder: string
	searchPlaceholder: string
	emptyText: ReactNode
	renderOption?: (option: T) => ReactNode
	/** False once the selection is full; only already selected options can then be toggled. */
	canSelectMore?: boolean
}) {
	const { t } = useLingui()
	const [search, setSearch] = useState("")
	const searchRef = useRef<HTMLInputElement>(null)
	const focusSearchOnMount = useCallback((node: HTMLInputElement | null) => {
		searchRef.current = node
		if (!node) return
		// Focus after the menu has completed its own initial focus handling.
		const frame = requestAnimationFrame(() => node.focus())
		return () => cancelAnimationFrame(frame)
	}, [])
	const contentRef = useRef<HTMLDivElement>(null)
	const query = search.trim().toLocaleLowerCase()
	const filteredOptions = options.filter((option) => option.label.toLocaleLowerCase().includes(query))
	const allSelected = filteredOptions.every((option) => selectedIds.has(option.id))
	const anySelected = filteredOptions.some((option) => selectedIds.has(option.id))

	const selectFiltered = (selected: boolean) => {
		const next = new Set(selectedIds)
		for (const option of filteredOptions) {
			if (selected) next.add(option.id)
			else next.delete(option.id)
		}
		onChange(next)
	}
	return (
		<DropdownMenu onOpenChange={() => setSearch("")}>
			<DropdownMenuTrigger asChild>
				<Button
					id={id}
					disabled={disabled}
					type="button"
					variant="outline"
					className={cn("relative w-full min-w-0 ps-10 pe-10 justify-start font-normal text-start", className)}
				>
					<Icon className="size-3.5 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
					<span className="truncate">
						{selectedIds.size === 0
							? placeholder
							: selectedIds.size === 1
								? options.find((option) => selectedIds.has(option.id))?.label
								: t`${selectedIds.size} selected`}
					</span>
					<ChevronDownIcon className="size-4 absolute end-4 top-1/2 -translate-y-1/2 opacity-50" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				ref={contentRef}
				onKeyDown={(event) => {
					if (event.key === "Tab") {
						event.preventDefault()
						searchRef.current?.focus()
					}
				}}
				align="start"
				className="w-[var(--radix-dropdown-menu-trigger-width)] max-h-[min(20rem,var(--radix-dropdown-menu-content-available-height))] flex flex-col overflow-hidden"
			>
				<div className="shrink-0 border-b mb-1">
					<div className="flex items-center gap-2 px-2.5">
						<SearchIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
						<Input
							ref={focusSearchOnMount}
							value={search}
							onChange={(event) => setSearch(event.target.value)}
							placeholder={searchPlaceholder}
							aria-label={searchPlaceholder}
							className="h-10 min-w-0 rounded-none border-0 bg-transparent px-0 shadow-none focus-visible:ring-0 focus-visible:ring-offset-0"
							onKeyDown={(event) => {
								if (event.key === "Escape") return
								// Keep menu typeahead and form submission from consuming search input.
								event.stopPropagation()
								if (event.key === "Enter") event.preventDefault()
								if (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Tab") {
									event.preventDefault()
									const items = contentRef.current?.querySelectorAll<HTMLElement>(
										'[role^="menuitem"]:not([data-disabled])'
									)
									const index = event.key === "ArrowUp" || event.shiftKey ? (items?.length ?? 1) - 1 : 0
									items?.[index]?.focus()
								}
							}}
						/>
					</div>
					<div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-1 pb-1">
						<div className="flex items-center">
							<DropdownMenuItem
								className="px-1.5 py-1 text-xs text-muted-foreground"
								disabled={!filteredOptions.length || allSelected || !canSelectMore}
								onSelect={(event) => {
									event.preventDefault()
									selectFiltered(true)
								}}
							>
								{query ? <Trans>Select matches</Trans> : <Trans>Select all</Trans>}
							</DropdownMenuItem>
							<span aria-hidden="true" className="text-xs text-muted-foreground/50">
								·
							</span>
							<DropdownMenuItem
								className="px-1.5 py-1 text-xs text-muted-foreground"
								disabled={!anySelected}
								onSelect={(event) => {
									event.preventDefault()
									selectFiltered(false)
								}}
							>
								{query ? <Trans>Clear matches</Trans> : <Trans>Clear all</Trans>}
							</DropdownMenuItem>
						</div>
						<span className="px-1.5 text-xs tabular-nums text-muted-foreground">{t`${selectedIds.size} selected`}</span>
					</div>
				</div>
				<div className="min-h-0 overflow-y-auto">
					{filteredOptions.length === 0 && (
						<output className="block px-2.5 py-3 text-sm text-muted-foreground">{emptyText}</output>
					)}
					{filteredOptions.map((option) => (
						<DropdownMenuCheckboxItem
							key={option.id}
							checked={selectedIds.has(option.id)}
							disabled={!canSelectMore && !selectedIds.has(option.id)}
							onSelect={(event) => event.preventDefault()}
							onCheckedChange={(checked) => {
								const next = new Set(selectedIds)
								if (checked) next.add(option.id)
								else next.delete(option.id)
								onChange(next)
							}}
							className="group min-w-0 gap-2.5 py-2 ps-2.5"
							indicatorClassName="static size-4 shrink-0 rounded border border-input group-data-[state=checked]:border-primary group-data-[state=checked]:bg-primary group-data-[state=checked]:text-primary-foreground [&_svg]:size-3"
						>
							{renderOption(option)}
						</DropdownMenuCheckboxItem>
					))}
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
