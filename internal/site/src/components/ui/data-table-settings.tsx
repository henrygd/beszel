import { Trans, useLingui } from "@lingui/react/macro"
import {
	type Column,
	functionalUpdate,
	type HeaderContext,
	type RowData,
	type SortingState,
	type Table,
	type Updater,
	type VisibilityState,
} from "@tanstack/react-table"
import { ArrowDownIcon, ArrowUpDownIcon, ArrowUpIcon, EyeIcon, Settings2Icon, XIcon } from "lucide-react"
import { subscribeKeys } from "nanostores"
import { Children, type ElementType, type ReactNode, useCallback, useEffect, useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { queueUserSettings } from "@/lib/api"
import { $userSettings } from "@/lib/stores"
import { cn } from "@/lib/utils"
import type { UserSettings } from "@/types"

declare module "@tanstack/react-table" {
	interface ColumnMeta<TData extends RowData, TValue> {
		/** Column name shown in the header and settings menu. A function so it's translated at render time. */
		label?: () => string
		/** Icon shown next to the column name in the header */
		Icon?: ElementType
		/** Leave out the sort direction icon in the header (for narrow columns) */
		hideSortIcon?: boolean
	}
}

type KeysOfType<T> = {
	[K in keyof UserSettings]-?: NonNullable<UserSettings[K]> extends T ? K : never
}[keyof UserSettings]
type SortSettingsKey = KeysOfType<SortingState>
type ColsSettingsKey = KeysOfType<VisibilityState>

/** Reads a table setting from user settings, falling back to the browser copy until server settings load. */
function readSetting<T>(key: keyof UserSettings): T | undefined {
	const value = $userSettings.get()[key]
	if (value !== undefined) {
		return value as T
	}
	try {
		return JSON.parse(localStorage.getItem(`besz-${key}`) || "null") ?? undefined
	} catch {
		return undefined
	}
}

function saveSetting(key: SortSettingsKey | ColsSettingsKey, value: SortingState | VisibilityState) {
	try {
		localStorage.setItem(`besz-${key}`, JSON.stringify(value))
	} catch {}
	$userSettings.setKey(key, value as never)
	queueUserSettings({ [key]: value })
}

/**
 * Sorting and column visibility for a data table, saved to the user's settings so they
 * follow the user across devices. Pass the returned values and handlers to useReactTable.
 */
export function useTableSettings({
	colsKey,
	sortKey,
	defaultSort,
	onSortingChange,
}: {
	/** User settings key for visible columns */
	colsKey: ColsSettingsKey
	/** User settings key for sorting. Use a separate key when the table's columns differ by page. */
	sortKey: SortSettingsKey
	defaultSort: SortingState
	/** Called after the user changes sorting (e.g. to reset the page index) */
	onSortingChange?: () => void
}) {
	const [sorting, setSorting] = useState<SortingState>(() => readSetting(sortKey) ?? defaultSort)
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>(() => readSetting(colsKey) ?? {})
	const sortingRef = useRef(sorting)
	const visibilityRef = useRef(columnVisibility)
	const onSortingChangeRef = useRef(onSortingChange)
	onSortingChangeRef.current = onSortingChange

	// Apply settings from server once they load (handles incognito / new devices)
	useEffect(() => {
		const applied = new Set<string>()
		return subscribeKeys($userSettings, [colsKey, sortKey], (vals) => {
			const cols = vals[colsKey] as VisibilityState | undefined
			if (!applied.has(colsKey) && cols !== undefined) {
				applied.add(colsKey)
				visibilityRef.current = cols
				setColumnVisibility(cols)
			}
			const sort = vals[sortKey] as SortingState | undefined
			if (!applied.has(sortKey) && sort !== undefined) {
				applied.add(sortKey)
				sortingRef.current = sort
				setSorting(sort)
			}
		})
	}, [colsKey, sortKey])

	const handleSortingChange = useCallback(
		(updater: Updater<SortingState>) => {
			const next = functionalUpdate(updater, sortingRef.current)
			sortingRef.current = next
			setSorting(next)
			saveSetting(sortKey, next)
			onSortingChangeRef.current?.()
		},
		[sortKey]
	)

	const handleColumnVisibilityChange = useCallback(
		(updater: Updater<VisibilityState>) => {
			const next = functionalUpdate(updater, visibilityRef.current)
			visibilityRef.current = next
			setColumnVisibility(next)
			saveSetting(colsKey, next)
		},
		[colsKey]
	)

	return {
		sorting,
		columnVisibility,
		onSortingChange: handleSortingChange,
		onColumnVisibilityChange: handleColumnVisibilityChange,
	}
}

/** Name of a column for menus: its `meta.label`, or the column id if it has none. */
export function getColumnLabel<TData, TValue>(column: Column<TData, TValue>) {
	return column.columnDef.meta?.label?.() ?? column.id
}

/** Sort direction icon, matching the arrows in the settings menu. Dimmed ⇅ when the column isn't sorted. */
function SortIcon({ sorted }: { sorted: false | "asc" | "desc" }) {
	if (sorted === "desc") {
		return <ArrowUpIcon className="size-4" />
	}
	if (sorted === "asc") {
		return <ArrowDownIcon className="size-4" />
	}
	return <ArrowUpDownIcon className="size-4 opacity-50" />
}

/**
 * Column header showing `meta.Icon` and `meta.label`. Sortable columns get a button that
 * toggles sorting, shows the sort direction, and is highlighted while the column is sorted.
 * Use as a column's `header`: `header: sortableHeader`.
 */
export function sortableHeader<TData, TValue>({ column }: HeaderContext<TData, TValue>) {
	const meta = column.columnDef.meta
	const Icon = meta?.Icon
	const content = (
		<>
			{Icon && <Icon className="size-4" />}
			{getColumnLabel(column)}
		</>
	)
	if (!column.getCanSort()) {
		return <span className="h-9 px-3 flex items-center gap-2">{content}</span>
	}
	const sorted = column.getIsSorted()
	return (
		<Button
			className={cn(
				"h-9 px-3 flex items-center gap-2 duration-50",
				sorted && "bg-accent/70 light:bg-accent text-accent-foreground/90"
			)}
			variant="ghost"
			onClick={() => column.toggleSorting(sorted === "asc")}
		>
			{content}
			{!meta?.hideSortIcon && <SortIcon sorted={sorted} />}
		</Button>
	)
}

/**
 * Filter input for a data table, with a button to clear it. Full width on small screens and
 * 16rem from the `md` breakpoint; pass `className` to switch at a different breakpoint.
 */
export function TableFilterInput({
	value,
	onChange,
	className = "md:flex-none md:w-64",
}: {
	value: string
	onChange: (value: string) => void
	className?: string
}) {
	const { t } = useLingui()
	return (
		<div className={cn("relative flex-1", className)}>
			<Input
				placeholder={t`Filter...`}
				value={value}
				onChange={(e) => onChange(e.target.value)}
				className="ps-4 pe-10 w-full"
			/>
			{value && (
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label={t`Clear`}
					className="absolute end-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
					onClick={() => onChange("")}
				>
					<XIcon className="h-4 w-4" />
				</Button>
			)}
		</div>
	)
}

/** A titled section of the table settings menu. */
export function TableSettingsSection({
	Icon,
	title,
	children,
}: {
	Icon: ElementType
	title: ReactNode
	children: ReactNode
}) {
	return (
		<div>
			<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
				<Icon className="size-4" />
				{title}
			</DropdownMenuLabel>
			<DropdownMenuSeparator />
			<div className="px-1 pb-1">{children}</div>
		</div>
	)
}

const sectionGridCols = ["", "", "md:grid-cols-2", "md:grid-cols-3", "md:grid-cols-4"]

/**
 * Settings button for a data table with "Sort By" and "Visible Fields" sections.
 * Extra sections (e.g. layout or status filters) can be passed as children and are shown first.
 */
export function TableSettingsMenu<TData>({ table, children }: { table: Table<TData>; children?: ReactNode }) {
	const { t } = useLingui()
	const sorting = table.getState().sorting
	const columns = table.getAllLeafColumns()
	const sortableColumns = columns.filter((column) => column.getCanSort())
	const hideableColumns = columns.filter((column) => column.getCanHide())
	const sectionCount = Children.count(children) + (sortableColumns.length ? 1 : 0) + (hideableColumns.length ? 1 : 0)

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button aria-label={t`View`} variant="outline" size="icon" className="shrink-0">
					<Settings2Icon className="size-4 opacity-80" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
				<div
					className={cn(
						"grid grid-cols-1 divide-y md:divide-y-0 [&>*:not(:last-child)]:border-r",
						sectionGridCols[sectionCount]
					)}
				>
					{children}
					{sortableColumns.length > 0 && (
						<TableSettingsSection Icon={ArrowUpDownIcon} title={<Trans>Sort By</Trans>}>
							{sortableColumns.map((column) => {
								const isSorted = sorting[0]?.id === column.id
								let Icon = <span className="w-6"></span>
								if (isSorted) {
									Icon = sorting[0].desc ? (
										<ArrowUpIcon className="me-2 size-4" />
									) : (
										<ArrowDownIcon className="me-2 size-4" />
									)
								}
								return (
									<DropdownMenuItem
										key={column.id}
										onSelect={(e) => {
											e.preventDefault()
											table.setSorting([{ id: column.id, desc: isSorted && !sorting[0].desc }])
										}}
									>
										{Icon}
										{getColumnLabel(column)}
									</DropdownMenuItem>
								)
							})}
						</TableSettingsSection>
					)}
					{hideableColumns.length > 0 && (
						<TableSettingsSection Icon={EyeIcon} title={<Trans>Visible Fields</Trans>}>
							{hideableColumns.map((column) => (
								<DropdownMenuCheckboxItem
									key={column.id}
									onSelect={(e) => e.preventDefault()}
									checked={column.getIsVisible()}
									onCheckedChange={(value) => column.toggleVisibility(!!value)}
								>
									{getColumnLabel(column)}
								</DropdownMenuCheckboxItem>
							))}
						</TableSettingsSection>
					)}
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
