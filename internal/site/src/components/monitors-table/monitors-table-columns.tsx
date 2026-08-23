/** biome-ignore-all lint/correctness/useHookAtTopLevel: Hooks live inside memoized column definitions */
import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { getPagePath } from "@nanostores/router"
import type { ColumnDef, HeaderContext } from "@tanstack/react-table"
import {
	ArrowUpDownIcon,
	CheckCheckIcon,
	GlobeIcon,
	HourglassIcon,
	NetworkIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	Trash2Icon,
} from "lucide-react"
import { memo, useRef, useState } from "react"
import { isReadOnlyUser, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import { cn } from "@/lib/utils"
import type { MonitorRecord } from "@/types"
import { MonitorDialog } from "../add-monitor"
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
import { Button } from "../ui/button"
import { Dialog } from "../ui/dialog"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"

export const MONITOR_TYPE_LABELS = {
	http: () => t`HTTP`,
	tcp: () => t`TCP`,
	ping: () => t`Ping`,
} as const

export const MONITOR_TYPE_ICONS = {
	http: GlobeIcon,
	tcp: NetworkIcon,
	ping: CheckCheckIcon,
} as const

const STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

export const IndicatorDot = memo(({ monitor }: { monitor: MonitorRecord }) => {
	const color = STATUS_COLORS[monitor.status as keyof typeof STATUS_COLORS] || "bg-gray-500"
	return <span className={`size-2.5 inline-block rounded-full ${color}`} />
})

/**
 * @param viewMode - "table" or "grid"
 * @returns - Column definitions for the monitors table
 */
export function MonitorTableColumns(_viewMode: "table" | "grid"): ColumnDef<MonitorRecord>[] {
	return [
		{
			size: 200,
			minSize: 0,
			accessorKey: "name",
			id: "monitor",
			name: () => t`Monitor`,
			sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
			filterFn: (() => {
				let filterInput = ""
				let filterInputLower = ""
				const nameCache = new Map<string, string>()
				const statusTranslations = {
					[SystemStatus.Up]: t`Up`.toLowerCase(),
					[SystemStatus.Down]: t`Down`.toLowerCase(),
					[SystemStatus.Paused]: t`Paused`.toLowerCase(),
				} as const
				return (row, _, newFilterInput) => {
					const mon = row.original
					if ((mon.url || "").toLowerCase().includes(newFilterInput.toLowerCase())) return true
					if ((mon.host || "").toLowerCase().includes(newFilterInput.toLowerCase())) return true
					if (newFilterInput !== filterInput) {
						filterInput = newFilterInput
						filterInputLower = newFilterInput.toLowerCase()
					}
					let nameLower = nameCache.get(mon.name)
					if (nameLower === undefined) {
						nameLower = mon.name.toLowerCase()
						nameCache.set(mon.name, nameLower)
					}
					if (nameLower.includes(filterInputLower)) return true
					if ((mon.type || "").toLowerCase().includes(filterInputLower)) return true
					const statusLower = statusTranslations[mon.status as keyof typeof statusTranslations]
					return statusLower?.includes(filterInputLower) || false
				}
			})(),
			enableHiding: false,
			invertSorting: false,
			Icon: GlobeIcon,
			cell: (info) => {
				const { name, id, type } = info.row.original
				const linkUrl = getPagePath($router, "monitor", { id })
				const TypeIcon = MONITOR_TYPE_ICONS[type as keyof typeof MONITOR_TYPE_ICONS] || GlobeIcon
				return (
					<>
						<span className="flex gap-2 items-center font-medium text-sm text-nowrap md:ps-1">
							<IndicatorDot monitor={info.row.original} />
							<Link href={linkUrl} tabIndex={-1} className="truncate z-10 relative">
								<TypeIcon className="size-3.5 me-1 -ms-1.5 opacity-70 shrink-0" />
								{name}
							</Link>
						</span>
						<Link href={linkUrl} className="inset-0 absolute size-full" aria-label={name}></Link>
					</>
				)
			},
			header: sortableHeader,
		},
		{
			accessorKey: "type",
			id: "type",
			name: () => t`Type`,
			sortingFn: (a, b) => (a.original.type || "").localeCompare(b.original.type || ""),
			Icon: NetworkIcon,
			header: sortableHeader,
			cell: (info) => {
				const mon = info.row.original
				const labelFn = MONITOR_TYPE_LABELS[mon.type as keyof typeof MONITOR_TYPE_LABELS]
				const target = mon.url || mon.host || ""
				return (
					<span className="text-sm text-muted-foreground" title={target}>
						{labelFn ? labelFn() : mon.type}
					</span>
				)
			},
		},
		{
			accessorKey: "status",
			id: "status",
			name: () => t`Status`,
			sortingFn: (a, b) => (a.original.status || "").localeCompare(b.original.status || ""),
			Icon: PlayCircleIcon,
			header: sortableHeader,
			cell: (info) => {
				const { status } = info.row.original
				const statusLabels = {
					[SystemStatus.Up]: () => t`Up`,
					[SystemStatus.Down]: () => t`Down`,
					[SystemStatus.Paused]: () => t`Paused`,
					[SystemStatus.Pending]: () => t`Pending`,
				} as const
				const labelFn = statusLabels[status as keyof typeof statusLabels]
				return <span className="text-sm">{labelFn ? labelFn() : status}</span>
			},
		},
		{
			accessorFn: ({ info }) => info.interval,
			id: "interval",
			name: () => t`Interval`,
			sortingFn: (a, b) => (a.original.interval || 0) - (b.original.interval || 0),
			sortUndefined: "last",
			Icon: HourglassIcon,
			header: sortableHeader,
			cell: (info) => {
				const interval = info.row.original.interval
				if (!interval) return null
				return <span className="text-sm tabular-nums">{interval}s</span>
			},
		},
		{
			id: "actions",
			header: () => null,
			cell: (info) => <ActionsButton monitor={info.row.original} />,
			enableHiding: false,
		},
	] as ColumnDef<MonitorRecord>[]
}

function sortableHeader(context: HeaderContext<MonitorRecord, unknown>) {
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

export function ActionsButton({ monitor }: { monitor: MonitorRecord }) {
	const [editOpen, setEditOpen] = useState(false)
	const [deleteOpen, setDeleteOpen] = useState(false)
	const editOpened = useRef(false)
	const { status, id, name } = monitor

	if (isReadOnlyUser()) return null

	const togglePause = () => {
		pb.collection("monitors").update(id, { status: status === SystemStatus.Paused ? SystemStatus.Pending : SystemStatus.Paused })
	}

	return (
		<>
			<div className="flex items-center gap-1 ms-auto justify-end">
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon"
							className="size-8"
							onClick={(e) => {
								e.preventDefault()
								e.stopPropagation()
								pb.send(`/api/beszel/uptime/check-now?monitor=${id}`, {})
							}}
						>
							<PlayCircleIcon className="size-4" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						<Trans>Check now</Trans>
					</TooltipContent>
				</Tooltip>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon"
							className="size-8"
							onClick={(e) => {
								e.preventDefault()
								e.stopPropagation()
								setEditOpen(true)
							}}
						>
							<PenBoxIcon className="size-4" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						<Trans>Edit</Trans>
					</TooltipContent>
				</Tooltip>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon"
							className="size-8"
							onClick={(e) => {
								e.preventDefault()
								e.stopPropagation()
								togglePause()
							}}
						>
							{status === SystemStatus.Paused ? <PlayCircleIcon className="size-4" /> : <PauseCircleIcon className="size-4" />}
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						{status === SystemStatus.Paused ? <Trans>Resume</Trans> : <Trans>Pause</Trans>}
					</TooltipContent>
				</Tooltip>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon"
							className="size-8"
							onClick={(e) => {
								e.preventDefault()
								e.stopPropagation()
								setDeleteOpen(true)
							}}
						>
							<Trash2Icon className="size-4" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						<Trans>Delete</Trans>
					</TooltipContent>
				</Tooltip>
			</div>

			<Dialog open={editOpen} onOpenChange={(open) => { if (open) editOpened.current = true; setEditOpen(open) }}>
				{editOpened.current && <MonitorDialog monitor={monitor} setOpen={setEditOpen} />}
			</Dialog>

			<AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Are you sure you want to delete {name}?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>
								This action cannot be undone. This will permanently delete all current records for {name} from the database.
							</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn("bg-destructive text-destructive-foreground hover:bg-destructive/90")}
							onClick={() => pb.collection("monitors").delete(id)}
						>
							<Trans>Continue</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	)
}
