import type { ColumnDef } from "@tanstack/react-table"
import { sortableHeader } from "@/components/ui/data-table-settings"
import { cn, decimalString, formatBytes, hourWithSeconds } from "@/lib/utils"
import type { ContainerRecord } from "@/types"
import { ContainerHealth, ContainerHealthLabels } from "@/lib/enums"
import {
	CircleArrowUpIcon,
	ClockIcon,
	ContainerIcon,
	CpuIcon,
	LayersIcon,
	MemoryStickIcon,
	ServerIcon,
	ShieldCheckIcon,
} from "lucide-react"
import { EthernetIcon, HourglassIcon, SquareArrowRightEnterIcon } from "../ui/icons"
import { Badge } from "../ui/badge"
import { t } from "@lingui/core/macro"
import { $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"

// Unit names and their corresponding number of seconds for converting docker status strings
const unitSeconds = [
	["s", 1],
	["mi", 60],
	["h", 3600],
	["d", 86400],
	["w", 604800],
	["mo", 2592000],
] as const
// Convert docker status string to number of seconds ("Up X minutes", "Up X hours", etc.)
function getStatusValue(status: string): number {
	const [_, num, unit] = status.split(" ")
	// Docker uses "a" or "an" instead of "1" for singular units (e.g., "Up a minute", "Up an hour")
	const numValue = num === "a" || num === "an" ? 1 : Number(num)
	for (const [unitName, value] of unitSeconds) {
		if (unit.startsWith(unitName)) {
			return numValue * value
		}
	}
	return 0
}

export const containerChartCols: ColumnDef<ContainerRecord>[] = [
	{
		id: "name",
		meta: { label: () => t`Name`, Icon: ContainerIcon },
		enableHiding: false,
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		accessorFn: (record) => record.name,
		header: sortableHeader,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 xl:w-48 block truncate">{getValue() as string}</span>
		},
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
			return a.original.name.localeCompare(b.original.name)
		},
		header: sortableHeader,
		cell: ({ getValue }) => {
			const allSystems = useStore($allSystemsById)
			return <div className="ms-1 max-w-40 truncate">{allSystems[getValue() as string]?.name ?? ""}</div>
		},
	},
	// {
	// 	id: "id",
	// 	accessorFn: (record) => record.id,
	// 	sortingFn: (a, b) => a.original.id.localeCompare(b.original.id),
	// 	header: ({ column }) => <HeaderButton column={column} name="ID" Icon={HashIcon} />,
	// 	cell: ({ getValue }) => {
	// 		return <span className="ms-1.5 me-3 font-mono">{getValue() as string}</span>
	// 	},
	// },
	{
		id: "cpu",
		meta: { label: () => t`CPU`, Icon: CpuIcon },
		accessorFn: (record) => record.cpu,
		invertSorting: true,
		header: sortableHeader,
		cell: ({ getValue }) => {
			const val = getValue() as number
			return <span className="ms-1 tabular-nums">{`${decimalString(val, val >= 10 ? 1 : 2)}%`}</span>
		},
	},
	{
		id: "memory",
		meta: { label: () => t`Memory`, Icon: MemoryStickIcon },
		accessorFn: (record) => record.memory,
		invertSorting: true,
		header: sortableHeader,
		cell: ({ getValue }) => {
			const val = getValue() as number
			const formatted = formatBytes(val, false, undefined, true)
			return (
				<span className="ms-1 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</span>
			)
		},
	},
	{
		id: "net",
		meta: { label: () => t`Net`, Icon: EthernetIcon },
		accessorFn: (record) => record.net,
		invertSorting: true,
		header: sortableHeader,
		minSize: 112,
		cell: ({ getValue }) => {
			const val = getValue() as number
			const formatted = formatBytes(val, true, undefined, false)
			return (
				<div className="ms-1 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</div>
			)
		},
	},
	{
		id: "health",
		meta: { label: () => t`Health`, Icon: ShieldCheckIcon },
		invertSorting: true,
		accessorFn: (record) => record.health,
		header: sortableHeader,
		minSize: 121,
		cell: ({ getValue }) => {
			const healthValue = getValue() as number
			const healthStatus = ContainerHealthLabels[healthValue] || "Unknown"
			return (
				<Badge variant="outline" className="dark:border-white/12">
					<span
						className={cn("size-2 me-1.5 rounded-full", {
							"bg-green-500": healthValue === ContainerHealth.Healthy,
							"bg-red-500": healthValue === ContainerHealth.Unhealthy,
							"bg-yellow-500": healthValue === ContainerHealth.Starting,
							"bg-zinc-500": healthValue === ContainerHealth.None,
						})}
					></span>
					{healthStatus}
				</Badge>
			)
		},
	},
	{
		id: "ports",
		meta: { label: () => t({ message: "Ports", context: "Container ports" }), Icon: SquareArrowRightEnterIcon },
		accessorFn: (record) => record.ports || undefined,
		header: sortableHeader,
		sortingFn: (a, b) => getPortValue(a.original.ports) - getPortValue(b.original.ports),
		minSize: 147,
		cell: ({ getValue }) => {
			const val = getValue() as string | undefined
			if (!val) {
				return <div className="ms-1.5 text-muted-foreground">-</div>
			}
			const className = "ms-1 w-27 block truncate tabular-nums"
			if (val.length > 14) {
				return (
					<Tooltip>
						<TooltipTrigger className={className}>{val}</TooltipTrigger>
						<TooltipContent>{val}</TooltipContent>
					</Tooltip>
				)
			}
			return <span className={className}>{val}</span>
		},
	},
	{
		id: "image",
		meta: { label: () => t({ message: "Image", context: "Docker image" }), Icon: LayersIcon },
		sortingFn: (a, b) => a.original.image.localeCompare(b.original.image),
		accessorFn: (record) => record.image,
		header: sortableHeader,
		cell: ({ getValue, row }) => {
			const val = getValue() as string
			return (
				<div className="ms-1 xl:w-40 flex items-center gap-2">
					<span className="truncate" title={val}>
						{val}
					</span>
					{row.original.updatable && (
						<Tooltip>
							<TooltipTrigger
								className="shrink-0 rounded-sm text-emerald-600 dark:text-emerald-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
								aria-label={t({ message: "Image update available", context: "Docker image" })}
								onClick={(event) => event.stopPropagation()}
							>
								<CircleArrowUpIcon className="size-4" aria-hidden="true" />
							</TooltipTrigger>
							<TooltipContent>{t({ message: "Image update available", context: "Docker image" })}</TooltipContent>
						</Tooltip>
					)}
				</div>
			)
		},
	},
	{
		id: "status",
		meta: { label: () => t`Status`, Icon: HourglassIcon },
		accessorFn: (record) => record.status,
		invertSorting: true,
		sortingFn: (a, b) => getStatusValue(a.original.status) - getStatusValue(b.original.status),
		header: sortableHeader,
		cell: ({ getValue }) => {
			return <span className="ms-1 w-25 block truncate">{getValue() as string}</span>
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
			return <span className="ms-1 tabular-nums">{hourWithSeconds(new Date(timestamp).toISOString())}</span>
		},
	},
]


/**
 * Convert port string to a number for sorting.
 * Handles formats like "80", "127.0.0.1:80", and "80, 443" (takes the first mapping).
 */
function getPortValue(ports: string | undefined): number {
	if (!ports) {
		return 0
	}
	const first = ports.includes(",") ? ports.substring(0, ports.indexOf(",")) : ports
	const colonIndex = first.lastIndexOf(":")
	const portStr = colonIndex === -1 ? first : first.substring(colonIndex + 1)
	return Number(portStr) || 0
}
