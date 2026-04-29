import type { CellContext, Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, copyToClipboard, formatMicroseconds, hourWithSeconds } from "@/lib/utils"
import {
	GlobeIcon,
	TimerIcon,
	WifiOffIcon,
	Trash2Icon,
	ArrowLeftRightIcon,
	MoreHorizontalIcon,
	ServerIcon,
	ClockIcon,
	NetworkIcon,
	RefreshCwIcon,
	PenBoxIcon,
	PauseCircleIcon,
	PlayCircleIcon,
	CopyIcon,
} from "lucide-react"
import { t } from "@lingui/core/macro"
import type { NetworkProbeRecord, SystemRecord } from "@/types"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Trans } from "@lingui/react/macro"
import { $allSystemsById, $longestSystemName } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { SystemStatus } from "@/lib/enums"
import { Checkbox } from "@/components/ui/checkbox"
import { useMemo } from "react"
import { formatBulkProbeLine } from "@/components/network-probes-table/probe-dialog"
import { Badge } from "../ui/badge"

const protocolColors: Record<string, string> = {
	icmp: "bg-blue-500/15 text-blue-600 dark:text-blue-400",
	tcp: "bg-purple-500/15 text-purple-600 dark:text-purple-400",
	http: "bg-green-500/15 text-green-700 dark:text-green-400",
}

const SYSTEM_STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

/**
 * A probe is considered muted if it's disabled or if its associated system is not up.
 */
const isMuted = (record: NetworkProbeRecord, systemRecord: SystemRecord | undefined) =>
	!record.enabled || systemRecord?.status !== SystemStatus.Up

