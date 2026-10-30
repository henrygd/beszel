import type { CellContext, Column, ColumnDef, RowData } from "@tanstack/react-table"
import { Button } from "@/components/ui/button"
import { cn, decimalString, formatBytes, hourWithSeconds } from "@/lib/utils"
import type { ContainerRecord } from "@/types"
import { ContainerHealth, ContainerHealthLabels } from "@/lib/enums"
import {
	BoxesIcon,
	CircleArrowUpIcon,
	ClockIcon,
	ComponentIcon,
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
import { ContainerGroupLabel, GROUP_INDENT } from "@/components/containers-table/container-group-row"
import { StackNodesCell } from "@/components/containers-table/stack-nodes-cell"
import {
	compareGroupAwareName,
	comparePorts,
	compareStatus,
	countLeaves,
	getServiceGroupingValue,
	getServiceValue,
	getStackGroupingValue,
	getStackValue,
	makeSystemComparator,
	MULTIPLE_NODES,
	sharedValue,
	SERVICE_COLUMN_ID,
	type StackNodeSummary,
	STACK_COLUMN_ID,
	worstHealth,
} from "@/lib/container-grouping"

declare module "@tanstack/react-table" {
	interface TableMeta<TData extends RowData> {
		/**
		 * Node summary of every stack by stack name, built by the page from the full data. The
		 * stack row reads it instead of its own leaf rows, so a filter narrows what is listed
		 * without changing what the row says about where the stack lives.
		 */
		stackSummaries?: Map<string, StackNodeSummary>
	}
}

export const containerChartCols: ColumnDef<ContainerRecord>[] = [
	{
		id: "name",
		sortingFn: compareGroupAwareName,
		accessorFn: (record) => record.name,
		// the number of containers lives here: the aggregation of a column the grouping runs on
		// is never called, so the stack and service columns cannot carry it
		aggregationFn: (_columnId, leafRows) => countLeaves(leafRows),
		aggregatedCell: ({ row, getValue }) => <ContainerGroupLabel row={row} count={getValue() as number} />,
		header: ({ column }) => <HeaderButton column={column} name={t`Name`} Icon={ContainerIcon} />,
		cell: ({ getValue, row }) => {
			// indented like the label of the group above it, so the nesting can be read per row.
			// no style at all without grouping, so the flat table keeps exactly the markup it has
			return (
				<span
					className="ms-1.5 xl:w-48 block truncate"
					style={row.depth ? { paddingInlineStart: row.depth * GROUP_INDENT } : undefined}
				>
					{getValue() as string}
				</span>
			)
		},
	},
	{
		id: "system",
		accessorFn: (record) => record.system,
		sortingFn: makeSystemComparator((value) => $allSystemsById.get()[value as string]?.name ?? ""),
		// a stack can be spread over the swarm, a service row is always on one node
		aggregationFn: (columnId, leafRows) =>
			sharedValue(leafRows.map((leafRow) => leafRow.getValue<string>(columnId))) ?? MULTIPLE_NODES,
		// a stack spans the swarm: the stack row counts its nodes and shows what runs where,
		// instead of naming an aggregate that is not a node. A service row is always on one node
		aggregatedCell: ({ row, getValue, table }) =>
			row.groupingColumnId === STACK_COLUMN_ID ? (
				<StackNodesCell row={row} table={table} />
			) : (
				<NodeNameCell value={getValue() as string} />
			),
		header: ({ column }) => <HeaderButton column={column} name={t`System`} Icon={ServerIcon} />,
		cell: ({ getValue }) => <NodeNameCell value={getValue() as string} />,
	},
	{
		id: STACK_COLUMN_ID,
		accessorFn: (record) => getStackValue(record),
		// containers without labels get a value of their own, so they don't end up in one group
		getGroupingValue: (record) => getStackGroupingValue(record),
		sortingFn: compareGroupAwareName,
		header: ({ column }) => (
			<HeaderButton
				column={column}
				name={t({ message: "Stack", context: "Docker Compose or Swarm stack" })}
				Icon={BoxesIcon}
			/>
		),
		cell: renderGroupValue,
		aggregatedCell: renderGroupValue,
	},
	{
		id: SERVICE_COLUMN_ID,
		accessorFn: (record) => getServiceValue(record),
		// the node is part of the key: replicas are only folded together per node
		getGroupingValue: (record) => getServiceGroupingValue(record),
		sortingFn: compareGroupAwareName,
		header: ({ column }) => (
			<HeaderButton
				column={column}
				name={t({ message: "Service", context: "Docker Swarm service" })}
				Icon={ComponentIcon}
			/>
		),
		cell: renderGroupValue,
		aggregatedCell: renderGroupValue,
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
		aggregationFn: "sum",
		header: ({ column }) => <HeaderButton column={column} name={t`CPU`} Icon={CpuIcon} />,
		cell: renderCpu,
		aggregatedCell: renderCpu,
	},
	{
		id: "memory",
		accessorFn: (record) => record.memory,
		invertSorting: true,
		aggregationFn: "sum",
		header: ({ column }) => <HeaderButton column={column} name={t`Memory`} Icon={MemoryStickIcon} />,
		cell: renderMemory,
		aggregatedCell: renderMemory,
	},
	{
		id: "net",
		accessorFn: (record) => record.net,
		invertSorting: true,
		aggregationFn: "sum",
		header: ({ column }) => <HeaderButton column={column} name={t`Net`} Icon={EthernetIcon} />,
		minSize: 112,
		cell: renderNet,
		aggregatedCell: renderNet,
	},
	{
		id: "health",
		invertSorting: true,
		accessorFn: (record) => record.health,
		// worst health of the group: Unhealthy > Starting > Healthy > None
		aggregationFn: (columnId, leafRows) => worstHealth(leafRows.map((leafRow) => leafRow.getValue<number>(columnId))),
		header: ({ column }) => <HeaderButton column={column} name={t`Health`} Icon={ShieldCheckIcon} />,
		minSize: 121,
		cell: renderHealth,
		aggregatedCell: renderHealth,
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
		sortingFn: comparePorts,
		aggregatedCell: () => <NoAggregate />,
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
		sortingFn: compareGroupAwareName,
		accessorFn: (record) => record.image,
		// no aggregate on purpose: a stack can run several images, and showing one would be a lie
		aggregatedCell: () => <NoAggregate />,
		header: ({ column }) => (
			<HeaderButton column={column} name={t({ message: "Image", context: "Docker image" })} Icon={LayersIcon} />
		),
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
		accessorFn: (record) => record.status,
		invertSorting: true,
		sortingFn: compareStatus,
		aggregatedCell: () => <NoAggregate />,
		header: ({ column }) => <HeaderButton column={column} name={t`Status`} Icon={HourglassIcon} />,
		cell: ({ getValue }) => {
			return <span className="ms-1 w-25 block truncate">{getValue() as string}</span>
		},
	},
	{
		id: "updated",
		invertSorting: true,
		accessorFn: (record) => record.updated,
		// freshest update in the group
		aggregationFn: "max",
		header: ({ column }) => <HeaderButton column={column} name={t`Updated`} Icon={ClockIcon} />,
		cell: renderUpdated,
		aggregatedCell: renderUpdated,
	},
]

/**
 * Cell renderers shared by the leaf rows and the group rows.
 *
 * The grouping feature of TanStack gives every column a default `aggregatedCell` that prints
 * `getValue()?.toString()`, so a group row only keeps the formatting of the column when the
 * column says so itself. Both keys point at one function instead of a copy of the format: the
 * aggregate is in the same unit as the value of a single container.
 */
function renderCpu({ getValue }: CellContext<ContainerRecord, unknown>) {
	const val = getValue() as number
	return <span className="ms-1 tabular-nums">{`${decimalString(val, val >= 10 ? 1 : 2)}%`}</span>
}

function renderMemory({ getValue }: CellContext<ContainerRecord, unknown>) {
	const formatted = formatBytes(getValue() as number, false, undefined, true)
	return (
		<span className="ms-1 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</span>
	)
}

function renderNet({ getValue }: CellContext<ContainerRecord, unknown>) {
	const formatted = formatBytes(getValue() as number, true, undefined, false)
	return (
		<div className="ms-1 tabular-nums">{`${decimalString(formatted.value, formatted.value >= 10 ? 1 : 2)} ${formatted.unit}`}</div>
	)
}

function renderHealth({ getValue }: CellContext<ContainerRecord, unknown>) {
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
}

function renderUpdated({ getValue }: CellContext<ContainerRecord, unknown>) {
	const timestamp = getValue() as number
	return <span className="ms-1 tabular-nums">{hourWithSeconds(new Date(timestamp).toISOString())}</span>
}

/**
 * Stack or service of a single container, and the same value on the group row of that level.
 * A container inside a group leaves the cell empty: the group row above already carries the
 * label, and repeating it on every replica only adds noise.
 */
function renderGroupValue({ getValue, row }: CellContext<ContainerRecord, unknown>) {
	if (!row.getIsGrouped() && row.depth > 0) {
		return null
	}
	const value = getValue() as string
	if (!value) {
		return <div className="ms-1.5 text-muted-foreground">-</div>
	}
	return (
		<span className="ms-1 xl:w-32 block truncate" title={value}>
			{value}
		</span>
	)
}

/** Name of the node a container, or a whole service row, runs on */
function NodeNameCell({ value }: { value: string }) {
	const allSystems = useStore($allSystemsById)
	return <div className="ms-1 max-w-40 truncate">{allSystems[value]?.name ?? ""}</div>
}

/** Placeholder for a column that cannot be summed up over a group */
function NoAggregate() {
	return <div className="ms-1.5 text-muted-foreground">-</div>
}

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
