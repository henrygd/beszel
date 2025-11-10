import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import type { CellContext, ColumnDef, HeaderContext } from "@tanstack/react-table"
import type { ClassValue } from "clsx"
import type { SystemRecord, SystemStats, SystemStatsRecord, SystemdService, SystemdStatsRecord  } from "@/types"
import {
	ArrowUpDownIcon,
	ChevronRightSquareIcon,
	CheckIcon,
	CopyIcon,
	CpuIcon,
	HardDriveIcon,
	ListChecks,
	MemoryStickIcon,
	MoreHorizontalIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	ServerIcon,
	Trash2Icon,
	WifiIcon,
} from "lucide-react"
import { memo, useMemo, useRef, useState } from "react"
import { isReadOnlyUser, pb } from "@/lib/api"
import { ConnectionType, connectionTypeLabels, MeterState, SystemStatus } from "@/lib/enums"
import { $longestSystemNameLen, $userSettings } from "@/lib/stores"
import {
	cn,
	copyToClipboard,
	decimalString,
	formatBytes,
	formatTemperature,
	getMeterState,
	parseSemVer,
} from "@/lib/utils"
import { SystemDialog } from "../add-system"
import AlertButton from "../alerts/alert-button"
import { $router, Link } from "../router"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "../ui/alert-dialog"
import { Button, buttonVariants } from "../ui/button"
import { Dialog } from "../ui/dialog"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "../ui/dropdown-menu"
import { EthernetIcon, GpuIcon, HourglassIcon, ThermometerIcon, WebSocketIcon } from "../ui/icons"

const STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

/**
 * @param viewMode - "table" or "grid"
 * @returns - Column definitions for the systems table
 */
