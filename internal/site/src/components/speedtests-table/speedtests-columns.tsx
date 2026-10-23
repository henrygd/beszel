import type { Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString, formatShortDate } from "@/lib/utils"
import {
	ArrowDownIcon,
	ArrowUpIcon,
	ClockArrowDownIcon,
	ClockArrowUpIcon,
	ClockIcon,
	EthernetPortIcon,
	ExternalLinkIcon,
	GaugeIcon,
	MoreHorizontalIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	RefreshCwIcon,
	ServerIcon,
	TimerIcon,
	Trash2Icon,
	WifiOffIcon,
	GlobeIcon,
	RadioTowerIcon,
	MapPinIcon,
} from "lucide-react"
import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import type { SpeedtestRecord, SystemRecord } from "@/types"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { SystemStatus } from "@/lib/enums"
import { Checkbox } from "@/components/ui/checkbox"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { formatSpeedtestInterval, getSpeedtestServerLabel } from "@/lib/speedtest-utils"
import { formatBandwidth } from "@/components/routes/system/charts/speedtest-charts"

const SYSTEM_STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

/** Status of a speedtest's latest run, shown as a colored dot. */
export function getSpeedtestStatusColor(speedtest: SpeedtestRecord, system: SystemRecord | undefined) {
	if (!speedtest.enabled || system?.status === SystemStatus.Paused) return "bg-primary/40"
	if (speedtest.error) return "bg-red-500"
	if (!speedtest.last_run || system?.status !== SystemStatus.Up) return "bg-yellow-500"
	return "bg-green-500"
}

/** Whether a speedtest can run now: it must be active and its system connected. */
export function canRunSpeedtest(speedtest: SpeedtestRecord, system: SystemRecord | undefined) {
	return speedtest.enabled && system?.status === SystemStatus.Up
}

/** Placeholder for values that haven't been measured yet. */
const empty = <span className="ms-1.5 text-muted-foreground">-</span>

