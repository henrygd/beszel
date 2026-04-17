import type { Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString, formatBytes, hourWithSeconds } from "@/lib/utils"
import type { ContainerRecord } from "@/types"
import { ContainerHealth, ContainerHealthLabels } from "@/lib/enums"
import {
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
import { $allSystemsById, $longestSystemNameLen } from "@/lib/stores"
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
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		accessorFn: (record) => record.name,
		header: ({ column }) => <HeaderButton column={column} name={t`Name`} Icon={ContainerIcon} />,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 xl:w-48 block truncate">{getValue() as string}</span>
		},
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
			const allSystems = useStore($allSystemsById)
			const longestName = useStore($longestSystemNameLen)
			return (
				<div className="ms-1 max-w-40 truncate" style={{ width: `${longestName / 1.05}ch` }}>
					{allSystems[getValue() as string]?.name ?? ""}
				</div>
			)
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
		accessorFn: (record) => record.cpu,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`CPU`} Icon={CpuIcon} />,
		cell: ({ getValue }) => {
			const val = getValue() as number
			return <span className="ms-1 tabular-nums">{`${decimalString(val, val >= 10 ? 1 : 2)}%`}</span>
		},
	},
	{
		id: "memory",
		accessorFn: (record) => record.memory,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Memory`} Icon={MemoryStickIcon} />,
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
		accessorFn: (record) => record.net,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Net`} Icon={EthernetIcon} />,
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
		invertSorting: true,
		accessorFn: (record) => record.health,
		header: ({ column }) => <HeaderButton column={column} name={t`Health`} Icon={ShieldCheckIcon} />,
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
		accessorFn: (record) => record.ports || undefined,
		header: ({ column }) => (
			<HeaderButton
				column={column}
				name={t({ message: "Ports", context: "Container ports" })}
				Icon={SquareArrowRightEnterIcon}
			/>
		),
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
		sortingFn: (a, b) => a.original.image.localeCompare(b.original.image),
		accessorFn: (record) => record.image,
		header: ({ column }) => (
			<HeaderButton column={column} name={t({ message: "Image", context: "Docker image" })} Icon={LayersIcon} />
		),
		cell: ({ getValue }) => {
			const val = getValue() as string
			return (
				<div className="ms-1 xl:w-40 truncate" title={val}>
					{val}
				</div>
			)
		},
	},
	{
		id: "status",
		accessorFn: (record) => record.status,
		invertSorting: true,
		sortingFn: (a, b) => getStatusValue(a.original.status) - getStatusValue(b.original.status),
		header: ({ column }) => <HeaderButton column={column} name={t`Status`} Icon={HourglassIcon} />,
		cell: ({ getValue }) => {
			return <span className="ms-1 w-25 block truncate">{getValue() as string}</span>
		},
	},
	{
		id: "updated",
		invertSorting: true,
		accessorFn: (record) => record.updated,
		header: ({ column }) => <HeaderButton column={column} name={t`Updated`} Icon={ClockIcon} />,
		cell: ({ getValue }) => {
			const timestamp = getValue() as number
			return <span className="ms-1 tabular-nums">{hourWithSeconds(new Date(timestamp).toISOString())}</span>
		},
	},
]

function HeaderButton({
	column,
	name,
	Icon,
}: {
	column: Column<ContainerRecord>
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
			{/* <ArrowUpDownIcon className="size-4" /> */}
		</Button>
	)
}

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
