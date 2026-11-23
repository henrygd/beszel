import type { CellContext, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { sortableHeader } from "@/components/ui/data-table-settings"
import { cn, copyToClipboard, decimalString, formatMicroseconds, hourWithSeconds } from "@/lib/utils"
import {
	GlobeIcon,
	TimerIcon,
	WifiOffIcon,
	Trash2Icon,
	ArrowLeftRightIcon,
	MoreHorizontalIcon,
	ServerIcon,
	ClockIcon,
	RefreshCwIcon,
	PenBoxIcon,
	PauseCircleIcon,
	PlayCircleIcon,
	CopyIcon,
	ShieldCheckIcon,
	ShieldOffIcon,
	Layers2Icon,
} from "lucide-react"
import { t } from "@lingui/core/macro"
import type { NetworkMonitorRecord, SystemRecord } from "@/types"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Plural, Trans } from "@lingui/react/macro"
import { $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { SystemStatus } from "@/lib/enums"
import { Checkbox } from "@/components/ui/checkbox"
import { useMemo } from "react"
import { formatBulkMonitorLine } from "@/components/network-monitors-table/monitor-dialog"
import { Badge } from "../ui/badge"
import {
	getCertDaysLeft,
	getCertExpiryLevel,
	getMonitorProtocolLabel,
	getMonitorTarget,
} from "@/lib/network-monitor-utils"

const certExpiryDotColors = { ok: "bg-green-500", warning: "bg-yellow-500", critical: "bg-red-500" }

const protocolColors: Record<string, string> = {
	icmp: "bg-blue-500/15! text-blue-600 dark:text-blue-400",
	tcp: "bg-purple-500/15! text-purple-600 dark:text-purple-400",
	http: "bg-teal-500/15! text-teal-700 dark:text-teal-400",
	https: "bg-green-500/15! text-green-700 dark:text-green-400",
	dns: "bg-amber-500/15! text-amber-600 dark:text-amber-400",
	// HTTP monitors that skip TLS certificate verification
	insecure: "bg-orange-500/15! text-orange-600 dark:text-orange-400",
}

const SYSTEM_STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

/**
 * A monitor is considered muted if it's disabled or if its associated system is not up.
 */
const isMuted = (record: NetworkMonitorRecord, systemRecord: SystemRecord | undefined) =>
	!record.enabled || systemRecord?.status !== SystemStatus.Up

export function getMonitorColumns({
	onEdit,
	onDelete,
	onSetEnabled,
}: {
	onEdit?: (monitor: NetworkMonitorRecord) => void
	onDelete?: (monitors: NetworkMonitorRecord[]) => void | Promise<void>
	onSetEnabled?: (monitors: NetworkMonitorRecord[], enabled: boolean) => void | Promise<void>
} = {}): ColumnDef<NetworkMonitorRecord>[] {
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
			id: "system",
			meta: { label: () => t`System`, Icon: ServerIcon },
			accessorFn: (record) => record.system,
			sortingFn: (a, b) => {
				const allSystems = $allSystemsById.get()
				const systemNameA = allSystems[a.original.system]?.name ?? ""
				const systemNameB = allSystems[b.original.system]?.name ?? ""
				const primary = systemNameA.localeCompare(systemNameB)
				if (primary !== 0) {
					return primary
				}
				return a.original.target.localeCompare(b.original.target)
			},
			header: sortableHeader,
			cell: ({ getValue }) => {
				const system = useStore($allSystemsById)[getValue() as string] as SystemRecord | undefined
				const name = system?.name
				const status = system?.status as SystemStatus // undefined val is fine but makes lsp mad

				return useMemo(
					() => (
						<div className="ms-1.5 max-w-44 flex gap-2 items-center tabular-nums">
							<span className={cn("shrink-0 size-2 rounded-full", SYSTEM_STATUS_COLORS[status])} />
							<span className="truncate">{name}</span>
						</div>
					),
					[status, name]
				)
			},
		},
		{
			id: "target",
			meta: { label: () => t`Target`, Icon: GlobeIcon },
			sortingFn: (a, b) =>
				(a.getValue("target") as string).localeCompare(b.getValue("target") as string, undefined, { numeric: true }),
			// The protocol column shows the IP version as a badge, so leave it out of the target.
			accessorFn: (record) => getMonitorTarget({ ...record, ipVersion: 0 }),
			header: sortableHeader,
			cell: ({ row, getValue }) => {
				const monitor = row.original
				const { status } = useStore($allSystemsById)[monitor.system] || {}

				let color = "bg-green-500"
				if (!monitor.enabled || status === SystemStatus.Paused) {
					color = "bg-primary/40"
				} else if (status === SystemStatus.Down || status === SystemStatus.Pending) {
					color = "bg-yellow-500"
				} else if (monitor.updated && !monitor.res) {
					color = "bg-red-500"
				}
				return (
					<div className="ms-1.5 max-w-64 flex gap-2 items-center tabular-nums">
						<span className={cn("shrink-0 size-2 rounded-full", color)} />
						<span className="truncate">{getValue() as string}</span>
					</div>
				)
			},
		},
		{
			id: "protocol",
			meta: { label: () => t`Protocol`, Icon: ArrowLeftRightIcon },
			accessorFn: (record) => getMonitorProtocolLabel(record),
			header: sortableHeader,
			cell: ({ row, getValue }) => {
				const protocol = getValue() as string
				if (row.original.skipTlsVerify) {
					return (
						<Badge className={cn("uppercase gap-1", protocolColors.insecure)}>
							{protocol}
							<ShieldOffIcon className="size-3" />
						</Badge>
					)
				}
				return <Badge className={cn("uppercase", protocolColors[protocol])}>{protocol}</Badge>
			},
		},
		{
			id: "ipVersion",
			meta: { label: () => t`IP Version`, Icon: Layers2Icon },
			accessorFn: (record) => record.ipVersion ?? 0,
			header: sortableHeader,
			cell: ({ getValue }) => {
				const ipVersion = getValue() as number
				return <Badge variant="secondary">{ipVersion ? `IPv${ipVersion}` : <Trans>Auto</Trans>}</Badge>
			},
		},
		{
			id: "interval",
			meta: { label: () => t`Interval`, Icon: RefreshCwIcon },
			accessorFn: (record) => record.interval,
			invertSorting: true,
			header: sortableHeader,
			cell: ({ getValue }) => <span className="ms-1.5 tabular-nums">{getValue() as number}s</span>,
		},
		{
			id: "res",
			meta: { label: () => t`Response`, Icon: TimerIcon },
			accessorFn: (record) => record.res,
			invertSorting: true,
			header: sortableHeader,
			cell: responseTimeCell,
		},
		{
			id: "res1h",
			meta: { label: () => t`Avg 1h`, Icon: TimerIcon },
			accessorFn: (record) => record.resAvg1h,
			invertSorting: true,
			header: sortableHeader,
			cell: responseTimeCell,
		},
		{
			id: "max1h",
			meta: { label: () => t`Max 1h`, Icon: TimerIcon },
			accessorFn: (record) => record.resMax1h,
			invertSorting: true,
			header: sortableHeader,
			cell: responseTimeCell,
		},
		{
			id: "min1h",
			meta: { label: () => t`Min 1h`, Icon: TimerIcon },
			accessorFn: (record) => record.resMin1h,
			invertSorting: true,
			header: sortableHeader,
			cell: responseTimeCell,
		},
		{
			id: "loss",
			meta: { label: () => t`Loss 1h`, Icon: WifiOffIcon },
			accessorFn: (record) => record.loss1h,
			invertSorting: true,
			header: sortableHeader,
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
						{loss1h === 100 ? loss1h : decimalString(loss1h, loss1h >= 10 ? 1 : 2)}%
					</span>
				)
			},
		},
		{
			id: "cert",
			meta: { label: () => t`Certificate`, Icon: ShieldCheckIcon },
			accessorFn: (record) => record.certInfo?.expires,
			header: sortableHeader,
			cell: ({ row }) => {
				const { certInfo, system } = row.original
				const systemRecord = useStore($allSystemsById)[system]

				if (!certInfo?.expires) {
					return <span className="ms-1.5 text-muted-foreground">-</span>
				}

				const daysLeft = getCertDaysLeft(certInfo)
				const color = isMuted(row.original, systemRecord)
					? "bg-muted-foreground/50"
					: certExpiryDotColors[getCertExpiryLevel(daysLeft)]
				return (
					<span className="ms-1.5 tabular-nums flex gap-2 items-center">
						<span className={cn("shrink-0 size-2 rounded-full", color)} />
						{daysLeft < 0 ? <Trans>Expired</Trans> : <Plural value={daysLeft} one="# day" other="# days" />}
					</span>
				)
			},
		},
		{
			id: "updated",
			meta: { label: () => t`Updated`, Icon: ClockIcon },
			invertSorting: true,
			accessorFn: (record) => record.updated,
			header: sortableHeader,
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
				const shouldPause = actionRows.some((monitor) => monitor.enabled)
				const bulkCopyContent = actionRows.map((monitor) => formatBulkMonitorLine(monitor)).join("\n")
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
								<Trans>Copy bulk config</Trans>
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
	dns: { warning: 150_000, critical: 800_000 },
}

function responseTimeCell(cell: CellContext<NetworkMonitorRecord, unknown>) {
	const monitor = cell.row.original
	const systemRecord = useStore($allSystemsById)[monitor.system]
	const responseTime = cell.getValue() as number | undefined

	if (!responseTime) {
		return <span className="ms-1.5 text-muted-foreground">-</span>
	}

	const muted = isMuted(monitor, systemRecord)
	let color = "bg-green-500"
	if (muted) {
		color = "bg-muted-foreground/50"
	} else if (responseTime > responseTimeThresholds[monitor.protocol].warning) {
		color = "bg-yellow-500"
	}
	if (!muted && responseTime > responseTimeThresholds[monitor.protocol].critical) {
		color = "bg-red-500"
	}
	return (
		<span className="ms-1.5 tabular-nums flex gap-2 items-center">
			<span className={cn("shrink-0 size-2 rounded-full", color)} />
			{formatMicroseconds(responseTime)}
		</span>
	)
}

