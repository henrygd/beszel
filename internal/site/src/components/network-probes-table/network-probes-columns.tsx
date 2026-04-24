import type { CellContext, Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString, hourWithSeconds } from "@/lib/utils"
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
import { pb } from "@/lib/api"
import { toast } from "@/components/ui/use-toast"
import { $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { SystemStatus } from "@/lib/enums"
import { Checkbox } from "@/components/ui/checkbox"
import { useMemo } from "react"

const protocolColors: Record<string, string> = {
	icmp: "bg-blue-500/15 text-blue-400",
	tcp: "bg-purple-500/15 text-purple-400",
	http: "bg-green-500/15 text-green-400",
}

async function deleteProbe(id: string) {
	try {
		await pb.collection("network_probes").delete(id)
	} catch (err: unknown) {
		toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
	}
}

async function setProbeEnabled(id: string, enabled: boolean) {
	try {
		await pb.collection("network_probes").update(id, { enabled })
	} catch (err: unknown) {
		toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
	}
}

const STATUS_COLORS = {
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
	longestName = 0,
	longestTarget = 0,
	onEdit?: (probe: NetworkProbeRecord) => void
): ColumnDef<NetworkProbeRecord>[] {
	return [
		{
			id: "select",
			header: ({ table }) => (
				<Checkbox
					className="ms-2"
					checked={table.getIsAllRowsSelected() || (table.getIsSomeRowsSelected() && "indeterminate")}
					onCheckedChange={(value) => table.toggleAllRowsSelected(!!value)}
					aria-label={t`Select all`}
				/>
			),
			cell: ({ row }) => (
				<Checkbox
					checked={row.getIsSelected()}
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
			cell: ({ getValue }) => (
				<div className="ms-1.5 max-w-40 block truncate tabular-nums" style={{ width: `${longestName / 1.05}ch` }}>
					{getValue() as string}
				</div>
			),
		},
		{
			id: "system",
			accessorFn: (record) => record.system,
			sortingFn: (a, b) => {
				const allSystems = $allSystemsById.get()
				const systemNameA = allSystems[a.original.system]?.name ?? ""
				const systemNameB = allSystems[b.original.system]?.name ?? ""
				return systemNameA.localeCompare(systemNameB)
			},
			header: ({ column }) => <HeaderButton column={column} name={t`System`} Icon={ServerIcon} />,
			cell: ({ getValue }) => {
				const system = useStore($allSystemsById)[getValue() as string] as SystemRecord | undefined
				const name = system?.name
				const status = system?.status as SystemStatus // undefined val is fine but makes lsp mad

				return useMemo(
					() => (
						<span className="ms-1.5 xl:w-20 truncate flex items-center gap-2">
							<span className={cn("shrink-0 size-2 rounded-full", STATUS_COLORS[status])} />
							{name}
						</span>
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
				<div className="ms-1.5 tabular-nums block truncate max-w-44" style={{ width: `${longestTarget / 1.05}ch` }}>
					{getValue() as string}
				</div>
			),
		},
		{
			id: "protocol",
			accessorFn: (record) => record.protocol,
			header: ({ column }) => <HeaderButton column={column} name={t`Protocol`} Icon={ArrowLeftRightIcon} />,
			cell: ({ getValue }) => {
				const protocol = getValue() as string
				return (
					<span className={cn("ms-1.5 px-2 py-0.5 rounded text-xs font-medium uppercase", protocolColors[protocol])}>
						{protocol}
					</span>
				)
			},
		},
		{
			id: "interval",
			accessorFn: (record) => record.interval,
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
				return <span className="ms-1.5 tabular-nums">{hourWithSeconds(new Date(timestamp).toISOString())}</span>
			},
		},
		{
			id: "actions",
			enableSorting: false,
			enableHiding: false,
			header: () => null,
			size: 40,
			cell: ({ row }) => {
				const { enabled } = row.original
				return (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								className="size-10"
								onClick={(event) => event.stopPropagation()}
								onMouseDown={(event) => event.stopPropagation()}
							>
								<span className="sr-only">
									<Trans>Open menu</Trans>
								</span>
								<MoreHorizontalIcon className="w-5" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" onClick={(event) => event.stopPropagation()}>
							<DropdownMenuItem onClick={() => onEdit?.(row.original)}>
								<PenBoxIcon className="me-2.5 size-4" />
								<Trans>Edit</Trans>
							</DropdownMenuItem>
							<DropdownMenuItem onClick={() => setProbeEnabled(row.original.id, !enabled)}>
								{enabled ? (
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
							<DropdownMenuSeparator />
							<DropdownMenuItem
								onClick={(event) => {
									event.stopPropagation()
									deleteProbe(row.original.id)
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
	} else if (responseTime > 200) {
		color = "bg-yellow-500"
	}
	if (!muted && responseTime > 2000) {
		color = "bg-red-500"
	}
	return (
		<span className="ms-1.5 tabular-nums flex gap-2 items-center">
			<span className={cn("shrink-0 size-2 rounded-full", color)} />
			{decimalString(responseTime, responseTime < 100 ? 2 : 1).toLocaleString()}ms
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
