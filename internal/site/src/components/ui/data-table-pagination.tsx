import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import type { PaginationState, Table, Updater } from "@tanstack/react-table"
import { ChevronLeftIcon, ChevronRightIcon, ChevronsLeftIcon, ChevronsRightIcon } from "lucide-react"
import { useCallback, useEffect, useId, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { queueUserSettings } from "@/lib/api"
import { $userSettings } from "@/lib/stores"

const defaultPageSizes = [10, 15, 20, 50, 100, 200]

/**
 * Pagination state for a table. The page size is a user setting shared by all tables.
 * Pass `pagination` to table state and `onPaginationChange` to the table options along with
 * `autoResetPageIndex: false`, so realtime data updates don't jump back to the first page.
 * Call `resetPageIndex` when the filter or sort changes.
 */
export function usePagination() {
	const { pageSize: savedPageSize = getLocalPageSize() } = useStore($userSettings, { keys: ["pageSize"] })
	// saved sizes below the smallest option (e.g. the removed 5) fall back to the smallest option
	const pageSize = Math.max(savedPageSize, defaultPageSizes[0])
	const [pageIndex, setPageIndex] = useState(0)
	const pagination = useMemo<PaginationState>(() => ({ pageIndex, pageSize }), [pageIndex, pageSize])
	const onPaginationChange = useCallback(
		(updater: Updater<PaginationState>) => {
			const next = typeof updater === "function" ? updater(pagination) : updater
			setPageIndex(next.pageIndex)
			if (next.pageSize !== pageSize) {
				localStorage.setItem("besz-pageSize", JSON.stringify(next.pageSize))
				$userSettings.setKey("pageSize", next.pageSize)
				queueUserSettings({ pageSize: next.pageSize })
			}
		},
		[pagination, pageSize]
	)
	const resetPageIndex = useCallback(() => setPageIndex(0), [])
	return { pagination, onPaginationChange, resetPageIndex }
}

/** Page size saved on this device, used until user settings load from the server */
function getLocalPageSize(): number {
	try {
		return JSON.parse(localStorage.getItem("besz-pageSize") || "null") ?? 20
	} catch {
		return 20
	}
}

export function DataTablePagination<T>({
	table,
	pageSizes = defaultPageSizes,
	showSelected = true,
}: {
	table: Table<T>
	pageSizes?: number[]
	showSelected?: boolean
}) {
	const { pageIndex } = table.getState().pagination
	const pageCount = table.getPageCount()
	// unique per instance, since several tables can be on the same page
	const selectId = useId()

	// move back when the current page no longer exists (e.g. after rows are removed)
	useEffect(() => {
		if (pageIndex > 0 && pageIndex >= pageCount) {
			table.setPageIndex(Math.max(0, pageCount - 1))
		}
	}, [table, pageIndex, pageCount])

	return (
		// sections show based on the width of this container rather than the viewport,
		// since tables are often rendered in cards narrower than the screen
		<div className="@container mt-3.5">
			<div className="flex items-center gap-4 ps-1 tabular-nums @xl:gap-6 @3xl:gap-8">
				<div className="text-muted-foreground hidden flex-1 text-sm @3xl:flex">
					{showSelected ? (
						<Trans>
							{table.getFilteredSelectedRowModel().rows.length} of {table.getFilteredRowModel().rows.length} row(s)
							selected.
						</Trans>
					) : (
						<Trans>{table.getFilteredRowModel().rows.length} row(s)</Trans>
					)}
				</div>
				<div className="hidden items-center gap-2 @xs:flex">
					<Label htmlFor={selectId} className="sr-only text-sm font-medium @xl:not-sr-only">
						<Trans>Rows per page</Trans>
					</Label>
					<Select
						value={`${table.getState().pagination.pageSize}`}
						onValueChange={(value) => {
							table.setPageSize(Number(value))
						}}
					>
						<SelectTrigger className="w-18" id={selectId}>
							<SelectValue placeholder={table.getState().pagination.pageSize} />
						</SelectTrigger>
						<SelectContent side="top">
							{pageSizes.map((pageSize) => (
								<SelectItem key={pageSize} value={`${pageSize}`}>
									{pageSize}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				<div className="text-sm font-medium whitespace-nowrap">
					<Trans>
						Page {pageIndex + 1} of {Math.max(1, pageCount)}
					</Trans>
				</div>
				<div className="ms-auto flex items-center gap-2 @3xl:ms-0">
					<Button
						variant="outline"
						className="hidden size-9 @md:flex"
						size="icon"
						onClick={() => table.setPageIndex(0)}
						disabled={!table.getCanPreviousPage()}
					>
						<span className="sr-only">Go to first page</span>
						<ChevronsLeftIcon className="size-5" />
					</Button>
					<Button
						variant="outline"
						className="size-9"
						size="icon"
						onClick={() => table.previousPage()}
						disabled={!table.getCanPreviousPage()}
					>
						<span className="sr-only">Go to previous page</span>
						<ChevronLeftIcon className="size-5" />
					</Button>
					<Button
						variant="outline"
						className="size-9"
						size="icon"
						onClick={() => table.nextPage()}
						disabled={!table.getCanNextPage()}
					>
						<span className="sr-only">Go to next page</span>
						<ChevronRightIcon className="size-5" />
					</Button>
					<Button
						variant="outline"
						className="hidden size-9 @md:flex"
						size="icon"
						onClick={() => table.setPageIndex(pageCount - 1)}
						disabled={!table.getCanNextPage()}
					>
						<span className="sr-only">Go to last page</span>
						<ChevronsRightIcon className="size-5" />
					</Button>
				</div>
			</div>
		</div>
	)
}
