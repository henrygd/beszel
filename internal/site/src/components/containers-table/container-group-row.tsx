import { t } from "@lingui/core/macro"
import { Plural } from "@lingui/react/macro"
import { flexRender, type Row } from "@tanstack/react-table"
import { ChevronDownIcon } from "lucide-react"
import { memo } from "react"
import { TableCell, TableRow } from "@/components/ui/table"
import { rowGroupLabel } from "@/lib/container-grouping"
import { cn } from "@/lib/utils"
import type { ContainerRecord } from "@/types"

/** Indent of one level of nesting, used by the group rows and by the rows below them */
export const GROUP_INDENT = 18

/**
 * Row of a group (a stack or a service inside it).
 *
 * Each cell is rendered the way TanStack splits them: the cell of the column this row is grouped
 * on keeps its normal `cell`, a column with an aggregate uses its `aggregatedCell`, and the cell
 * of another grouping level is a placeholder and stays empty. The `aggregatedCell` has to come
 * from the column: the grouping feature installs a default one that just prints the value, so
 * falling back to `cell` here would never happen and the group row would lose every formatter.
 *
 * `expanded` is not read in the body -- it is a prop so that memo sees the change. TanStack keeps
 * the same row objects when only the expanded state changes, so comparing `row` alone would stop
 * the chevron from turning.
 */
export const ContainerGroupRow = memo(function ContainerGroupRow({
	row,
	expanded,
}: {
	row: Row<ContainerRecord>
	expanded: boolean
}) {
	return (
		<TableRow
			className="bg-muted/40 dark:bg-muted/25 font-medium cursor-pointer"
			data-depth={row.depth}
			data-expanded={expanded}
			onClick={row.getToggleExpandedHandler()}
		>
			{row.getVisibleCells().map((cell) => (
				<TableCell
					key={cell.id}
					className="py-0 ps-4.5"
					style={{
						height: 54,
						width: cell.column.getSize(),
					}}
				>
					{cell.getIsPlaceholder()
						? null
						: flexRender(
								cell.getIsAggregated() ? cell.column.columnDef.aggregatedCell : cell.column.columnDef.cell,
								cell.getContext()
							)}
				</TableCell>
			))}
		</TableRow>
	)
})

/**
 * Chevron, group label and number of containers. Rendered by the aggregated cell of the name
 * column: the column the grouping runs on never gets its aggregation called, so the counter
 * cannot live on the stack or service column.
 */
export function ContainerGroupLabel({ row, count }: { row: Row<ContainerRecord>; count: number }) {
	const expanded = row.getIsExpanded()
	return (
		<div className="flex items-center gap-1.5" style={{ paddingInlineStart: row.depth * GROUP_INDENT }}>
			<button
				type="button"
				aria-expanded={expanded}
				aria-label={expanded ? t`Collapse group` : t`Expand group`}
				className="shrink-0 rounded-sm p-1 -ms-1 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
				onClick={(event) => {
					event.stopPropagation()
					row.toggleExpanded()
				}}
			>
				<ChevronDownIcon
					className={cn("size-4 transition-transform duration-150", !expanded && "-rotate-90 rtl:rotate-90")}
					aria-hidden="true"
				/>
			</button>
			<span className="truncate xl:w-40" title={rowGroupLabel(row)}>
				{rowGroupLabel(row)}
			</span>
			<span className="shrink-0 text-xs text-muted-foreground tabular-nums">
				<Plural value={count} one="# container" other="# containers" />
			</span>
		</div>
	)
}
