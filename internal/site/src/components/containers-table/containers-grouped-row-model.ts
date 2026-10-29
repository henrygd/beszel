import { getGroupedRowModel, type RowModel, type Table } from "@tanstack/react-table"
import { shouldUnwrapContainerGroup, unwrapGroups } from "@/lib/container-grouping"
import type { ContainerRecord } from "@/types"

/**
 * The stock grouped row model with wrapper groups removed.
 *
 * Containers without stack or service labels get a unique sentinel grouping value so they don't
 * all land in one "undefined" group, and a service with a single replica doesn't deserve a level
 * of its own. Both are groups of exactly one child that are dropped here, after the library has
 * built the tree. The result is memoized on the identity of the stock model, which table-core
 * already memoizes on the grouping state and the filtered rows, so nothing is recomputed while
 * the data stays the same.
 *
 * This is the only file that knows about the internals of TanStack grouping; the logic itself
 * lives in lib/container-grouping.ts.
 */
export function getContainerGroupedRowModel(): (table: Table<ContainerRecord>) => () => RowModel<ContainerRecord> {
	const groupedRowModel = getGroupedRowModel<ContainerRecord>()
	return (table) => {
		const getStockModel = groupedRowModel(table)
		let lastInput: RowModel<ContainerRecord> | undefined
		let lastOutput: RowModel<ContainerRecord> | undefined
		return () => {
			const stockModel = getStockModel()
			if (stockModel !== lastInput || lastOutput === undefined) {
				lastInput = stockModel
				lastOutput = unwrapGroups(stockModel, shouldUnwrapContainerGroup) as RowModel<ContainerRecord>
			}
			return lastOutput
		}
	}
}
