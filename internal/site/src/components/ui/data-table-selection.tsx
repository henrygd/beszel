import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import type { Table } from "@tanstack/react-table"
import { XIcon } from "lucide-react"
import type { ReactNode } from "react"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"

/**
 * Header checkbox that toggles every row matching the current filter, across all pages.
 * Rows hidden by the filter are left untouched so bulk actions never hit them.
 */
export function SelectFilteredRowsCheckbox<TData>({ table }: { table: Table<TData> }) {
	const filteredRows = table.getFilteredRowModel().rows
	const selectedCount = filteredRows.filter((row) => row.getIsSelected()).length
	const allSelected = filteredRows.length > 0 && selectedCount === filteredRows.length
	return (
		<Checkbox
			className="ms-2"
			checked={allSelected || (selectedCount > 0 && "indeterminate")}
			onClick={(event) => event.stopPropagation()}
			onCheckedChange={(value) =>
				table.setRowSelection((prev) => {
					const next = { ...prev }
					for (const row of filteredRows) {
						if (value) {
							next[row.id] = true
						} else {
							delete next[row.id]
						}
					}
					return next
				})
			}
			aria-label={t`Select all`}
		/>
	)
}

/** Bulk action bar shown while rows are selected. Fixed to the bottom on small screens. */
export function SelectionToolbar({
	count,
	onClear,
	children,
}: {
	count: number
	onClear: () => void
	children: ReactNode
}) {
	return (
		<div className="fixed bottom-0 left-0 z-50 flex w-full items-center gap-2 p-4 backdrop-blur-md md-lg:static md-lg:w-auto md-lg:p-0 md-lg:backdrop-blur-none">
			<Button variant="ghost" size="icon" aria-label={t`Clear`} title={t`Clear`} onClick={onClear}>
				<XIcon className="size-4" />
			</Button>
			<span className="me-auto whitespace-nowrap text-sm tabular-nums text-muted-foreground md-lg:me-1">
				<Trans>{count} selected</Trans>
			</span>
			{children}
		</div>
	)
}
