import type { Column, ColumnDef } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString } from "@/lib/utils"
import {
	GlobeIcon,
	TagIcon,
	TimerIcon,
	ActivityIcon,
	WifiOffIcon,
	Trash2Icon,
	ArrowLeftRightIcon,
	MoreHorizontalIcon,
} from "lucide-react"
import { t } from "@lingui/core/macro"
import type { NetworkProbeRecord } from "@/types"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Trans } from "@lingui/react/macro"

export interface ProbeRow extends NetworkProbeRecord {
	key: string
	latency?: number
	loss?: number
}

const protocolColors: Record<string, string> = {
	icmp: "bg-blue-500/15 text-blue-400",
	tcp: "bg-purple-500/15 text-purple-400",
	http: "bg-green-500/15 text-green-400",
}

export function getProbeColumns(
	deleteProbe: (id: string) => void,
	longestName = 0,
	longestTarget = 0
): ColumnDef<ProbeRow>[] {
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
					<span
						className={cn("ms-1.5 px-2 py-0.5 rounded text-xs font-medium uppercase", protocolColors[protocol] ?? "")}
					>
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
				return (
					<span className="ms-1.5 tabular-nums flex gap-2 items-center">
						<span className={cn("shrink-0 size-2 rounded-full", val > 100 ? "bg-yellow-500" : "bg-green-500")} />
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
				return (
					<span className="ms-1.5 tabular-nums flex gap-2 items-center">
						<span className={cn("shrink-0 size-2 rounded-full", val > 0 ? "bg-yellow-500" : "bg-green-500")} />
						{val}%
					</span>
				)
			},
		},
		{
			id: "actions",
			enableSorting: false,
			header: () => null,
			cell: ({ row }) => (
				<div className="flex justify-end">
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
				</div>
			),
		},
	]
}

function HeaderButton({ column, name, Icon }: { column: Column<ProbeRow>; name: string; Icon: React.ElementType }) {
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
