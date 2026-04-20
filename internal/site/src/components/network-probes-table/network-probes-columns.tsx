import type { Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString, hourWithSeconds } from "@/lib/utils"
import {
	GlobeIcon,
	TagIcon,
	TimerIcon,
	ActivityIcon,
	WifiOffIcon,
	Trash2Icon,
	ArrowLeftRightIcon,
	MoreHorizontalIcon,
	ServerIcon,
	ClockIcon,
} from "lucide-react"
import { t } from "@lingui/core/macro"
import type { NetworkProbeRecord } from "@/types"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Trans } from "@lingui/react/macro"
import { pb } from "@/lib/api"
import { toast } from "../ui/use-toast"
import { $allSystemsById } from "@/lib/stores"
import { useStore } from "@nanostores/react"

// export interface ProbeRow extends NetworkProbeRecord {
// 	key: string
// 	latency?: number
// 	loss?: number
// }

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

export function getProbeColumns(longestName = 0, longestTarget = 0): ColumnDef<NetworkProbeRecord>[] {
	return [
		{
			id: "name",
			sortingFn: (a, b) => (a.original.name || a.original.target).localeCompare(b.original.name || b.original.target),
			accessorFn: (record) => record.name || record.target,
			header: ({ column }) => <HeaderButton column={column} name={t`Name`} Icon={TagIcon} />,
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
				const allSystems = useStore($allSystemsById)
				return <span className="ms-1.5 xl:w-34 block truncate">{allSystems[getValue() as string]?.name ?? ""}</span>
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
			header: ({ column }) => <HeaderButton column={column} name={t`Interval`} Icon={TimerIcon} />,
			cell: ({ getValue }) => <span className="ms-1.5 tabular-nums">{getValue() as number}s</span>,
		},
		{
			id: "latency",
			accessorFn: (record) => record.latency,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Latency`} Icon={ActivityIcon} />,
			cell: ({ row }) => {
				const val = row.original.latency
				if (val === undefined) {
					return <span className="ms-1.5 text-muted-foreground">-</span>
				}
				let color = "bg-green-500"
				if (val > 200) {
					color = "bg-yellow-500"
				}
				if (!val || val > 2000) {
					color = "bg-red-500"
				}
				return (
					<span className="ms-1.5 tabular-nums flex gap-2 items-center">
						<span className={cn("shrink-0 size-2 rounded-full", color)} />
						{decimalString(val, val < 100 ? 2 : 1).toLocaleString()} ms
					</span>
				)
			},
		},
		{
			id: "loss",
			accessorFn: (record) => record.loss,
			invertSorting: true,
			header: ({ column }) => <HeaderButton column={column} name={t`Loss`} Icon={WifiOffIcon} />,
			cell: ({ row }) => {
				const val = row.original.loss
				if (val === undefined) {
					return <span className="ms-1.5 text-muted-foreground">-</span>
				}
				let color = "bg-green-500"
				if (val > 0) {
					color = val > 20 ? "bg-red-500" : "bg-yellow-500"
				}
				return (
					<span className="ms-1.5 tabular-nums flex gap-2 items-center">
						<span className={cn("shrink-0 size-2 rounded-full", color)} />
						{val}%
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
			header: () => null,
			size: 40,
			cell: ({ row }) => (
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
			),
		},
	]
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
			{/* <ArrowUpDownIcon className="size-4" /> */}
		</Button>
	)
}
