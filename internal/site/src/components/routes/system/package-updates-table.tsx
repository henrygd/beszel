import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	type Column,
	type ColumnDef,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getPaginationRowModel,
	getSortedRowModel,
	type SortingState,
	useReactTable,
} from "@tanstack/react-table"
import {
	ArrowUpDownIcon,
	GitCompareArrowsIcon,
	LoaderCircleIcon,
	PackageCheckIcon,
	PackageIcon,
	PackageOpenIcon,
	ShieldAlertIcon,
	XIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge, type BadgeProps } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { DataTablePagination, usePagination } from "@/components/ui/data-table-pagination"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"
import { classifyVersionChange, type VersionChange } from "@/lib/package-updates"
import { cn, formatShortDate } from "@/lib/utils"
import type { PackageUpdate, PackageUpdates, PackageUpdatesApplyResult } from "@/types"

type AutoApplyMode = "all" | "security" | "off"

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
			header: ({ column }) => (
				<HeaderButton
					column={column}
					name={t({ message: "Package", comment: "Software package" })}
					Icon={PackageIcon}
				/>
			),
			cell: ({ getValue }) => <span className="ms-1.5 block">{getValue() as string}</span>,
		},
		{
			id: "current",
			accessorFn: (pkg) => pkg.current ?? "",
			enableSorting: false,
			header: () => (
				<span className="flex items-center gap-2 px-3">
					<PackageCheckIcon className="size-4" />
					<Trans comment="Current package version">Current</Trans>
				</span>
			),
			cell: ({ getValue }) => (
				<span className="ms-1.5 block font-mono text-sm text-muted-foreground">{(getValue() as string) || "-"}</span>
			),
		},
		{
			id: "available",
			accessorFn: (pkg) => pkg.available,
			enableSorting: false,
			header: () => (
				<span className="flex items-center gap-2 px-3">
					<PackageOpenIcon className="size-4" />
					<Trans comment="Package version available to install">Available</Trans>
				</span>
			),
			cell: ({ getValue }) => <span className="ms-1.5 block font-mono text-sm">{getValue() as string}</span>,
		},
	]
	if (securityKnown) {
		columns.push({
			id: "security",
			accessorFn: (pkg) => (pkg.security ? 1 : 0),
			header: ({ column }) => (
				<HeaderButton
					column={column}
					name={t({ message: "Security", comment: "Security update" })}
					Icon={ShieldAlertIcon}
				/>
			),
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
		accessorFn: (pkg) => changeRank[pkg.change],
		header: ({ column }) => (
			<HeaderButton
				column={column}
				name={t({ message: `Change`, comment: "Version change" })}
				Icon={GitCompareArrowsIcon}
			/>
		),
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
	const [sorting, setSorting] = useState<SortingState>([{ id: "name", desc: false }])
	const [globalFilter, setGlobalFilter] = useState("")
	const [applyMode, setApplyMode] = useState<"all" | "security" | null>(null)
	const [applying, setApplying] = useState(false)
	const [savingAutoApply, setSavingAutoApply] = useState(false)
	const { pagination, onPaginationChange, resetPageIndex } = usePagination()
	const admin = isAdmin()

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
		onSortingChange: (updater) => {
			setSorting(updater)
			resetPageIndex()
		},
		onGlobalFilterChange: (value) => {
			setGlobalFilter(value)
			resetPageIndex()
		},
		autoResetPageIndex: false,
		onPaginationChange,
		state: { sorting, globalFilter, pagination },
		globalFilterFn: (row, _columnId, filterValue: string) => {
			const pkg = row.original
			const searchString = `${pkg.name} ${pkg.current ?? ""} ${pkg.available} ${changeLabel(pkg.change)}`.toLowerCase()
			return filterValue
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})

	async function applyUpdates(mode: "all" | "security") {
		setApplying(true)
		try {
			// Fire-and-forget: agent applies in the background. The package list
			// refreshes when system.info.pu updates via the normal agent poll.
			const result = await pb.send<PackageUpdatesApplyResult>("/api/beszel/package-updates/apply", {
				method: "POST",
				query: { system: systemId },
				body: { mode },
			})
			if (result.status === "error" || (result.error && !result.ok && result.status !== "started" && result.status !== "running")) {
				toast({
					variant: "destructive",
					title: t`Failed to apply package updates`,
					description: result.error || result.status || t`Unknown error`,
				})
				return
			}
			toast({
				title: mode === "security" ? t`Applying security updates` : t`Applying package updates`,
				description: t`The list will refresh when the agent reports new counts.`,
			})
		} catch (err: any) {
			const detail =
				err?.response?.error || err?.data?.error || err?.message || String(err ?? "")
			toast({
				variant: "destructive",
				title: t`Failed to apply package updates`,
				description: detail,
			})
		} finally {
			setApplying(false)
			setApplyMode(null)
		}
	}

	async function saveAutoApply(mode: AutoApplyMode) {
		setSavingAutoApply(true)
		try {
			const result = await pb.send<{ ok?: boolean; mode?: string; error?: string }>(
				"/api/beszel/package-updates/auto-apply",
				{
					method: "POST",
					query: { system: systemId },
					body: { mode },
				}
			)
			if (!result.ok) {
				toast({
					variant: "destructive",
					title: t`Failed to update auto-apply`,
					description: result.error || t`Unknown error`,
				})
				return
			}
			setData((prev) =>
				prev
					? {
							...prev,
							autoApply: (result.mode as AutoApplyMode) || mode,
						}
					: prev
			)
			toast({
				title: t`Auto-apply updated`,
				description:
					mode === "off"
						? t`Automatic package updates are off for this host.`
						: mode === "security"
							? t`This host will auto-apply security updates.`
							: t`This host will auto-apply all updates.`,
			})
		} catch (err: any) {
			const detail =
				err?.response?.error || err?.data?.error || err?.message || String(err ?? "")
			toast({
				variant: "destructive",
				title: t`Failed to update auto-apply`,
				description: detail,
			})
		} finally {
			setSavingAutoApply(false)
		}
	}

	if (!data && !error) {
		return null
	}

	const securityCount = rows.filter((pkg) => pkg.security).length
	const tableRows = table.getRowModel().rows
	const canApply = admin && !!data?.applyEnabled && rows.length > 0
	const canSetAutoApply = admin && !!data?.applyEnabled
	const autoApplyValue: AutoApplyMode =
		data?.autoApply === "security" || data?.autoApply === "all" ? data.autoApply : "off"

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
					<div className="ms-auto flex flex-wrap items-center gap-2 w-full md:w-auto justify-end">
						{canSetAutoApply && (
							<div className="flex items-center gap-2">
								<span className="text-sm text-muted-foreground whitespace-nowrap">
									<Trans>Auto-apply</Trans>
								</span>
								<Select
									value={autoApplyValue}
									disabled={savingAutoApply}
									onValueChange={(value) => {
										if (value === "all" || value === "security" || value === "off") {
											void saveAutoApply(value)
										}
									}}
								>
									<SelectTrigger className="h-9 w-[9.5rem]" aria-label={t`Auto-apply`}>
										{savingAutoApply ? (
											<LoaderCircleIcon className="size-4 animate-spin" />
										) : (
											<SelectValue />
										)}
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">
											<Trans>All updates</Trans>
										</SelectItem>
										<SelectItem value="security">
											<Trans>Security only</Trans>
										</SelectItem>
										<SelectItem value="off">
											<Trans>Off</Trans>
										</SelectItem>
									</SelectContent>
								</Select>
							</div>
						)}
						{!canSetAutoApply && data?.autoApply && (
							<span className="text-sm text-muted-foreground">
								<Trans>Auto-apply: {data.autoApply}</Trans>
							</span>
						)}
						{canApply && (
							<>
								{securityKnown && securityCount > 0 && (
									<Button
										type="button"
										variant="outline"
										size="sm"
										disabled={applying}
										onClick={() => setApplyMode("security")}
									>
										{applying && applyMode === "security" ? (
											<LoaderCircleIcon className="size-4 animate-spin" />
										) : (
											<ShieldAlertIcon className="size-4" />
										)}
										<Trans>Apply security</Trans>
									</Button>
								)}
								<Button
									type="button"
									variant="default"
									size="sm"
									disabled={applying}
									onClick={() => setApplyMode("all")}
								>
									{applying && applyMode === "all" ? (
										<LoaderCircleIcon className="size-4 animate-spin" />
									) : (
										<PackageCheckIcon className="size-4" />
									)}
									<Trans>Apply all</Trans>
								</Button>
							</>
						)}
						{rows.length > 0 && (
							<div className="relative w-full max-w-full md:w-64">
								<Input
									placeholder={t`Filter...`}
									value={globalFilter}
									onChange={(event) => table.setGlobalFilter(event.target.value)}
									className="px-4 w-full max-w-full md:w-64"
								/>
								{globalFilter && (
									<Button
										type="button"
										variant="ghost"
										size="icon"
										aria-label={t`Clear`}
										className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
										onClick={() => table.setGlobalFilter("")}
									>
										<XIcon className="h-4 w-4" />
									</Button>
								)}
							</div>
						)}
					</div>
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
									<TableCell colSpan={columns.length} className="h-24 text-center pointer-events-none">
										{rows.length ? <Trans>No results.</Trans> : <Trans>Up to date</Trans>}
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</table>
				</div>
			)}
			{!error && <DataTablePagination table={table} showSelected={false} />}
			<AlertDialog open={!!applyMode} onOpenChange={(open) => !open && !applying && setApplyMode(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							{applyMode === "security" ? (
								<Trans>Apply security updates?</Trans>
							) : (
								<Trans>Apply all package updates?</Trans>
							)}
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>
								This will run the host package manager on this system. Reboots are not triggered automatically.
							</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={applying}>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn(buttonVariants({ variant: applyMode === "all" ? "destructive" : "default" }))}
							disabled={applying || !applyMode}
							onClick={(event) => {
								event.preventDefault()
								if (applyMode) void applyUpdates(applyMode)
							}}
						>
							{applying ? <LoaderCircleIcon className="size-4 animate-spin" /> : null}
							<Trans>Continue</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</Card>
	)
}
