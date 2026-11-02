import { t } from "@lingui/core/macro"
import {
	type ColumnDef,
	type ExpandedState,
	flexRender,
	getExpandedRowModel,
	getGroupedRowModel,
	type Row,
	type TableOptions,
} from "@tanstack/react-table"
import { ChevronRightIcon, GroupIcon } from "lucide-react"
import { type ReactNode, useState } from "react"
import {
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu"
import { TableCell, TableRow } from "@/components/ui/table"
import { cn, useBrowserStorage } from "@/lib/utils"

// Prefix of the grouping value given to rows without a group of their own
const SOLO = "\0"

/**
 * Grouping value for a column whose rows may have no group. Each such row gets a group of its
 * own, which `mapGroupedRows` draws as the plain row, instead of all of them forming one group.
 */
export function soloGroupingValue(value: string | null | undefined, id: string) {
	return value || `${SOLO}${id}`
}

/** Whether a grouped row holds a single row that has no group */
export function isSoloGroup<T>(row: Row<T>) {
	return row.getIsGrouped() && String(row.groupingValue).startsWith(SOLO)
}

/** Whether a row sits inside a real group, so it can be indented under the group row */
export function isInGroup<T>(row: Row<T>) {
	// group row ids are `${columnId}:${groupingValue}`
	return row.depth > 0 && !row.parentId?.includes(`:${SOLO}`)
}

/**
 * Grouping state and table options for a table that can group by one column at a time.
 * `options` are the column ids that can be grouped by right now; a stored choice that is not
 * among them (the column is hidden, say) falls back to no grouping.
 */
export function useTableGrouping<TData>(storageKey: string, options: readonly string[]) {
	const [stored, setGroupBy] = useBrowserStorage<string>(storageKey, "")
	const [expanded, setExpanded] = useState<ExpandedState>({})
	const groupBy = options.includes(stored) ? stored : ""

	const tableOptions = {
		getGroupedRowModel: getGroupedRowModel(),
		getExpandedRowModel: getExpandedRowModel(),
		// keep groups open across data refreshes
		autoResetExpanded: false,
		// the grouped value is shown on the group row instead
		groupedColumnMode: "remove",
		onExpandedChange: setExpanded,
	} satisfies Partial<TableOptions<TData>>

	return {
		groupBy,
		setGroupBy,
		tableOptions,
		state: { grouping: groupBy ? [groupBy] : [], expanded },
	}
}

/**
 * Spread into the table's `defaultColumn`. Drops TanStack's default aggregation (shown as raw
 * text), so group rows only fill the columns that define how to aggregate.
 */
export const groupingDefaultColumn = {
	aggregatedCell: undefined,
	aggregationFn: undefined,
} satisfies Partial<ColumnDef<unknown>>

/** Draws grouped rows: plain rows as they are, solo groups as their single row, groups as `GroupRow` */
export function mapGroupedRows<T>(
	rows: Row<T>[],
	renderRow: (row: Row<T>) => ReactNode,
	renderGroup: (row: Row<T>) => ReactNode = (row) => <GroupRow key={row.id} row={row} />
) {
	return rows.map((row) => {
		if (!row.getIsGrouped()) {
			return renderRow(row)
		}
		if (isSoloGroup(row)) {
			return renderRow(row.subRows[0])
		}
		return renderGroup(row)
	})
}

/**
 * Row of a group that expands on click. Each cell draws its column's `aggregatedCell`, or its
 * normal cell when the column only sets an `aggregationFn`. Not memoized: it has to redraw when
 * the row is expanded or collapsed.
 */
export function GroupRow<T>({
	row,
	cellClassName = "py-0 ps-4.5",
	height = 54,
}: {
	row: Row<T>
	cellClassName?: string
	height?: number
}) {
	return (
		<TableRow
			className="cursor-pointer bg-muted/30"
			aria-expanded={row.getIsExpanded()}
			onClick={row.getToggleExpandedHandler()}
		>
			{row.getVisibleCells().map((cell) => {
				const { aggregatedCell, aggregationFn, cell: leafCell } = cell.column.columnDef
				const content = aggregatedCell ?? (aggregationFn ? leafCell : null)
				return (
					<TableCell key={cell.id} className={cellClassName} style={{ height, width: cell.column.getSize() }}>
						{content ? flexRender(content, cell.getContext()) : null}
					</TableCell>
				)
			})}
		</TableRow>
	)
}

/** Group name cell with an expand chevron and the number of rows in the group, for an `aggregatedCell` */
export function GroupNameCell<T>({ row, label, className }: { row: Row<T>; label: ReactNode; className?: string }) {
	return (
		<div className={cn("flex items-center gap-1.5", className)}>
			<ChevronRightIcon
				className={cn("size-4 shrink-0 transition-transform", row.getIsExpanded() && "rotate-90")}
				aria-hidden="true"
			/>
			<span className="truncate font-medium">{label}</span>
			<span className="text-muted-foreground tabular-nums">({row.getLeafRows().length})</span>
		</div>
	)
}

// Menu value for no grouping
const NONE = "\0none"

/** "Group By" section for a table's view menu; place inside a `DropdownMenuContent` */
export function GroupByMenuItems({
	options,
	value,
	onChange,
}: {
	options: { value: string; label: ReactNode }[]
	value: string
	onChange: (value: string) => void
}) {
	return (
		<>
			<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
				<GroupIcon className="size-4" />
				{t`Group By`}
			</DropdownMenuLabel>
			<DropdownMenuSeparator />
			<DropdownMenuRadioGroup
				className="px-1 pb-1"
				value={value || NONE}
				onValueChange={(next) => onChange(next === NONE ? "" : next)}
			>
				<DropdownMenuRadioItem value={NONE} onSelect={(e) => e.preventDefault()}>
					{t`None`}
				</DropdownMenuRadioItem>
				{options.map((option) => (
					<DropdownMenuRadioItem key={option.value} value={option.value} onSelect={(e) => e.preventDefault()}>
						{option.label}
					</DropdownMenuRadioItem>
				))}
			</DropdownMenuRadioGroup>
		</>
	)
}
