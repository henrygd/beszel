import { ColumnDef } from "@tanstack/react-table"
import { AlertsHistoryRecord } from "@/types"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { formatShortDate, toFixedFloat, formatDuration, cn } from "@/lib/utils"
import { alertInfo } from "@/lib/alerts"
import { Trans } from "@lingui/react/macro"
import { t } from "@lingui/core/macro"

export const alertsHistoryColumns: ColumnDef<AlertsHistoryRecord>[] = [
	{
		accessorKey: "system",
		enableSorting: true,
		header: ({ column }) => (
			<Button variant="ghost" onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}>
				<Trans>System</Trans>
			</Button>
		),
		cell: ({ row }) => (
			<div className="ps-2 max-w-60 truncate">{row.original.expand?.system?.name || row.original.system}</div>
		),
		filterFn: (row, _, filterValue) => {
			const display = row.original.expand?.system?.name || row.original.system || ""
			return display.toLowerCase().includes(filterValue.toLowerCase())
		},
	},
	{
		// accessorKey: "name",
		id: "name",
		accessorFn: (record) => {
			const name = record.name
			const info = alertInfo[name]
			return info?.name().replace("cpu", "CPU") || name
		},
		header: ({ column }) => (
			<Button variant="ghost" onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}>
				<Trans>Name</Trans>
			</Button>
		),
		cell: ({ getValue, row }) => {
			let name = getValue() as string
			const info = alertInfo[row.original.name]
			const Icon = info?.icon

			return (
				<span className="flex items-center gap-2 ps-1 min-w-40">
					{Icon && <Icon className="size-3.5" />}
					{name}
				</span>
			)
		},
	},
	{
		accessorKey: "value",
		enableSorting: false,
		header: () => (
			<Button variant="ghost">
				<Trans>Value</Trans>
			</Button>
		),
		cell({ row, getValue }) {
			const name = row.original.name
			if (name === "Status") {
				return <span className="ps-2">{t`Down`}</span>
			}
			const value = getValue() as number
			const unit = alertInfo[name]?.unit
			return (
				<span className="tabular-nums ps-2.5">
					{toFixedFloat(value, value < 10 ? 2 : 1)}
					{unit}
				</span>
			)
		},
	},
	{
		accessorKey: "state",
		enableSorting: true,
		sortingFn: (rowA, rowB) => (rowA.original.resolved ? 1 : 0) - (rowB.original.resolved ? 1 : 0),
		header: ({ column }) => (
			<Button variant="ghost" onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}>
				<Trans comment="Context: alert state (active or resolved)">State</Trans>
			</Button>
		),
		cell: ({ row }) => {
			const resolved = row.original.resolved
			return (
				<Badge
					className={cn(
						"capitalize pointer-events-none",
						resolved
							? "bg-green-100 text-green-800 border-green-200 dark:opacity-80"
							: "bg-yellow-100 text-yellow-800 border-yellow-200"
					)}
				>
					{/* {resolved ? <CircleCheckIcon className="size-3 me-0.5" /> : <CircleAlertIcon className="size-3 me-0.5" />} */}
					{resolved ? <Trans>Resolved</Trans> : <Trans>Active</Trans>}
				</Badge>
			)
		},
	},
	{
		accessorKey: "created",
		accessorFn: (record) => formatShortDate(record.created),
		enableSorting: true,
		invertSorting: true,
		header: ({ column }) => (
			<Button variant="ghost" onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}>
				<Trans comment="Context: date created">Created</Trans>
			</Button>
		),
		cell: ({ getValue, row }) => (
			<span className="ps-1 tabular-nums tracking-tight" title={`${row.original.created} UTC`}>
				{getValue() as string}
			</span>
		),
	},
	{
		accessorKey: "resolved",
		enableSorting: true,
		invertSorting: true,
		header: ({ column }) => (
			<Button variant="ghost" onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}>
				<Trans>Resolved</Trans>
			</Button>
		),
		cell: ({ row, getValue }) => {
			const resolved = getValue() as string | null
			if (!resolved) {
				return null
			}
			return (
				<span className="ps-1 tabular-nums tracking-tight" title={`${row.original.resolved} UTC`}>
					{formatShortDate(resolved)}
				</span>
			)
		},
	},
	{
		accessorKey: "duration",
		invertSorting: true,
		enableSorting: true,
		sortingFn: (rowA, rowB) => {
			const aCreated = new Date(rowA.original.created)
			const bCreated = new Date(rowB.original.created)
			const aResolved = rowA.original.resolved ? new Date(rowA.original.resolved) : null
			const bResolved = rowB.original.resolved ? new Date(rowB.original.resolved) : null
			const aDuration = aResolved ? aResolved.getTime() - aCreated.getTime() : null
			const bDuration = bResolved ? bResolved.getTime() - bCreated.getTime() : null
			if (!aDuration && bDuration) return -1
			if (aDuration && !bDuration) return 1
			return (aDuration || 0) - (bDuration || 0)
		},
		header: ({ column }) => (
			<Button variant="ghost" onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}>
				<Trans>Duration</Trans>
			</Button>
		),
		cell: ({ row }) => {
			const duration = formatDuration(row.original.created, row.original.resolved)
			if (!duration) {
				return null
			}
			return <span className="ps-2">{duration}</span>
		},
	},
]