export function getSpeedtestColumns({
	onEdit,
	onDelete,
	onSetEnabled,
	onRunNow,
}: {
	onEdit?: (speedtest: SpeedtestRecord) => void
	onDelete?: (speedtests: SpeedtestRecord[]) => void | Promise<void>
	onSetEnabled?: (speedtests: SpeedtestRecord[], enabled: boolean) => void | Promise<void>
	onRunNow?: (speedtests: SpeedtestRecord[]) => void | Promise<void>
} = {}): ColumnDef<SpeedtestRecord>[] {
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
			meta: { label: t`System` },
			accessorFn: (record) => $allSystemsById.get()[record.system]?.name ?? "",
			header: ({ column }) => <HeaderButton column={column} name={t`System`} Icon={ServerIcon} />,
			cell: ({ row }) => {
				const system = useStore($allSystemsById)[row.original.system] as SystemRecord | undefined
				const status = system?.status as SystemStatus
				return (
					<div className="ms-1.5 max-w-44 flex gap-2 items-center">
						<span className={cn("shrink-0 size-2 rounded-full", SYSTEM_STATUS_COLORS[status])} />
						<span className="truncate">{system?.name}</span>
					</div>
				)
			},
		},
		{
			id: "server",
			meta: { label: t`Server` },
			// Only the server name; the location is in the sheet and the table filter.
			accessorFn: (record) => record.server_name || getSpeedtestServerLabel(record) || t`Automatic`,
			header: ({ column }) => <HeaderButton column={column} name={t`Server`} Icon={GlobeIcon} />,
			cell: ({ row, getValue }) => {
				const speedtest = row.original
				const system = useStore($allSystemsById)[speedtest.system]
				const dot = <span className={cn("shrink-0 size-2 rounded-full", getSpeedtestStatusColor(speedtest, system))} />
				return (
					<div className="ms-1.5 max-w-72 flex gap-2 items-center">
						{speedtest.error ? (
							<Tooltip>
								<TooltipTrigger asChild>{dot}</TooltipTrigger>
								<TooltipContent className="max-w-80 text-wrap">{speedtest.error}</TooltipContent>
							</Tooltip>
						) : (
							dot
						)}
						<span className="truncate">{getValue() as string}</span>
						{/* Automatic speedtests show the server of the latest run. */}
						{!speedtest.server_id && speedtest.server_name && (
							<Badge variant="outline" className="shrink-0 font-normal text-muted-foreground">
								<Trans>Auto</Trans>
							</Badge>
						)}
					</div>
				)
			},
		},
		{
			id: "interface",
			meta: { label: t`Interface` },
			accessorFn: (record) => record.interface,
			header: ({ column }) => <HeaderButton column={column} name={t`Interface`} Icon={EthernetPortIcon} />,
			cell: ({ getValue }) => {
				const value = getValue() as string
				// Empty uses the default route.
				if (!value) {
					return (
						<span className="ms-1.5">
							<Trans>Auto</Trans>
						</span>
					)
				}
				return <span className="ms-1.5 block max-w-40 truncate">{value}</span>
			},
		},
		{
			id: "location",
			meta: { label: t`Location` },
			accessorFn: (record) => record.server_location,
			header: ({ column }) => <HeaderButton column={column} name={t`Location`} Icon={MapPinIcon} />,
			cell: textCell,
		},
		{
			id: "interval",
			meta: { label: t`Interval` },
			accessorFn: (record) => record.interval,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Interval`} Icon={RefreshCwIcon} />,
			cell: ({ getValue }) => (
				<span className="ms-1.5 tabular-nums">{formatSpeedtestInterval(getValue() as number)}</span>
			),
		},
		{
			id: "download",
			meta: { label: t`Download` },
			accessorFn: (record) => record.download,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Download`} Icon={ArrowDownIcon} />,
			cell: bandwidthCell,
		},
		{
			id: "upload",
			meta: { label: t`Upload` },
			accessorFn: (record) => record.upload,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Upload`} Icon={ArrowUpIcon} />,
			cell: bandwidthCell,
		},
		{
			id: "ping",
			meta: { label: t`Ping` },
			accessorFn: (record) => record.ping,
			header: ({ column }) => <HeaderButton column={column} name={t`Ping`} Icon={TimerIcon} />,
			cell: latencyCell,
		},
		{
			id: "jitter",
			meta: { label: t`Jitter` },
			accessorFn: (record) => record.jitter,
			header: ({ column }) => <HeaderButton column={column} name={t`Jitter`} Icon={TimerIcon} />,
			cell: latencyCell,
		},
		{
			id: "loss",
			meta: { label: t({ message: "Loss", context: "Packet loss" }) },
			accessorFn: (record) => record.loss,
			header: ({ column }) => (
				<HeaderButton column={column} name={t({ message: "Loss", context: "Packet loss" })} Icon={WifiOffIcon} />
			),
			cell: ({ row }) => {
				const { loss, download } = row.original
				// -1 means the server doesn't report packet loss.
				if (!download || loss < 0) return empty
				return <span className="ms-1.5 tabular-nums">{decimalString(loss, loss >= 10 ? 1 : 2)}%</span>
			},
		},
		{
			id: "download_latency",
			meta: { label: t`Download latency` },
			accessorFn: (record) => record.download_latency,
			header: ({ column }) => <HeaderButton column={column} name={t`Download latency`} Icon={ClockArrowDownIcon} />,
			cell: latencyCell,
		},
		{
			id: "download_jitter",
			meta: { label: t`Download jitter` },
			accessorFn: (record) => record.download_jitter,
			header: ({ column }) => <HeaderButton column={column} name={t`Download jitter`} Icon={TimerIcon} />,
			cell: latencyCell,
		},
		{
			id: "upload_latency",
			meta: { label: t`Upload latency` },
			accessorFn: (record) => record.upload_latency,
			header: ({ column }) => <HeaderButton column={column} name={t`Upload latency`} Icon={ClockArrowUpIcon} />,
			cell: latencyCell,
		},
		{
			id: "upload_jitter",
			meta: { label: t`Upload jitter` },
			accessorFn: (record) => record.upload_jitter,
			header: ({ column }) => <HeaderButton column={column} name={t`Upload jitter`} Icon={TimerIcon} />,
			cell: latencyCell,
		},
		{
			id: "isp",
			meta: { label: t`ISP` },
			accessorFn: (record) => record.isp,
			header: ({ column }) => <HeaderButton column={column} name={t`ISP`} Icon={RadioTowerIcon} />,
			cell: textCell,
		},
		{
			id: "last_run",
			meta: { label: t`Last run` },
			accessorFn: (record) => record.last_run,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Last run`} Icon={ClockIcon} />,
			cell: ({ getValue }) => {
				const timestamp = getValue() as number
				if (!timestamp) return empty
				return <span className="ms-1.5 tabular-nums">{formatShortDate(new Date(timestamp).toISOString())}</span>
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
				const shouldPause = actionRows.some((speedtest) => speedtest.enabled)
				const allSystems = useStore($allSystemsById)
				const runnableRows = actionRows.filter((speedtest) => canRunSpeedtest(speedtest, allSystems[speedtest.system]))
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
							<DropdownMenuItem disabled={!runnableRows.length} onClick={() => onRunNow?.(runnableRows)}>
								<GaugeIcon className="me-2.5 size-4" />
								<Trans>Run now</Trans>
							</DropdownMenuItem>
							{!isBulkAction && (
								<DropdownMenuItem onClick={() => onEdit?.(row.original)}>
									<PenBoxIcon className="me-2.5 size-4" />
									<Trans>Edit</Trans>
								</DropdownMenuItem>
							)}
							<DropdownMenuItem onClick={() => onSetEnabled?.(actionRows, !shouldPause)}>
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
							{!isBulkAction && row.original.url && (
								<DropdownMenuItem asChild>
									<a href={row.original.url} target="_blank" rel="noopener noreferrer">
										<ExternalLinkIcon className="me-2.5 size-4" />
										<Trans>View result</Trans>
									</a>
								</DropdownMenuItem>
							)}
							<DropdownMenuSeparator />
							<DropdownMenuItem onClick={() => onDelete?.(actionRows)}>
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

/** Columns hidden until the user enables them in the view menu. */
export const DEFAULT_HIDDEN_SPEEDTEST_COLUMNS = Object.fromEntries(
	["jitter", "isp", "location", "download_latency", "download_jitter", "upload_latency", "upload_jitter"].map((id) => [
		id,
		false,
	])
)

function textCell({ getValue }: { getValue: () => unknown }) {
	const value = getValue() as string
	if (!value) return empty
	return <span className="ms-1.5 block max-w-56 truncate">{value}</span>
}

function bandwidthCell({ getValue }: { getValue: () => unknown }) {
	const value = getValue() as number
	if (!value) return empty
	return <span className="ms-1.5 tabular-nums">{formatBandwidth(value)}</span>
}

function latencyCell({ row, getValue }: { row: { original: SpeedtestRecord }; getValue: () => unknown }) {
	const value = getValue() as number
	if (!row.original.download) return empty
	return <span className="ms-1.5 tabular-nums">{decimalString(value, value >= 100 ? 0 : 1)} ms</span>
}

function HeaderButton({
	column,
	name,
	Icon,
}: {
	column: Column<SpeedtestRecord>
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
