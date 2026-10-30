import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import type { Row, Table } from "@tanstack/react-table"
import { CircleDashedIcon, TriangleAlertIcon } from "lucide-react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { rowGroupLabel, type StackNodeSummary, stackServiceGaps } from "@/lib/container-grouping"
import { $allSystemsById } from "@/lib/stores"
import { cn } from "@/lib/utils"
import type { ContainerRecord } from "@/types"

/**
 * System column of a stack row: how many nodes the stack runs on, the matrix "service x node"
 * behind it, and a mark when a service is absent from one of those nodes.
 *
 * The numbers come from the summary the page put on the table, which is built from the full data
 * and not from the filtered rows: "worker is not up on node-3" is a fact about the stack, and it
 * is needed exactly when the filter is narrowed down to that service. The matrix in the tooltip
 * shows the whole picture, so the difference between it and the rows below is visible.
 *
 * No arithmetic lives here. The statuses are the only thing the cell adds, read from the systems
 * store so that pausing a system repaints the row without waiting for the next poll.
 */
export function StackNodesCell({ row, table }: { row: Row<ContainerRecord>; table: Table<ContainerRecord> }) {
	const allSystems = useStore($allSystemsById)
	const summary = table.options.meta?.stackSummaries?.get(rowGroupLabel(row))
	if (!summary) {
		// the table passed no summary: an empty cell is better than a count taken from the visible
		// rows, which would disagree with the matrix as soon as a filter is on
		return null
	}
	const nodeLabel = (node: string) => allSystems[node]?.name ?? node
	const nodeStatus = (node: string) => allSystems[node]?.status
	const lines = stackGapLines(summary, nodeLabel, nodeStatus)
	const missing = lines.some((line) => line.missing)
	const GapIcon = missing ? TriangleAlertIcon : CircleDashedIcon
	return (
		<div className="ms-1 flex items-center gap-1.5 max-w-40">
			<Tooltip>
				<TooltipTrigger
					className="truncate tabular-nums rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
					// the row toggles its group on click, so the trigger must keep the click
					onClick={stopRowToggle}
				>
					<Plural value={summary.nodes.length} one="# node" other="# nodes" />
				</TooltipTrigger>
				<TooltipContent className="p-0">
					<StackNodesMatrix summary={summary} nodeLabel={nodeLabel} />
				</TooltipContent>
			</Tooltip>
			{lines.length > 0 && (
				<Tooltip>
					<TooltipTrigger
						aria-label={lines.map((line) => (line.hint ? `${line.text}. ${line.hint}` : line.text)).join(" ")}
						className={cn(
							"shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
							missing ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground"
						)}
						onClick={stopRowToggle}
					>
						<GapIcon className="size-4" aria-hidden="true" />
					</TooltipTrigger>
					<TooltipContent className="max-w-70">
						<ul className="flex flex-col gap-1">
							{lines.map((line) => (
								<li key={line.text}>
									{line.text}
									{line.hint && <span className="ms-1 text-muted-foreground">{line.hint}</span>}
								</li>
							))}
						</ul>
					</TooltipContent>
				</Tooltip>
			)}
		</div>
	)
}

/**
 * Containers of every service of the stack on every node it runs on, with the totals per node.
 * A pair without a container reads as a dash: the service is simply not there, and a zero would
 * suggest the node reported one and it died.
 */
export function StackNodesMatrix({
	summary,
	nodeLabel,
}: {
	summary: StackNodeSummary
	nodeLabel: (node: string) => string
}) {
	return (
		<div className="px-3 py-2">
			<table className="text-xs tabular-nums border-separate border-spacing-x-2 border-spacing-y-0.5">
				<thead>
					<tr className="text-muted-foreground">
						<th className="text-start font-normal">{t({ message: "Service", context: "Docker Swarm service" })}</th>
						{summary.nodes.map((node) => (
							<th key={node} className="text-end font-normal">
								{nodeLabel(node)}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{summary.services.map((coverage) => (
						<tr key={coverage.service}>
							<td className="text-start">{coverage.service}</td>
							{summary.nodes.map((node) => (
								<td key={node} className="text-end">
									{coverage.countByNode[node] ?? "—"}
								</td>
							))}
						</tr>
					))}
				</tbody>
				<tfoot>
					<tr className="text-muted-foreground">
						<th className="text-start font-normal">
							<Plural value={summary.total} one="# container" other="# containers" />
						</th>
						{summary.nodes.map((node) => (
							<td key={node} className="text-end">
								{summary.containersByNode[node] ?? 0}
							</td>
						))}
					</tr>
				</tfoot>
			</table>
			<p className="mt-2 max-w-60 text-xs text-muted-foreground text-balance">
				<Trans>
					Expected nodes are inferred from where the stack currently runs. Docker Swarm desired replicas are not
					collected.
				</Trans>
			</p>
		</div>
	)
}

/** One line of the gap mark: what is absent, and why the node cannot be asked */
interface StackGapLine {
	text: string
	hint?: string
	/** the node is up, so this is a service that is not running and not a node that is silent */
	missing: boolean
}

/**
 * Lines of the gap mark. A node that is up and runs the stack but not one of its services is
 * named per service; a node that is not up says nothing about any service, so it is reported
 * once, grouped by its status, instead of being repeated under every service of the stack.
 */
function stackGapLines(
	summary: StackNodeSummary,
	nodeLabel: (node: string) => string,
	nodeStatus: (node: string) => string | undefined
): StackGapLine[] {
	const lines: StackGapLine[] = []
	const silent = new Set<string>()
	for (const gap of stackServiceGaps(summary, nodeStatus)) {
		if (gap.missingOn.length) {
			const service = gap.service
			const nodes = gap.missingOn.map(nodeLabel).join(", ")
			lines.push({ text: t`No containers of ${service} on ${nodes}`, missing: true })
		}
		for (const node of gap.unknownOn) {
			silent.add(node)
		}
	}
	const byStatus = new Map<string, string[]>()
	for (const node of summary.nodes) {
		if (silent.has(node)) {
			const status = nodeStatus(node) ?? ""
			byStatus.set(status, [...(byStatus.get(status) ?? []), node])
		}
	}
	for (const [status, silentNodes] of byStatus) {
		const nodes = silentNodes.map(nodeLabel).join(", ")
		lines.push({
			text: t`No data from ${nodes}`,
			hint: status ? t`Node is ${status}` : undefined,
			missing: false,
		})
	}
	return lines
}

function stopRowToggle(event: React.MouseEvent) {
	event.stopPropagation()
}
