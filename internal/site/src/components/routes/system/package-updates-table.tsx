import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	type Column,
	type ColumnDef,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type SortingState,
	useReactTable,
} from "@tanstack/react-table"
import {
	ArrowUpDownIcon,
	GitCompareArrowsIcon,
	PackageCheckIcon,
	PackageIcon,
	PackageOpenIcon,
	ShieldAlertIcon,
	XIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { Badge, type BadgeProps } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { pb } from "@/lib/api"
import { classifyVersionChange, type VersionChange } from "@/lib/package-updates"
import { cn, formatShortDate } from "@/lib/utils"
import type { PackageUpdate, PackageUpdates } from "@/types"

interface PackageUpdateRow extends PackageUpdate {
	change: VersionChange
}

/** Sort order of version changes, largest first. */
const changeRank: Record<VersionChange, number> = { major: 4, minor: 3, patch: 2, revision: 1, other: 0 }

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
			return t({ message: "Major", context: "Version change" })
		case "minor":
			return t({ message: "Minor", context: "Version change" })
		case "patch":
			return t({ message: "Patch", context: "Version change" })
		case "revision":
			return t({ message: "Revision", context: "Version change" })
		default:
			return t({ message: "Other", context: "Version change" })
	}
}

function HeaderButton({
	column,
	name,
	Icon,
}: {
	column: Column<PackageUpdateRow>
	name: string
	Icon: React.ElementType
}) {
	const isSorted = column.getIsSorted()
	return (
		<Button
			className={cn(
				"h-9 px-3 flex items-center gap-2 duration-50",
				isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90"
			)}
			variant="ghost"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			<Icon className="size-4" />
			{name}
			<ArrowUpDownIcon className="size-4" />
		</Button>
	)
}

function getColumns(securityKnown: boolean): ColumnDef<PackageUpdateRow>[] {
	const columns: ColumnDef<PackageUpdateRow>[] = [
		{
			id: "name",
			accessorFn: (pkg) => pkg.name,
			sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
			header: ({ column }) => <HeaderButton column={column} name={t`Package`} Icon={PackageIcon} />,
			cell: ({ getValue }) => <span className="ms-1.5 block">{getValue() as string}</span>,
		},
		{
			id: "current",
			accessorFn: (pkg) => pkg.current ?? "",
			enableSorting: false,
			header: () => (
				<span className="flex items-center gap-2 px-3">
					<PackageCheckIcon className="size-4" />
					<Trans context="Installed package version">Current</Trans>
				</span>
			),
			cell: ({ getValue }) => (
				<span className="ms-1.5 block font-mono text-xs text-muted-foreground">{(getValue() as string) || "-"}</span>
			),
		},
		{
			id: "available",
			accessorFn: (pkg) => pkg.available,
			enableSorting: false,
			header: () => (
				<span className="flex items-center gap-2 px-3">
					<PackageOpenIcon className="size-4" />
					<Trans context="Package version available to install">Available</Trans>
				</span>
			),
			cell: ({ getValue }) => <span className="ms-1.5 block font-mono text-xs">{getValue() as string}</span>,
		},
		{
			id: "change",
			accessorFn: (pkg) => changeRank[pkg.change],
			header: ({ column }) => <HeaderButton column={column} name={t`Change`} Icon={GitCompareArrowsIcon} />,
			cell: ({ row }) => (
				<Badge variant={changeVariant[row.original.change]} className="ms-1.5">
					{changeLabel(row.original.change)}
				</Badge>
			),
		},
	]
	if (securityKnown) {
		columns.push({
			id: "security",
			accessorFn: (pkg) => (pkg.security ? 1 : 0),
			header: ({ column }) => <HeaderButton column={column} name={t`Security`} Icon={ShieldAlertIcon} />,
			cell: ({ row }) =>
				row.original.security ? (
					<span className="ms-1.5 flex items-center gap-1.5 text-red-600 dark:text-red-400">
						<ShieldAlertIcon className="size-4" />
						<Trans>Security</Trans>
					</span>
				) : null,
		})
	}
	return columns
}

/**
 * Lists pending package updates reported by the agent. The agent caches the result of
 * its background check, so this refetches only when the update counts change.
 */
export default function PackageUpdatesTable({ systemId, counts }: { systemId: string; counts: string }) {
	const [data, setData] = useState<PackageUpdates | null>(null)
	const [error, setError] = useState<string | null>(null)
	const [sorting, setSorting] = useState<SortingState>([{ id: "name", desc: false }])
	const [globalFilter, setGlobalFilter] = useState("")

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
		onSortingChange: setSorting,
		onGlobalFilterChange: setGlobalFilter,
		state: { sorting, globalFilter },
		globalFilterFn: (row, _columnId, filterValue: string) => {
			const pkg = row.original
			const searchString = `${pkg.name} ${pkg.current ?? ""} ${pkg.available} ${changeLabel(pkg.change)}`.toLowerCase()
			return filterValue
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
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
						<div className="relative ms-auto w-full max-w-full md:w-64">
							<Input
								placeholder={t`Filter...`}
								value={globalFilter}
								onChange={(event) => setGlobalFilter(event.target.value)}
								className="px-4 w-full max-w-full md:w-64"
							/>
							{globalFilter && (
								<Button
									type="button"
									variant="ghost"
									size="icon"
									aria-label={t`Clear`}
									className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
									onClick={() => setGlobalFilter("")}
								>
									<XIcon className="h-4 w-4" />
								</Button>
							)}
						</div>
					)}
				</div>
			</CardHeader>
			{error ? (
				<p className="px-2 sm:px-1 text-sm text-muted-foreground">{error}</p>
			) : (
				<div className="h-min max-h-[calc(100dvh-17rem)] max-w-full relative overflow-auto border rounded-md">
					<table className="text-sm w-full text-nowrap">
						<TableHeader className="sticky top-0 z-50 w-full border-b-2">
							{table.getHeaderGroups().map((headerGroup) => (
								<tr key={headerGroup.id}>
									{headerGroup.headers.map((header) => (
										<TableHead className="px-2" key={header.id}>
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
									<TableCell colSpan={columns.length} className="h-24 text-center pointer-events-none">
										{rows.length ? <Trans>No results.</Trans> : <Trans>Up to date</Trans>}
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</table>
				</div>
			)}
		</Card>
	)
}
