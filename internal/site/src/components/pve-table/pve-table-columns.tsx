import type { Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString, formatBytes, hourWithSeconds, toFixedFloat } from "@/lib/utils"
import type { PveVmRecord } from "@/types"
import {
	ArrowUpDownIcon,
	ClockIcon,
	CpuIcon,
	MemoryStickIcon,
	MonitorIcon,
	ServerIcon,
	TagIcon,
	TimerIcon,
} from "lucide-react"
import { EthernetIcon } from "../ui/icons"
import { Badge } from "../ui/badge"
import { t } from "@lingui/core/macro"
import { $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"

/** Format uptime in seconds to a human-readable string */
export function formatUptime(seconds: number): string {
	if (seconds < 60) return `${seconds}s`
	if (seconds < 3600) {
		const m = Math.floor(seconds / 60)
		return `${m}m`
	}
	if (seconds < 86400) {
		const h = Math.floor(seconds / 3600)
		const m = Math.floor((seconds % 3600) / 60)
		return m > 0 ? `${h}h ${m}m` : `${h}h`
	}
	const d = Math.floor(seconds / 86400)
	const h = Math.floor((seconds % 86400) / 3600)
	return h > 0 ? `${d}d ${h}h` : `${d}d`
}

export const pveVmCols: ColumnDef<PveVmRecord>[] = [
	{
		id: "name",
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		accessorFn: (record) => record.name,
		header: ({ column }) => <HeaderButton column={column} name={t`Name`} Icon={MonitorIcon} />,
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
			return <span className="ms-1.5 xl:w-34 block truncate">{allSystems[getValue() as string]?.name ?? ""}</span>
		},
	},
	{
		id: "type",
		accessorFn: (record) => record.type,
		sortingFn: (a, b) => a.original.type.localeCompare(b.original.type),
		header: ({ column }) => <HeaderButton column={column} name={t`Type`} Icon={TagIcon} />,
		cell: ({ getValue }) => {
			const type = getValue() as string
			return (
				<Badge variant="outline" className="dark:border-white/12 ms-1.5">
					{type}
				</Badge>
			)
		},
	},
	{
		id: "cpu",
		accessorFn: (record) => record.cpu,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`CPU`} Icon={CpuIcon} />,
		cell: ({ getValue }) => {
			const val = getValue() as number
			return <span className="ms-1.5 tabular-nums">{`${decimalString(val, val >= 10 ? 1 : 2)}%`}</span>
		},
	},
	{
		id: "mem",
		accessorFn: (record) => record.mem,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Memory`} Icon={MemoryStickIcon} />,
		cell: ({ getValue }) => {
			const val = getValue() as number
			const formatted = formatBytes(val, false, undefined, true)
			return (
				<span className="ms-1.5 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</span>
			)
		},
	},
	{
		id: "net",
		accessorFn: (record) => record.net,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Net`} Icon={EthernetIcon} />,
		cell: ({ getValue }) => {
			const val = getValue() as number
			const formatted = formatBytes(val, true, undefined, false)
			return (
				<span className="ms-1.5 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</span>
			)
		},
	},
	{
		id: "maxcpu",
		accessorFn: (record) => record.maxcpu,
		header: ({ column }) => <HeaderButton column={column} name={t`vCPUs`} Icon={CpuIcon} />,
		invertSorting: true,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 tabular-nums">{getValue() as number}</span>
		},
	},
	{
		id: "maxmem",
		accessorFn: (record) => record.maxmem,
		header: ({ column }) => <HeaderButton column={column} name={t`Max Mem`} Icon={MemoryStickIcon} />,
		invertSorting: true,
		cell: ({ getValue }) => {
			// maxmem is stored in bytes; convert to MB for formatBytes
			const formatted = formatBytes(getValue() as number, false, undefined, false)
			return <span className="ms-1.5 tabular-nums">{`${toFixedFloat(formatted.value, 2)} ${formatted.unit}`}</span>
		},
	},
	{
		id: "uptime",
		accessorFn: (record) => record.uptime,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Uptime`} Icon={TimerIcon} />,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 w-25 block truncate">{formatUptime(getValue() as number)}</span>
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
]

function HeaderButton({ column, name, Icon }: { column: Column<PveVmRecord>; name: string; Icon: React.ElementType }) {
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
			<ArrowUpDownIcon className="size-4" />
		</Button>
	)
}