export function getProbeColumns(
	longestName = "",
	longestTarget = "",
	{
		onEdit,
		onDelete,
		onSetEnabled,
	}: {
		onEdit?: (probe: NetworkProbeRecord) => void
		onDelete?: (probes: NetworkProbeRecord[]) => void | Promise<void>
		onSetEnabled?: (probes: NetworkProbeRecord[], enabled: boolean) => void | Promise<void>
	} = {}
): ColumnDef<NetworkProbeRecord>[] {
	return [
		{
			id: "select",
			header: ({ table }) => (
				<Checkbox
					className="ms-2"
					checked={table.getIsAllRowsSelected() || (table.getIsSomeRowsSelected() && "indeterminate")}
					onClick={(event) => event.stopPropagation()}
					onCheckedChange={(value) => table.toggleAllRowsSelected(!!value)}
					aria-label={t`Select all`}
				/>
			),
			cell: ({ row }) => (
				<Checkbox
					checked={row.getIsSelected()}
					onClick={(event) => event.stopPropagation()}
					onCheckedChange={(value) => row.toggleSelected(!!value)}
					aria-label={t`Select row`}
				/>
			),
			enableSorting: false,
			enableHiding: false,
			size: 44,
		},
		{
			id: "name",
			sortingFn: (a, b) => (a.original.name || a.original.target).localeCompare(b.original.name || b.original.target),
			accessorFn: (record) => record.name || record.target,
			header: ({ column }) => <HeaderButton column={column} name={t`Name`} Icon={NetworkIcon} />,
			cell: ({ row, getValue }) => {
				const probe = row.original
				const { status } = useStore($allSystemsById)[probe.system] || {}

				let color = "bg-green-500"
				if (!probe.enabled || status === SystemStatus.Paused) {
					color = "bg-primary/40"
				} else if (status === SystemStatus.Down || status === SystemStatus.Pending) {
					color = "bg-yellow-500"
				}
				return (
					<div className="ms-1.5 max-w-40 flex gap-2 items-center tabular-nums">
						<span className={cn("shrink-0 size-2 rounded-full", color)} />
						<div className="relative w-fit min-w-0 max-w-full">
							<span className="invisible block overflow-hidden whitespace-nowrap" aria-hidden="true">
								{longestName}
							</span>
							<span className="absolute inset-0 truncate">{getValue() as string}</span>
						</div>
					</div>
				)
			},
		},
		{
			id: "system",
			accessorFn: (record) => record.system,
			sortingFn: (a, b) => {
				const allSystems = $allSystemsById.get()
				const systemNameA = allSystems[a.original.system]?.name ?? ""
				const systemNameB = allSystems[b.original.system]?.name ?? ""
				const primary = systemNameA.localeCompare(systemNameB)
				if (primary !== 0) {
					return primary
				}
				return (a.original.name || a.original.target).localeCompare(b.original.name || b.original.target)
			},
			header: ({ column }) => <HeaderButton column={column} name={t`System`} Icon={ServerIcon} />,
			cell: ({ getValue }) => {
				const system = useStore($allSystemsById)[getValue() as string] as SystemRecord | undefined
				const longestSystemName = useStore($longestSystemName)
				const name = system?.name
				const status = system?.status as SystemStatus // undefined val is fine but makes lsp mad

				return useMemo(
					() => (
						<div className="ms-1.5 max-w-44 flex gap-2 items-center tabular-nums">
							<span className={cn("shrink-0 size-2 rounded-full", SYSTEM_STATUS_COLORS[status])} />
							<div className="relative w-fit min-w-0 max-w-full">
								<span className="invisible block whitespace-nowrap" aria-hidden="true">
									{longestSystemName}
								</span>
								<span className="absolute inset-0 truncate">{name}</span>
							</div>
						</div>
					),
					[status, name]
				)
			},
		},
		{
			id: "target",
			sortingFn: (a, b) => a.original.target.localeCompare(b.original.target),
			accessorFn: (record) => record.target,
			header: ({ column }) => <HeaderButton column={column} name={t`Target`} Icon={GlobeIcon} />,
			cell: ({ getValue }) => (
				<div className="ms-1.5 relative w-fit max-w-44 tabular-nums">
					<span className="invisible block whitespace-nowrap" aria-hidden="true">
						{longestTarget}
					</span>
					<span className="absolute inset-0 truncate">{getValue() as string}</span>
				</div>
			),
		},
		{
			id: "protocol",
			accessorFn: (record) => record.protocol,
			header: ({ column }) => <HeaderButton column={column} name={t`Protocol`} Icon={ArrowLeftRightIcon} />,
			cell: ({ getValue }) => {
				const protocol = getValue() as string
				return <Badge className={cn("uppercase", protocolColors[protocol])}>{protocol}</Badge>
			},
		},
		{
			id: "interval",
			accessorFn: (record) => record.interval,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Interval`} Icon={RefreshCwIcon} />,
			cell: ({ getValue }) => <span className="ms-1.5 tabular-nums">{getValue() as number}s</span>,
		},
		{
			id: "res",
			accessorFn: (record) => record.res,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Response`} Icon={TimerIcon} />,
			cell: responseTimeCell,
		},
		{
			id: "res1h",
			accessorFn: (record) => record.resAvg1h,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Avg 1h`} Icon={TimerIcon} />,
			cell: responseTimeCell,
		},
		{
			id: "max1h",
			accessorFn: (record) => record.resMax1h,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Max 1h`} Icon={TimerIcon} />,
			cell: responseTimeCell,
		},
		{
			id: "min1h",
			accessorFn: (record) => record.resMin1h,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Min 1h`} Icon={TimerIcon} />,
			cell: responseTimeCell,
		},
		{
			id: "loss",
			accessorFn: (record) => record.loss1h,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Loss 1h`} Icon={WifiOffIcon} />,
			cell: ({ row }) => {
				const { loss1h, res, system } = row.original
				const systemRecord = useStore($allSystemsById)[system]

				if (loss1h === undefined || (!res && !loss1h)) {
					return <span className="ms-1.5 text-muted-foreground">-</span>
				}

				const muted = isMuted(row.original, systemRecord)
				let color = "bg-green-500"
				if (muted) {
					color = "bg-muted-foreground/50"
				} else if (loss1h) {
					color = loss1h > 20 ? "bg-red-500" : "bg-yellow-500"
				}
				return (
					<span className="ms-1.5 tabular-nums flex gap-2 items-center">
						<span className={cn("shrink-0 size-2 rounded-full", color)} />
						{loss1h}%
					</span>
				)
			},
		},
		{
			id: "updated",
			invertSorting: true,
			accessorFn: (record) => record.updated,
			header: ({ column }) => <HeaderButton column={column} name={t`Updated`} Icon={ClockIcon} />,
			cell: ({ getValue }) => {
				const timestamp = getValue() as number
				if (!timestamp) {
					return <span className="ms-1.5 text-muted-foreground">-</span>
				}
				return <span className="ms-1.5 tabular-nums">{hourWithSeconds(timestamp)}</span>
			},
		},
		{
			id: "actions",
			enableSorting: false,
			enableHiding: false,
			header: () => null,
			size: 40,
			cell: ({ row, table }) => {
				const selectedRows = table.getSelectedRowModel().rows
				const actionRows =
					row.getIsSelected() && selectedRows.length > 1
						? selectedRows.map((selectedRow) => selectedRow.original)
						: [row.original]
				const isBulkAction = actionRows.length > 1
				const shouldPause = actionRows.some((probe) => probe.enabled)
				const bulkCopyContent = actionRows.map((probe) => formatBulkProbeLine(probe)).join("\n")
				return (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button variant="ghost" size="icon" className="size-10">
								<span className="sr-only">
									<Trans>Open menu</Trans>
								</span>
								<MoreHorizontalIcon className="w-5" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" onClick={(event) => event.stopPropagation()}>
							{!isBulkAction && (
								<DropdownMenuItem
									onClick={() => {
										onEdit?.(row.original)
									}}
								>
									<PenBoxIcon className="me-2.5 size-4" />
									<Trans>Edit</Trans>
								</DropdownMenuItem>
							)}
							<DropdownMenuItem
								onClick={() => {
									onSetEnabled?.(actionRows, !shouldPause)
								}}
							>
								{shouldPause ? (
									<>
										<PauseCircleIcon className="me-2.5 size-4" />
										<Trans>Pause</Trans>
									</>
								) : (
									<>
										<PlayCircleIcon className="me-2.5 size-4" />
										<Trans>Resume</Trans>
									</>
								)}
							</DropdownMenuItem>
							<DropdownMenuItem
								onClick={() => {
									copyToClipboard(bulkCopyContent)
								}}
							>
								<CopyIcon className="me-2.5 size-4" />
								<Trans>Bulk copy</Trans>
							</DropdownMenuItem>
							<DropdownMenuSeparator />
							<DropdownMenuItem
								onClick={() => {
									onDelete?.(actionRows)
								}}
							>
								<Trash2Icon className="me-2.5 size-4" />
								<Trans>Delete</Trans>
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				)
			},
		},
	]
}

