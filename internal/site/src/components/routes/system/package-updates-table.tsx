import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	type ColumnDef,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getPaginationRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table"
import {
	GitCompareArrowsIcon,
	PackageCheckIcon,
	PackageIcon,
	PackageOpenIcon,
	ShieldAlertIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { Badge, type BadgeProps } from "@/components/ui/badge"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { DataTablePagination, usePagination } from "@/components/ui/data-table-pagination"
import {
	sortableHeader,
	TableFilterInput,
	TableSettingsMenu,
	useTableSettings,
} from "@/components/ui/data-table-settings"
import { Separator } from "@/components/ui/separator"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { pb } from "@/lib/api"
import { classifyVersionChange, type VersionChange } from "@/lib/package-updates"
import { cn, formatShortDate, matchesTableFilter } from "@/lib/utils"
import type { PackageUpdate, PackageUpdates } from "@/types"

interface PackageUpdateRow extends PackageUpdate {
	change: VersionChange
}

/** Sort order of version changes, so ascending puts major first. */
const changeRank: Record<VersionChange, number> = { major: 0, minor: 1, patch: 2, revision: 3, other: 4 }

const changeVariant: Record<VersionChange, BadgeProps["variant"]> = {
	major: "danger",
	minor: "warning",
	patch: "success",
	revision: "secondary",
	other: "outline",
}

function changeLabel(change: VersionChange) {
	switch (change) {
		case "major":
			return t({ message: "Major", comment: "Package version" })
		case "minor":
			return t({ message: "Minor", comment: "Package version" })
		case "patch":
			return t({ message: "Patch", comment: "Package version" })
		case "revision":
			return t({ message: "Revision", comment: "Package version" })
		default:
			return t({ message: "Other", comment: "Package version" })
	}
}


function getColumns(securityKnown: boolean): ColumnDef<PackageUpdateRow>[] {
	const columns: ColumnDef<PackageUpdateRow>[] = [
		{
			id: "name",
			meta: { label: () => t({ message: "Package", comment: "Software package" }), Icon: PackageIcon },
			enableHiding: false,
			accessorFn: (pkg) => pkg.name,
			sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
			header: sortableHeader,
			cell: ({ getValue }) => <span className="ms-1.5 block">{getValue() as string}</span>,
		},
		{
			id: "current",
			meta: { label: () => t({ message: "Current", comment: "Current package version" }), Icon: PackageCheckIcon },
			accessorFn: (pkg) => pkg.current ?? "",
			enableSorting: false,
			header: sortableHeader,
			cell: ({ getValue }) => (
				<span className="ms-1.5 block font-mono text-sm text-muted-foreground">{(getValue() as string) || "-"}</span>
			),
		},
		{
			id: "available",
			meta: {
				label: () => t({ message: "Available", comment: "Package version available to install" }),
				Icon: PackageOpenIcon,
			},
			accessorFn: (pkg) => pkg.available,
			enableSorting: false,
			header: sortableHeader,
			cell: ({ getValue }) => <span className="ms-1.5 block font-mono text-sm">{getValue() as string}</span>,
		},
	]
	if (securityKnown) {
		columns.push({
			id: "security",
			meta: { label: () => t({ message: "Security", comment: "Security update" }), Icon: ShieldAlertIcon },
			accessorFn: (pkg) => (pkg.security ? 1 : 0),
			header: sortableHeader,
			cell: ({ row }) =>
				row.original.security ? (
					<span className="ms-1.5 flex items-center gap-1.5 text-red-600 dark:text-red-400">
						<ShieldAlertIcon className="size-4" />
						<Trans>Security</Trans>
					</span>
				) : null,
		})
	}
	// Change is always the last column, pinned to the right edge of the table
	columns.push({
		id: "change",
		meta: { label: () => t({ message: `Change`, comment: "Version change" }), Icon: GitCompareArrowsIcon },
		accessorFn: (pkg) => changeRank[pkg.change],
		header: sortableHeader,
		cell: ({ row }) => (
			<Badge variant={changeVariant[row.original.change]} className="ms-1.5">
				{changeLabel(row.original.change)}
			</Badge>
		),
	})
	return columns
}

/**
 * Lists pending package updates reported by the agent. The agent caches the result of
 * its background check, so this refetches only when the update counts change.
 */
export default function PackageUpdatesTable({ systemId, counts }: { systemId: string; counts: string }) {
	const [data, setData] = useState<PackageUpdates | null>(null)
	const [error, setError] = useState<string | null>(null)
	const [globalFilter, setGlobalFilter] = useState("")
	const { pagination, onPaginationChange, resetPageIndex } = usePagination()
	const tableSettings = useTableSettings({
		colsKey: "packageUpdateCols",
		sortKey: "packageUpdateSortMode",
		defaultSort: [{ id: "name", desc: false }],
		onSortingChange: resetPageIndex,
	})

	useEffect(() => {
		let cancelled = false
		pb.send<PackageUpdates>("/api/beszel/package-updates", { query: { system: systemId } })
			.then((result) => {
				if (cancelled) return
				setData(result)
				setError(null)
			})
			.catch((err) => {
				if (cancelled) return
				setError(err?.message || t`Failed to load package updates`)
			})
		return () => {
			cancelled = true
		}
	}, [systemId, counts])

	const rows = useMemo<PackageUpdateRow[]>(
		() => (data?.packages ?? []).map((pkg) => ({ ...pkg, change: classifyVersionChange(pkg.current, pkg.available) })),
		[data]
	)
	const securityKnown = !!data?.securityKnown
	const columns = useMemo(() => getColumns(securityKnown), [securityKnown])

	const table = useReactTable({
		data: rows,
		columns,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		onSortingChange: tableSettings.onSortingChange,
		onColumnVisibilityChange: tableSettings.onColumnVisibilityChange,
		onGlobalFilterChange: (value) => {
			setGlobalFilter(value)
			resetPageIndex()
		},
		autoResetPageIndex: false,
		onPaginationChange,
		state: {
			sorting: tableSettings.sorting,
			columnVisibility: tableSettings.columnVisibility,
			globalFilter,
			pagination,
		},
		globalFilterFn: (row, _columnId, filterValue: string) => {
			const pkg = row.original
			return matchesTableFilter(filterValue, pkg.name, pkg.current, pkg.available, changeLabel(pkg.change))
		},
	})

	if (!data && !error) {
		return null
	}

	const securityCount = rows.filter((pkg) => pkg.security).length
	const tableRows = table.getRowModel().rows

	return (
		<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 mb-3 sm:mb-4">
				<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>Package Updates</Trans>
						</CardTitle>
						<CardDescription className="flex items-center flex-wrap">
							{data?.manager && (
								<>
									<span className="font-mono">{data.manager}</span>
									<Separator orientation="vertical" className="h-4 mx-2 bg-primary/40" />
								</>
							)}
							<Trans>Total: {rows.length}</Trans>
							{securityKnown && (
								<>
									<Separator orientation="vertical" className="h-4 mx-2 bg-primary/40" />
									<Trans>Security: {securityCount}</Trans>
								</>
							)}
							{!!data?.checkedAt && (
								<>
									<Separator orientation="vertical" className="h-4 mx-2 bg-primary/40" />
									<Trans>Checked {formatShortDate(new Date(data.checkedAt * 1000).toISOString())}</Trans>
								</>
							)}
						</CardDescription>
					</div>
					{rows.length > 0 && (
						<div className="flex gap-2 ms-auto w-full md:w-auto">
							<TableFilterInput value={globalFilter} onChange={table.setGlobalFilter} />
							<TableSettingsMenu table={table} />
						</div>
					)}
				</div>
			</CardHeader>
			{error ? (
				<p className="px-2 sm:px-1 text-sm text-muted-foreground">{error}</p>
			) : (
				<div className="max-w-full relative overflow-x-auto border rounded-md">
					<table className="text-sm w-full text-nowrap">
						<TableHeader className="sticky top-0 z-50 w-full border-b-2">
							{table.getHeaderGroups().map((headerGroup) => (
								<tr key={headerGroup.id}>
									{headerGroup.headers.map((header) => (
										<TableHead className={cn("px-2", header.column.id === "change" && "w-0")} key={header.id}>
											{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
										</TableHead>
									))}
								</tr>
							))}
						</TableHeader>
						<TableBody>
							{tableRows.length ? (
								tableRows.map((row) => (
									<TableRow key={row.id}>
										{row.getVisibleCells().map((cell) => (
											<TableCell key={cell.id} className="py-2.5">
												{flexRender(cell.column.columnDef.cell, cell.getContext())}
											</TableCell>
										))}
									</TableRow>
								))
							) : (
								<TableRow>
									<TableCell
										colSpan={table.getVisibleLeafColumns().length}
										className="h-24 text-center pointer-events-none"
									>
										{rows.length ? <Trans>No results.</Trans> : <Trans>Up to date</Trans>}
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</table>
				</div>
			)}
			{!error && <DataTablePagination table={table} showSelected={false} />}
		</Card>
	)
}
