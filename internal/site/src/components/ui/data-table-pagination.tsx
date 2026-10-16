import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import type { PaginationState, Table, Updater } from "@tanstack/react-table"
import { ChevronLeftIcon, ChevronRightIcon, ChevronsLeftIcon, ChevronsRightIcon } from "lucide-react"
import { useCallback, useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { queueUserSettings } from "@/lib/api"
import { $userSettings } from "@/lib/stores"
import { cn } from "@/lib/utils"

const defaultPageSizes = [10, 20, 50, 100, 200]

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

	// move back when the current page no longer exists (e.g. after rows are removed)
	useEffect(() => {
		if (pageIndex > 0 && pageIndex >= pageCount) {
			table.setPageIndex(Math.max(0, pageCount - 1))
		}
	}, [table, pageIndex, pageCount])

	// hide pagination controls when all rows fit on the smallest page size
	const showControls = table.getPrePaginationRowModel().rows.length > pageSizes[0]

	return (
		<div className={cn("flex items-center justify-between ps-1 my-3 tabular-nums", !showControls && "max-lg:hidden")}>
			<div className="text-muted-foreground hidden flex-1 text-sm lg:flex">
				{showSelected ? (
					<Trans>
						{table.getFilteredSelectedRowModel().rows.length} of {table.getFilteredRowModel().rows.length} row(s)
						selected.
					</Trans>
				) : (
					<Trans>{table.getFilteredRowModel().rows.length} row(s)</Trans>
				)}
			</div>
			{showControls && (
				<div className="flex w-full items-center gap-8 lg:w-fit lg:ms-auto">
					<div className="hidden items-center gap-2 lg:flex">
						<Label htmlFor="rows-per-page" className="text-sm font-medium">
							<Trans>Rows per page</Trans>
						</Label>
						<Select
							value={`${table.getState().pagination.pageSize}`}
							onValueChange={(value) => {
								table.setPageSize(Number(value))
							}}
						>
							<SelectTrigger className="w-18" id="rows-per-page">
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
					<div className="flex w-fit items-center justify-center text-sm font-medium">
						<Trans>
							Page {table.getState().pagination.pageIndex + 1} of {table.getPageCount()}
						</Trans>
					</div>
					<div className="ms-auto flex items-center gap-2 lg:ms-0">
						<Button
							variant="outline"
							className="hidden size-9 p-0 lg:flex"
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
							className="hidden size-9 lg:flex"
							size="icon"
							onClick={() => table.setPageIndex(table.getPageCount() - 1)}
							disabled={!table.getCanNextPage()}
						>
							<span className="sr-only">Go to last page</span>
							<ChevronsRightIcon className="size-5" />
						</Button>
					</div>
				</div>
			)}
		</div>
	)
}