const responseTimeThresholds = {
	http: { warning: 800_000, critical: 3_000_000 },
	tcp: { warning: 500_000, critical: 2_000_000 },
	icmp: { warning: 100_000, critical: 500_000 },
}

function responseTimeCell(cell: CellContext<NetworkProbeRecord, unknown>) {
	const probe = cell.row.original
	const systemRecord = useStore($allSystemsById)[probe.system]
	const responseTime = cell.getValue() as number | undefined

	if (!responseTime) {
		return <span className="ms-1.5 text-muted-foreground">-</span>
	}

	const muted = isMuted(probe, systemRecord)
	let color = "bg-green-500"
	if (muted) {
		color = "bg-muted-foreground/50"
	} else if (responseTime > responseTimeThresholds[probe.protocol].warning) {
		color = "bg-yellow-500"
	}
	if (!muted && responseTime > responseTimeThresholds[probe.protocol].critical) {
		color = "bg-red-500"
	}
	return (
		<span className="ms-1.5 tabular-nums flex gap-2 items-center">
			<span className={cn("shrink-0 size-2 rounded-full", color)} />
			{formatMicroseconds(responseTime)}
		</span>
	)
}

function HeaderButton({
	column,
	name,
	Icon,
}: {
	column: Column<NetworkProbeRecord>
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
			{Icon && <Icon className="size-4" />}
			{name}
		</Button>
	)
}
