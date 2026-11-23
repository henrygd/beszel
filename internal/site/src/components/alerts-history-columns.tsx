import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import type { ColumnDef } from "@tanstack/react-table"
import { Badge } from "@/components/ui/badge"
import { sortableHeader } from "@/components/ui/data-table-settings"
import { alertInfo } from "@/lib/alerts"
import { cn, formatDuration, formatShortDate, toFixedFloat } from "@/lib/utils"
import type { AlertsHistoryRecord } from "@/types"

export const alertsHistoryColumns: ColumnDef<AlertsHistoryRecord>[] = [
	{
		accessorKey: "system",
		meta: { label: () => t`System` },
		enableSorting: true,
		header: sortableHeader,
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
		meta: { label: () => t`Name` },
		enableHiding: false,
		accessorFn: (record) => {
			const name = record.name
			const info = alertInfo[name]
			const label = info?.name().replace("cpu", "CPU") || name
			return record.monitor_name ? `${label}: ${record.monitor_name}` : label
		},
		header: sortableHeader,
		cell: ({ getValue, row }) => {
			const name = getValue() as string
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
		meta: { label: () => t`Value` },
		enableSorting: false,
		header: sortableHeader,
		cell({ row, getValue }) {
			const name = row.original.name
			const info = alertInfo[name]
			if (info?.triggeredDesc) {
				return <span className="ps-2">{info.triggeredDesc()}</span>
			}
			if (name === "Status") {
				return <span className="ps-2">{t`Down`}</span>
			}
			const value = getValue() as number
			if (name === "NetworkMonitorCert") {
				return (
					<span className="tabular-nums ps-2.5">
						{value < 0 ? <Trans>Expired</Trans> : <Plural value={value} one="# day" other="# days" />}
					</span>
				)
			}
			const unit = info?.unit
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
		meta: { label: () => t({ message: "State", comment: "Context: alert state (active or resolved)" }) },
		enableSorting: true,
		sortingFn: (rowA, rowB) => (rowA.original.resolved ? 1 : 0) - (rowB.original.resolved ? 1 : 0),
		header: sortableHeader,
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
		meta: { label: () => t({ message: "Created", comment: "Context: date created" }) },
		accessorFn: (record) => formatShortDate(record.created),
		enableSorting: true,
		invertSorting: true,
		header: sortableHeader,
		cell: ({ getValue, row }) => (
			<span className="ps-1 tabular-nums tracking-tight" title={`${row.original.created} UTC`}>
				{getValue() as string}
			</span>
		),
	},
	{
		accessorKey: "resolved",
		meta: { label: () => t`Resolved` },
		enableSorting: true,
		invertSorting: true,
		header: sortableHeader,
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
		meta: { label: () => t`Duration` },
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
		header: sortableHeader,
		cell: ({ row }) => {
			const duration = formatDuration(row.original.created, row.original.resolved)
			if (!duration) {
				return null
			}
			return <span className="ps-2">{duration}</span>
		},
	},
]
