import type { ColumnDef } from "@tanstack/react-table"
import { sortableHeader } from "@/components/ui/data-table-settings"
import { cn, decimalString, formatBytes, hourWithSeconds } from "@/lib/utils"
import type { SystemdRecord } from "@/types"
import { ServiceStatus, ServiceStatusLabels, ServiceSubState, ServiceSubStateLabels } from "@/lib/enums"
import {
	ActivityIcon,
	ClockIcon,
	CpuIcon,
	MemoryStickIcon,
	TerminalSquareIcon,
} from "lucide-react"
import { Badge } from "../ui/badge"
import { t } from "@lingui/core/macro"
// import { $allSystemsById } from "@/lib/stores"
// import { useStore } from "@nanostores/react"

function getSubStateColor(subState: ServiceSubState) {
	switch (subState) {
		case ServiceSubState.Running:
			return "bg-green-500"
		case ServiceSubState.Failed:
			return "bg-red-500"
		case ServiceSubState.Dead:
			return "bg-yellow-500"
		default:
			return "bg-zinc-500"
	}
}


export const systemdTableCols: ColumnDef<SystemdRecord>[] = [
	{
		id: "name",
		meta: { label: () => t`Name`, Icon: TerminalSquareIcon },
		enableHiding: false,
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		accessorFn: (record) => record.name,
		header: sortableHeader,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 xl:w-50 block truncate">{getValue() as string}</span>
		},
	},
	// {
	// 	id: "system",
	// 	accessorFn: (record) => record.system,
	// 	sortingFn: (a, b) => {
	// 		const allSystems = $allSystemsById.get()
	// 		const systemNameA = allSystems[a.original.system]?.name ?? ""
	// 		const systemNameB = allSystems[b.original.system]?.name ?? ""
	// 		return systemNameA.localeCompare(systemNameB)
	// 	},
	// 	header: ({ column }) => <HeaderButton column={column} name={t`System`} Icon={ServerIcon} />,
	// 	cell: ({ getValue }) => {
	// 		const allSystems = useStore($allSystemsById)
	// 		return <span className="ms-1.5 xl:w-34 block truncate">{allSystems[getValue() as string]?.name ?? ""}</span>
	// 	},
	// },
	{
		id: "state",
		meta: { label: () => t`State`, Icon: ActivityIcon },
		accessorFn: (record) => record.state,
		header: sortableHeader,
		cell: ({ getValue }) => {
			const statusValue = getValue() as ServiceStatus
			const statusLabel = ServiceStatusLabels[statusValue] || "Unknown"
			return (
				<Badge variant="outline" className="dark:border-white/12">
					<span className={cn("size-2 me-1.5 rounded-full", getStatusColor(statusValue))} />
					{statusLabel}
				</Badge>
			)
		},
	},
	{
		id: "sub",
		meta: { label: () => t`Sub State`, Icon: ActivityIcon },
		accessorFn: (record) => record.sub,
		header: sortableHeader,
		cell: ({ getValue }) => {
			const subState = getValue() as ServiceSubState
			const subStateLabel = ServiceSubStateLabels[subState] || "Unknown"
			return (
				<Badge variant="outline" className="dark:border-white/12 text-xs capitalize">
					<span className={cn("size-2 me-1.5 rounded-full", getSubStateColor(subState))} />
					{subStateLabel}
				</Badge>
			)
		},
	},
	{
		id: "cpu",
		meta: { label: () => `${t`CPU`} (10m)`, Icon: CpuIcon },
		accessorFn: (record) => {
			if (record.sub !== ServiceSubState.Running) {
				return -1
			}
			return record.cpu
		},
		invertSorting: true,
		header: sortableHeader,
		cell: ({ getValue }) => {
			const val = getValue() as number
			if (val < 0) {
				return <span className="ms-1.5 text-muted-foreground">N/A</span>
			}
			return <span className="ms-1.5 tabular-nums">{`${decimalString(val, val >= 10 ? 1 : 2)}%`}</span>
		},
	},
	{
		id: "cpuPeak",
		meta: { label: () => t`CPU Peak`, Icon: CpuIcon },
		accessorFn: (record) => {
			if (record.sub !== ServiceSubState.Running) {
				return -1
			}
			return record.cpuPeak ?? 0
		},
		invertSorting: true,
		header: sortableHeader,
		cell: ({ getValue }) => {
			const val = getValue() as number
			if (val < 0) {
				return <span className="ms-1.5 text-muted-foreground">N/A</span>
			}
			return <span className="ms-1.5 tabular-nums">{`${decimalString(val, val >= 10 ? 1 : 2)}%`}</span>
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
			if (!val) {
				return <span className="ms-1.5 text-muted-foreground">N/A</span>
			}
			const formatted = formatBytes(val, false, undefined, false)
			return (
				<span className="ms-1.5 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</span>
			)
		},
	},
	{
		id: "memPeak",
		meta: { label: () => t`Memory Peak`, Icon: MemoryStickIcon },
		accessorFn: (record) => record.memPeak,
		invertSorting: true,
		header: sortableHeader,
		cell: ({ getValue }) => {
			const val = getValue() as number
			if (!val) {
				return <span className="ms-1.5 text-muted-foreground">N/A</span>
			}
			const formatted = formatBytes(val, false, undefined, false)
			return (
				<span className="ms-1.5 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</span>
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
			return (
				<span className="ms-1.5 tabular-nums">
					{hourWithSeconds(new Date(timestamp).toISOString())}
				</span>
			)
		},
	},
]


export function getStatusColor(status: ServiceStatus) {
	switch (status) {
		case ServiceStatus.Active:
			return "bg-green-500"
		case ServiceStatus.Failed:
			return "bg-red-500"
		case ServiceStatus.Reloading:
		case ServiceStatus.Activating:
		case ServiceStatus.Deactivating:
			return "bg-yellow-500"
		default:
			return "bg-zinc-500"
	}
}