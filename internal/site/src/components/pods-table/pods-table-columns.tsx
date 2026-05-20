import type { Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString, formatBytes, hourWithSeconds } from "@/lib/utils"
import type { PodRecord } from "@/types"
import {
	ArrowUpDownIcon,
	ClockIcon,
	CpuIcon,
	LayersIcon,
	MemoryStickIcon,
	ServerIcon,
	BoxIcon,
} from "lucide-react"
import { EthernetIcon } from "../ui/icons"
import { Badge } from "../ui/badge"
import { t } from "@lingui/core/macro"
import { $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"

export const podChartCols: ColumnDef<PodRecord>[] = [
	{
		id: "name",
		sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
		accessorFn: (record) => record.name,
		header: ({ column }) => <HeaderButton column={column} name={t`Name`} Icon={BoxIcon} />,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 xl:w-48 block truncate">{getValue() as string}</span>
		},
	},
	{
		id: "namespace",
		sortingFn: (a, b) => a.original.namespace.localeCompare(b.original.namespace),
		accessorFn: (record) => record.namespace,
		header: ({ column }) => <HeaderButton column={column} name={t`Namespace`} Icon={LayersIcon} />,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 xl:w-32 block truncate">{getValue() as string}</span>
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
		id: "memory",
		accessorFn: (record) => record.memory,
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
			const formatted = formatBytes(val, true, undefined, true)
			return (
				<span className="ms-1.5 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</span>
			)
		},
	},
	{
		id: "status",
		invertSorting: true,
		accessorFn: (record) => record.status,
		header: ({ column }) => <HeaderButton column={column} name={t`Status`} Icon={ClockIcon} />,
		cell: ({ getValue }) => {
			const status = getValue() as string
			return (
				<Badge variant="outline" className="dark:border-white/12">
					<span className={cn("size-2 me-1.5 rounded-full", {
						"bg-green-500": status === "Running",
						"bg-yellow-500": status === "Pending",
						"bg-red-500": status === "Failed",
						"bg-blue-500": status === "Succeeded",
						"bg-zinc-500": !["Running", "Pending", "Failed", "Succeeded"].includes(status),
					})}>
					</span>
					{status}
				</Badge>
			)
		},
	},
	{
		id: "restarts",
		accessorFn: (record) => record.restarts,
		invertSorting: true,
		header: ({ column }) => <HeaderButton column={column} name={t`Restarts`} Icon={undefined} />,
		cell: ({ getValue }) => {
			return <span className="ms-1.5 tabular-nums">{getValue() as number}</span>
		},
	},
	{
		id: "updated",
		invertSorting: true,
		accessorFn: (record) => record.updated,
		header: ({ column }) => <HeaderButton column={column} name={t`Updated`} Icon={ClockIcon} />,
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

function HeaderButton({ column, name, Icon }: { column: Column<PodRecord>; name: string; Icon?: React.ElementType }) {
	const isSorted = column.getIsSorted()
	return (
		<Button
			className={cn("h-9 px-3 flex items-center gap-2 duration-50", isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90")}
			variant="ghost"
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{Icon && <Icon className="size-4" />}
			{name}
			<ArrowUpDownIcon className="size-4" />
		</Button>
	)
}