export default function SystemsTableColumns(viewMode: "table" | "grid"): ColumnDef<SystemRecord>[] {
	return [
		{
			// size: 200,
			size: 100,
			minSize: 0,
			accessorKey: "name",
			id: "system",
			name: () => t`System`,
			filterFn: (() => {
				let filterInput = ""
				let filterInputLower = ""
				const nameCache = new Map<string, string>()
				const statusTranslations = {
					[SystemStatus.Up]: t`Up`.toLowerCase(),
					[SystemStatus.Down]: t`Down`.toLowerCase(),
					[SystemStatus.Paused]: t`Paused`.toLowerCase(),
				} as const

				// match filter value against name or translated status
				return (row, _, newFilterInput) => {
					const { name, status } = row.original
					if (newFilterInput !== filterInput) {
						filterInput = newFilterInput
						filterInputLower = newFilterInput.toLowerCase()
					}
					let nameLower = nameCache.get(name)
					if (nameLower === undefined) {
						nameLower = name.toLowerCase()
						nameCache.set(name, nameLower)
					}
					if (nameLower.includes(filterInputLower)) {
						return true
					}
					const statusLower = statusTranslations[status as keyof typeof statusTranslations]
					return statusLower?.includes(filterInputLower) || false
				}
			})(),
			enableHiding: false,
			invertSorting: false,
			Icon: ServerIcon,
			cell: (info) => {
				const { name } = info.row.original
				const longestName = useStore($longestSystemNameLen)
				return (
					<>
						<span className="flex gap-2 items-center font-medium text-sm text-nowrap md:ps-1">
							<IndicatorDot system={info.row.original} />
							{/* NOTE: change to 1 ch if switching to monospace font */}
							<span className="truncate" style={{ width: `${longestName / 1.1}ch` }}>
								{name}
							</span>
						</span>
						<Link
							href={getPagePath($router, "system", { name })}
							className="inset-0 absolute size-full"
							aria-label={name}
						></Link>
					</>
				)
			},
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.cpu,
			id: "cpu",
			name: () => t`CPU`,
			cell: TableCellWithMeter,
			Icon: CpuIcon,
			header: sortableHeader,
		},
		{
			// accessorKey: "info.mp",
			accessorFn: ({ info }) => info.mp,
			id: "memory",
			name: () => t`Memory`,
			cell: TableCellWithMeter,
			Icon: MemoryStickIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.dp,
			id: "disk",
			name: () => t`Disk`,
			cell: TableCellWithMeter,
			Icon: HardDriveIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.g,
			id: "gpu",
			name: () => "GPU",
			cell: TableCellWithMeter,
			Icon: GpuIcon,
			header: sortableHeader,
		},
		{
			id: "loadAverage",
			accessorFn: ({ info }) => {
				const sum = info.la?.reduce((acc, curr) => acc + curr, 0)
				// TODO: remove this in future release in favor of la array
				if (!sum) {
					return (info.l1 ?? 0) + (info.l5 ?? 0) + (info.l15 ?? 0)
				}
				return sum
			},
			name: () => t({ message: "Load Avg", comment: "Short label for load average" }),
			size: 0,
			Icon: HourglassIcon,
			header: sortableHeader,
			cell(info: CellContext<SystemRecord, unknown>) {
				const { info: sysInfo, status } = info.row.original
				// agent version
				const { minor, patch } = parseSemVer(sysInfo.v)
				let loadAverages = sysInfo.la

				// use legacy load averages if agent version is less than 12.1.0
				if (!loadAverages || (minor === 12 && patch < 1)) {
					loadAverages = [sysInfo.l1 ?? 0, sysInfo.l5 ?? 0, sysInfo.l15 ?? 0]
				}

				const max = Math.max(...loadAverages)
				if (max === 0 && (status === SystemStatus.Paused || minor < 12)) {
					return null
				}

				const normalizedLoad = max / (sysInfo.t ?? 1)
				const threshold = getMeterState(normalizedLoad * 100)

				return (
					<div className="flex items-center gap-[.35em] w-full tabular-nums tracking-tight">
						<span
							className={cn("inline-block size-2 rounded-full me-0.5", {
								[STATUS_COLORS[SystemStatus.Up]]: threshold === MeterState.Good,
								[STATUS_COLORS[SystemStatus.Pending]]: threshold === MeterState.Warn,
								[STATUS_COLORS[SystemStatus.Down]]: threshold === MeterState.Crit,
								[STATUS_COLORS[SystemStatus.Paused]]: status !== SystemStatus.Up,
							})}
						/>
						{loadAverages?.map((la, i) => (
							<span key={i}>{decimalString(la, la >= 10 ? 1 : 2)}</span>
						))}
					</div>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.bb || (info.b || 0) * 1024 * 1024,
			id: "net",
			name: () => t`Net`,
			size: 0,
			Icon: EthernetIcon,
			header: sortableHeader,
			cell(info) {
				const sys = info.row.original
				const userSettings = useStore($userSettings, { keys: ["unitNet"] })
				if (sys.status === SystemStatus.Paused) {
					return null
				}
				const { value, unit } = formatBytes(info.getValue() as number, true, userSettings.unitNet, false)
				return (
					<span className="tabular-nums whitespace-nowrap">
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.dt,
			id: "temp",
			name: () => t({ message: "Temp", comment: "Temperature label in systems table" }),
			size: 50,
			hideSort: true,
			Icon: ThermometerIcon,
			header: sortableHeader,
			cell(info) {
				const val = info.getValue() as number
				const userSettings = useStore($userSettings, { keys: ["unitTemp"] })
				if (!val) {
					return null
				}
				const { value, unit } = formatTemperature(val, userSettings.unitTemp)
				return (
					<span className={cn("tabular-nums whitespace-nowrap", viewMode === "table" && "ps-0.5")}>
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.v,
			id: "agent",
			name: () => t`Agent`,
			// invertSorting: true,
			size: 50,
			Icon: WifiIcon,
			hideSort: true,
			header: sortableHeader,
			cell(info) {
				const version = info.getValue() as string
				if (!version) {
					return null
				}
				const system = info.row.original
				const color = {
					"text-green-500": version === globalThis.BESZEL.HUB_VERSION,
					"text-yellow-500": version !== globalThis.BESZEL.HUB_VERSION,
					"text-red-500": system.status !== SystemStatus.Up,
				}
				return (
					<Link
						href={getPagePath($router, "system", { name: system.name })}
						className={cn(
							"flex gap-1.5 items-center md:pe-5 tabular-nums relative z-10",
							viewMode === "table" && "ps-0.5"
						)}
						tabIndex={-1}
						title={connectionTypeLabels[system.info.ct as ConnectionType]}
						role="none"
					>
						{system.info.ct === ConnectionType.WebSocket && (
							<WebSocketIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{system.info.ct === ConnectionType.SSH && (
							<ChevronRightSquareIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{!system.info.ct && <IndicatorDot system={system} className={cn(color, "bg-current mx-0.5")} />}
						<span className="truncate max-w-14">{info.getValue() as string}</span>
					</Link>
				)
			},
		},
		{
			id: "systemd",
			name: () => t`Services`,
			size: 50,
			Icon: ListChecks,
			hideSort: true,
			header: sortableHeader,
			cell: ({ row }) => <SystemdCell systemId={row.original.id} />,
		},
		{
			id: "actions",
			// @ts-expect-error
			name: () => t({ message: "Actions", comment: "Table column" }),
			size: 50,
			cell: ({ row }) => (
				<div className="relative z-10 flex justify-end items-center gap-1 -ms-3">
					<AlertButton system={row.original} />
					<ActionsButton system={row.original} />
				</div>
			),
		},
	] as ColumnDef<SystemRecord>[]
}

function sortableHeader(context: HeaderContext<SystemRecord, unknown>) {
	const { column } = context
	// @ts-expect-error
	const { Icon, hideSort, name }: { Icon: React.ElementType; name: () => string; hideSort: boolean } = column.columnDef
	const isSorted = column.getIsSorted()
	return (
		<Button
			variant="ghost"
			className={cn("h-9 px-3 flex duration-50", isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90")}
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{Icon && <Icon className="me-2 size-4" />}
			{name()}
			{hideSort || <ArrowUpDownIcon className="ms-2 size-4" />}
		</Button>
	)
}

function TableCellWithMeter(info: CellContext<SystemRecord, unknown>) {
	const val = Number(info.getValue()) || 0
	const threshold = getMeterState(val)
	const meterClass = cn(
		"h-full",
		(info.row.original.status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
	)
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight w-full">
			<span className="min-w-8 shrink-0">{decimalString(val, val >= 10 ? 1 : 2)}%</span>
			<span className="flex-1 min-w-8 grid bg-muted h-[1em] rounded-sm overflow-hidden">
				<span className={meterClass} style={{ width: `${val}%` }}></span>
			</span>
		</div>
	)
}

export function IndicatorDot({ system, className }: { system: SystemRecord; className?: ClassValue }) {
	className ||= STATUS_COLORS[system.status as keyof typeof STATUS_COLORS] || ""
	return (
		<span
			className={cn("shrink-0 size-2 rounded-full", className)}
			// style={{ marginBottom: "-1px" }}
		/>
	)
}

const SystemdCell = ({ systemId }: { systemId: string }) => {
	const [stats, setStats] = useState<SystemdService[] | null>(null);

	useEffect(() => {
		const fetchStats = async () => {
			try {
				const record = await pb.collection("systemd_stats").getFirstListItem<SystemdStatsRecord>(`system="${systemId}"`, {
					sort: "-created",
				});
				setStats(record.stats);
			} catch (error) {
				// Handle case where no stats are found
				setStats(null);
			}
		};

		fetchStats();
	}, [systemId]);

	if (!stats) {
		return <span className="text-muted-foreground">-</span>;
	}

	const failed = stats.filter(s => s.s === 'failed').length;

	if (failed > 0) {
		return (
			<div className="tabular-nums text-red-500">
				{failed}
			</div>
		);
	}

	return (
		<div className="text-green-500 flex items-center justify-center">
			<CheckIcon className="size-4" />
		</div>
	);
};

export const ActionsButton = memo(({ system }: { system: SystemRecord }) => {
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [editOpen, setEditOpen] = useState(false)
	const editOpened = useRef(false)
	const { t } = useLingui()
	const { id, status, host, name } = system

	return useMemo(() => {
		return (
			<>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="ghost" size={"icon"}>
							<span className="sr-only">
								<Trans>Open menu</Trans>
							</span>
							<MoreHorizontalIcon className="w-5" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						{!isReadOnlyUser() && (
							<DropdownMenuItem
								onSelect={() => {
									editOpened.current = true
									setEditOpen(true)
								}}
							>
								<PenBoxIcon className="me-2.5 size-4" />
								<Trans>Edit</Trans>
							</DropdownMenuItem>
						)}
						<DropdownMenuItem
							className={cn(isReadOnlyUser() && "hidden")}
							onClick={() => {
								pb.collection("systems").update(id, {
									status: status === SystemStatus.Paused ? SystemStatus.Pending : SystemStatus.Paused,
								})
							}}
						>
							{status === SystemStatus.Paused ? (
								<>
									<PlayCircleIcon className="me-2.5 size-4" />
									<Trans>Resume</Trans>
								</>
							) : (
								<>
									<PauseCircleIcon className="me-2.5 size-4" />
									<Trans>Pause</Trans>
								</>
							)}
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(name)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy name</Trans>
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(host)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy host</Trans>
						</DropdownMenuItem>
						<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
						<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")} onSelect={() => setDeleteOpen(true)}>
							<Trash2Icon className="me-2.5 size-4" />
							<Trans>Delete</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				{/* edit dialog */}
				<Dialog open={editOpen} onOpenChange={setEditOpen}>
					{editOpened.current && <SystemDialog system={system} setOpen={setEditOpen} />}
				</Dialog>
				{/* deletion dialog */}
				<AlertDialog open={deleteOpen} onOpenChange={(open) => setDeleteOpen(open)}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>
								<Trans>Are you sure you want to delete {name}?</Trans>
							</AlertDialogTitle>
							<AlertDialogDescription>
								<Trans>
									This action cannot be undone. This will permanently delete all current records for {name} from the
									database.
								</Trans>
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>
								<Trans>Cancel</Trans>
							</AlertDialogCancel>
							<AlertDialogAction
								className={cn(buttonVariants({ variant: "destructive" }))}
								onClick={() => pb.collection("systems").delete(id)}
							>
								<Trans>Continue</Trans>
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</>
		)
	}, [id, status, host, name, t, deleteOpen, editOpen])
})
