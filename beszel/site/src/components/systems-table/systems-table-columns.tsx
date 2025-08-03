import { SystemRecord } from "@/types"
import { CellContext, ColumnDef, HeaderContext } from "@tanstack/react-table"
import { ClassValue } from "clsx"
import {
	ArrowUpDownIcon,
	CopyIcon,
	CpuIcon,
	HardDriveIcon,
	MemoryStickIcon,
	MoreHorizontalIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	ServerIcon,
	Trash2Icon,
	WifiIcon,
} from "lucide-react"
import { Button } from "../ui/button"
import {
	cn,
	copyToClipboard,
	decimalString,
	formatBytes,
	formatTemperature,
	getMeterState,
	isReadOnlyUser,
	parseSemVer,
} from "@/lib/utils"
import { EthernetIcon, GpuIcon, HourglassIcon, ThermometerIcon } from "../ui/icons"
import { useStore } from "@nanostores/react"
import { $userSettings, pb } from "@/lib/stores"
import { Trans, useLingui } from "@lingui/react/macro"
import { useMemo, useRef, useState } from "react"
import { memo } from "react"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "../ui/dropdown-menu"
import AlertButton from "../alerts/alert-button"
import { Dialog } from "../ui/dialog"
import { SystemDialog } from "../add-system"
import { AlertDialog } from "../ui/alert-dialog"
import {
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "../ui/alert-dialog"
import { buttonVariants } from "../ui/button"
import { t } from "@lingui/core/macro"
import { MeterState } from "@/lib/enums"

/**
 * @param viewMode - "table" or "grid"
 * @returns - Column definitions for the systems table
 */
export default function SystemsTableColumns(viewMode: "table" | "grid"): ColumnDef<SystemRecord>[] {
	const statusTranslations = {
		up: () => t`Up`.toLowerCase(),
		down: () => t`Down`.toLowerCase(),
		paused: () => t`Paused`.toLowerCase(),
	}
	return [
		{
			size: 200,
			minSize: 0,
			accessorKey: "name",
			id: "system",
			name: () => t`System`,
			filterFn: (row, _, filterVal) => {
				const filterLower = filterVal.toLowerCase()
				const { name, status } = row.original
				// Check if the filter matches the name or status for this row
				if (
					name.toLowerCase().includes(filterLower) ||
					statusTranslations[status as keyof typeof statusTranslations]?.().includes(filterLower)
				) {
					return true
				}
				return false
			},
			enableHiding: false,
			invertSorting: false,
			Icon: ServerIcon,
			cell: (info) => (
				<span className="flex gap-2 items-center font-medium text-sm text-nowrap md:ps-1 md:pe-5">
					<IndicatorDot system={info.row.original} />
					{info.getValue() as string}
				</span>
			),
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
				if (max === 0 && (status === "paused" || minor < 12)) {
					return null
				}

				const normalizedLoad = max / (sysInfo.t ?? 1)
				const threshold = getMeterState(normalizedLoad * 100)

				return (
					<div className="flex items-center gap-[.35em] w-full tabular-nums tracking-tight">
						<span
							className={cn("inline-block size-2 rounded-full me-0.5", {
								"bg-green-500": threshold === MeterState.Good,
								"bg-yellow-500": threshold === MeterState.Warn,
								"bg-red-600": threshold === MeterState.Crit,
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
				if (sys.status === "paused") {
					return null
				}
				const userSettings = useStore($userSettings)
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
				if (!val) {
					return null
				}
				const userSettings = useStore($userSettings)
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
				return (
					<span className={cn("flex gap-2 items-center md:pe-5 tabular-nums", viewMode === "table" && "ps-0.5")}>
						<IndicatorDot
							system={system}
							className={
								(system.status !== "up" && "bg-primary/30") ||
								(version === globalThis.BESZEL.HUB_VERSION && "bg-green-500") ||
								"bg-yellow-500"
							}
						/>
						<span className="truncate max-w-14">{info.getValue() as string}</span>
					</span>
				)
			},
		},
		{
			id: "actions",
			// @ts-ignore
			name: () => t({ message: "Actions", comment: "Table column" }),
			size: 50,
			cell: ({ row }) => (
				<div className="flex justify-end items-center gap-1 -ms-3">
					<AlertButton system={row.original} />
					<ActionsButton system={row.original} />
				</div>
			),
		},
	] as ColumnDef<SystemRecord>[]
}

function sortableHeader(context: HeaderContext<SystemRecord, unknown>) {
	const { column } = context
	// @ts-ignore
	const { Icon, hideSort, name }: { Icon: React.ElementType; name: () => string; hideSort: boolean } = column.columnDef
	return (
		<Button
			variant="ghost"
			className="h-9 px-3 flex"
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
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight">
			<span className="min-w-8">{decimalString(val, val >= 10 ? 1 : 2)}%</span>
			<span className="grow min-w-8 block bg-muted h-[1em] relative rounded-sm overflow-hidden">
				<span
					className={cn(
						"absolute inset-0 w-full h-full origin-left",
						(info.row.original.status !== "up" && "bg-primary/30") ||
							(threshold === MeterState.Good && "bg-green-500") ||
							(threshold === MeterState.Warn && "bg-yellow-500") ||
							"bg-red-600"
					)}
					style={{
						transform: `scalex(${val / 100})`,
					}}
				></span>
			</span>
		</div>
	)
}

export function IndicatorDot({ system, className }: { system: SystemRecord; className?: ClassValue }) {
	className ||= {
		"bg-green-500": system.status === "up",
		"bg-red-500": system.status === "down",
		"bg-primary/40": system.status === "paused",
		"bg-yellow-500": system.status === "pending",
	}
	return (
		<span
			className={cn("flex-shrink-0 size-2 rounded-full", className)}
			// style={{ marginBottom: "-1px" }}
		/>
	)
}

export const ActionsButton = memo(({ system }: { system: SystemRecord }) => {
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [editOpen, setEditOpen] = useState(false)
	let editOpened = useRef(false)
	const { t } = useLingui()
	const { id, status, host, name } = system

	return useMemo(() => {
		return (
			<>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="ghost" size={"icon"} data-nolink>
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
									status: status === "paused" ? "pending" : "paused",
								})
							}}
						>
							{status === "paused" ? (
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
