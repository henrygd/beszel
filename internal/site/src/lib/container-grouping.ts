/**
 * Pure grouping and aggregation helpers for containers.
 *
 * No React and no runtime import of @tanstack/react-table: the table only adds the TanStack
 * wiring on top (grouped row model, group row, column aggregations). Everything here works on
 * structural types, so the container table (ContainerRecord) and the charts (container stats,
 * where the same fields are stored as `st` / `sv`) can share it, and it can be lifted into a
 * primitive the other data tables share without touching the table itself.
 */
import { ContainerHealth } from "@/lib/enums"

/** Column id used for the first grouping level */
export const STACK_COLUMN_ID = "stack"
/** Column id used for the second grouping level */
export const SERVICE_COLUMN_ID = "service"

/**
 * Prefix of the grouping value given to records without stack or service labels.
 * TanStack builds a group key from `String(groupingValue)`, so every unlabeled record needs its
 * own value, otherwise they would all end up in a single "undefined" group. The wrapper groups
 * these values create are then removed again by {@link unwrapGroups}.
 */
export const UNGROUPED = "\u0000ungrouped:"

/** Separator between the node and the service name inside a service grouping value */
const SERVICE_KEY_SEPARATOR = "\u0000"

/** Aggregated value of the system column when a stack spans more than one node */
export const MULTIPLE_NODES = "\u0000nodes:multiple"

/**
 * Anything that carries the container stack and service labels: a container record from the
 * `containers` collection (`stack` / `service`) or an entry of the stats history (`st` / `sv`).
 */
export interface ContainerGroupSource {
	id?: string
	/** container name (records) */
	name?: string
	/** container name (stats entries) */
	n?: string
	/** node the container runs on */
	system?: string
	/** docker stack or compose project (records) */
	stack?: string
	/** docker stack or compose project (stats entries) */
	st?: string
	/** service within the stack (records) */
	service?: string
	/** service within the stack (stats entries) */
	sv?: string
}

/** Stack of a container, empty when it has no stack labels */
export function getStackValue(source: ContainerGroupSource): string {
	return source.stack ?? source.st ?? ""
}

/** Service of a container, empty when it has no service labels */
export function getServiceValue(source: ContainerGroupSource): string {
	return source.service ?? source.sv ?? ""
}

/** Node the container runs on */
export function getNodeValue(source: ContainerGroupSource): string {
	return source.system ?? ""
}

/** Identity of a container inside its node, used to keep sentinel grouping values unique */
export function getSourceKey(source: ContainerGroupSource): string {
	return source.id ?? source.name ?? source.n ?? ""
}

/** Grouping value for the stack level: the stack, or a unique sentinel when there is none */
export function getStackGroupingValue(source: ContainerGroupSource): string {
	return getStackValue(source) || `${UNGROUPED}${getNodeValue(source)}/${getSourceKey(source)}`
}

/**
 * Grouping value for the service level. The node is part of the key, so replicas of one service
 * are only folded together per node — a stack is shared by the whole swarm, a service row is not.
 */
export function getServiceGroupingValue(source: ContainerGroupSource): string {
	const service = getServiceValue(source)
	if (!service) {
		return `${UNGROUPED}${getNodeValue(source)}/${getSourceKey(source)}`
	}
	return `${getNodeValue(source)}${SERVICE_KEY_SEPARATOR}${service}`
}

/** Service name of a service grouping value, without the node prefix */
export function serviceNameFromGroupingValue(value: unknown): string {
	const key = `${value ?? ""}`
	const separator = key.indexOf(SERVICE_KEY_SEPARATOR)
	return separator === -1 ? key : key.slice(separator + SERVICE_KEY_SEPARATOR.length)
}

/** True for a grouping value that only wraps a record without labels */
export function isUngroupedValue(value: unknown): boolean {
	return `${value ?? ""}`.startsWith(UNGROUPED)
}

/** True when at least one container reports a stack, i.e. grouping is worth showing */
export function hasStacks(sources: ContainerGroupSource[]): boolean {
	return sources.some((source) => Boolean(getStackValue(source)))
}

/** Grouping state for the table: stack then service, and nothing at all without stacks */
export function buildGroupingState({ hasStacks: stacks, enabled }: { hasStacks: boolean; enabled: boolean }): string[] {
	return stacks && enabled ? [STACK_COLUMN_ID, SERVICE_COLUMN_ID] : []
}

/**
 * Id TanStack gives to a group row. Only the grouping value and the parent take part, so the id
 * survives polling and a page reload — which is what makes the saved expanded state usable.
 */
export function groupRowId(columnId: string, groupingValue: unknown, parentId?: string): string {
	const id = `${columnId}:${groupingValue}`
	return parentId ? `${parentId}>${id}` : id
}

/**
 * How bad each health value is, highest wins. The numeric order of the ContainerHealth enum is
 * not the order of severity, and a container without a healthcheck is weaker than a healthy one:
 * a group reports None only when none of its containers has a healthcheck.
 */
export const HEALTH_SEVERITY: Record<ContainerHealth, number> = {
	[ContainerHealth.Unhealthy]: 3,
	[ContainerHealth.Starting]: 2,
	[ContainerHealth.Healthy]: 1,
	[ContainerHealth.None]: 0,
}

/** Worst health of a group: Unhealthy > Starting > Healthy > None */
export function worstHealth(values: Array<ContainerHealth | number | undefined>): ContainerHealth {
	let worst = ContainerHealth.None
	let worstSeverity = -1
	for (const value of values) {
		const health = (value ?? ContainerHealth.None) as ContainerHealth
		const severity = HEALTH_SEVERITY[health] ?? 0
		if (severity > worstSeverity) {
			worstSeverity = severity
			worst = health
		}
	}
	return worst
}

/** Sum of the numbers of a group, ignoring values that are not numbers */
export function sumValues(values: Array<number | undefined>): number {
	let sum = 0
	for (const value of values) {
		if (typeof value === "number" && Number.isFinite(value)) {
			sum += value
		}
	}
	return sum
}

/** Number of containers in a group */
export function countLeaves(leafRows: unknown[]): number {
	return leafRows.length
}

/** The value shared by every row of a group, or undefined when they differ */
export function sharedValue<T>(values: T[]): T | undefined {
	if (values.length === 0) {
		return undefined
	}
	const first = values[0]
	return values.every((value) => value === first) ? first : undefined
}

/** Minimal shape of a row that can be grouped. Row<TData> of TanStack satisfies it. */
export interface GroupNodeLike<TRow> {
	id: string
	depth: number
	parentId?: string
	subRows: TRow[]
	groupingColumnId?: string
	groupingValue?: unknown
}

/** Minimal shape of a TanStack row model */
export interface RowModelLike<TRow> {
	rows: TRow[]
	flatRows: TRow[]
	rowsById: Record<string, TRow>
}

/** Decides whether a group row is only a wrapper and should be replaced by its children */
export type GroupUnwrapPredicate = (row: {
	groupingColumnId?: string
	groupingValue?: unknown
	subRows: unknown[]
}) => boolean

/**
 * Default predicate for the container table. A group is unwrapped when
 * - its grouping value is a sentinel, i.e. the container has no labels for this level, or
 * - it is a service group with a single container: one replica should not add a level.
 */
export const shouldUnwrapContainerGroup: GroupUnwrapPredicate = (row) => {
	if (!row.groupingColumnId) {
		return false
	}
	if (isUngroupedValue(row.groupingValue)) {
		return true
	}
	return row.groupingColumnId === SERVICE_COLUMN_ID && row.subRows.length < 2
}

/**
 * Post-process a grouped row model: drop wrapper groups and lift their children to the level of
 * the group that held them, then rebuild depth, parentId, flatRows and rowsById. The model is
 * returned unchanged when there is nothing to unwrap.
 */
export function unwrapGroups<TRow extends GroupNodeLike<TRow>>(
	model: RowModelLike<TRow>,
	shouldUnwrap: GroupUnwrapPredicate = shouldUnwrapContainerGroup
): RowModelLike<TRow> {
	const rows = unwrapRows(model.rows, shouldUnwrap)
	if (rows === model.rows) {
		return model
	}
	const flatRows: TRow[] = []
	const rowsById: Record<string, TRow> = {}
	relevelRows(rows, 0, undefined, flatRows, rowsById)
	return { rows, flatRows, rowsById }
}

function unwrapRows<TRow extends GroupNodeLike<TRow>>(rows: TRow[], shouldUnwrap: GroupUnwrapPredicate): TRow[] {
	let changed = false
	const unwrapped: TRow[] = []
	for (const row of rows) {
		if (!row.groupingColumnId) {
			unwrapped.push(row)
			continue
		}
		const subRows = unwrapRows(row.subRows, shouldUnwrap)
		if (subRows !== row.subRows) {
			row.subRows = subRows
			changed = true
		}
		if (shouldUnwrap(row)) {
			unwrapped.push(...subRows)
			changed = true
		} else {
			unwrapped.push(row)
		}
	}
	return changed ? unwrapped : rows
}

function relevelRows<TRow extends GroupNodeLike<TRow>>(
	rows: TRow[],
	depth: number,
	parentId: string | undefined,
	flatRows: TRow[],
	rowsById: Record<string, TRow>
) {
	for (const row of rows) {
		row.depth = depth
		row.parentId = parentId
		flatRows.push(row)
		rowsById[row.id] = row
		if (row.subRows?.length) {
			relevelRows(row.subRows, depth + 1, row.id, flatRows, rowsById)
		}
	}
}

/** Minimal shape of a row the comparators need */
export interface ComparableRow {
	getIsGrouped: () => boolean
	groupingColumnId?: string
	groupingValue?: unknown
	getValue: <TValue>(columnId: string) => TValue
}

/** Label shown for a group row: the stack name, or the service name without the node prefix */
export function rowGroupLabel(row: { groupingColumnId?: string; groupingValue?: unknown }): string {
	if (!row.groupingColumnId || isUngroupedValue(row.groupingValue)) {
		return ""
	}
	const value = `${row.groupingValue ?? ""}`
	return row.groupingColumnId === SERVICE_COLUMN_ID ? serviceNameFromGroupingValue(value) : value
}

/** Compare two values as strings, in the order of the current locale */
export function compareString(a: unknown, b: unknown): number {
	return `${a ?? ""}`.localeCompare(`${b ?? ""}`)
}

/**
 * Text of a row for sorting: group rows use their group label, leaf rows the cell value.
 * A group row carries the record of its first child as `original` and an aggregate as its value,
 * so neither of those may be used here.
 */
function groupAwareText(row: ComparableRow, columnId: string): string {
	return row.getIsGrouped() ? rowGroupLabel(row) : `${row.getValue<unknown>(columnId) ?? ""}`
}

/** Sort group rows by their group label and leaf rows by the column value */
export function compareGroupAwareName(rowA: ComparableRow, rowB: ComparableRow, columnId: string): number {
	return compareString(groupAwareText(rowA, columnId), groupAwareText(rowB, columnId))
}

/** Sort by resolved system name, then by the name column, group aware on both */
export function makeSystemComparator(resolveSystemName: (value: unknown) => string) {
	return (rowA: ComparableRow, rowB: ComparableRow, columnId: string): number => {
		const primary = compareString(
			resolveSystemName(rowA.getValue<unknown>(columnId)),
			resolveSystemName(rowB.getValue<unknown>(columnId))
		)
		if (primary !== 0) {
			return primary
		}
		return compareGroupAwareName(rowA, rowB, "name")
	}
}

// Unit names and their number of seconds, for converting docker status strings
const unitSeconds = [
	["s", 1],
	["mi", 60],
	["h", 3600],
	["d", 86400],
	["w", 604800],
	["mo", 2592000],
] as const

/** Convert a docker status string to seconds ("Up X minutes", "Up X hours", etc.) */
export function statusSortValue(status: unknown): number {
	const [_, num, unit] = `${status ?? ""}`.split(" ")
	if (!unit) {
		return 0
	}
	// Docker uses "a" or "an" instead of "1" for singular units (e.g. "Up a minute", "Up an hour")
	const numValue = num === "a" || num === "an" ? 1 : Number(num)
	for (const [unitName, value] of unitSeconds) {
		if (unit.startsWith(unitName)) {
			return numValue * value
		}
	}
	return 0
}

/** Sort by container uptime. Group rows have no status and are kept out by sortUndefined. */
export function compareStatus(rowA: ComparableRow, rowB: ComparableRow, columnId: string): number {
	return statusSortValue(rowA.getValue<unknown>(columnId)) - statusSortValue(rowB.getValue<unknown>(columnId))
}

/**
 * Convert a port string to a number for sorting.
 * Handles formats like "80", "127.0.0.1:80", and "80, 443" (takes the first mapping).
 */
export function portSortValue(ports: unknown): number {
	if (!ports || typeof ports !== "string") {
		return 0
	}
	const first = ports.includes(",") ? ports.substring(0, ports.indexOf(",")) : ports
	const colonIndex = first.lastIndexOf(":")
	const portStr = colonIndex === -1 ? first : first.substring(colonIndex + 1)
	return Number(portStr) || 0
}

/** Sort by first published port */
export function comparePorts(rowA: ComparableRow, rowB: ComparableRow, columnId: string): number {
	return portSortValue(rowA.getValue<unknown>(columnId)) - portSortValue(rowB.getValue<unknown>(columnId))
}

/**
 * Coverage of one service of a stack over the nodes the stack runs on.
 * `countByNode` only holds the nodes that actually run a container of the service; the nodes of
 * the stack that do not are listed in `absentOn`, whatever their status is.
 */
export interface StackServiceCoverage {
	service: string
	/** node → containers of this service on it */
	countByNode: Record<string, number>
	total: number
	/** nodes of the stack without a container of this service */
	absentOn: string[]
}

/**
 * Where a stack lives: the nodes that run it, how many containers each of them runs, and the
 * coverage of every named service. A container with a stack label and no service label counts
 * on its node and in the total, but it is not a service of the stack: in the table it is a row
 * of its own, not a replica, so giving it a coverage row would invent a service.
 */
export interface StackNodeSummary {
	stack: string
	/** ids of the systems with at least one container of the stack, in the order of their names */
	nodes: string[]
	containersByNode: Record<string, number>
	/** by service name */
	services: StackServiceCoverage[]
	/** containers of the stack, equal to the sum over `containersByNode` */
	total: number
}

/** Nodes a service is absent from, split by whether the node can be asked at all */
export interface StackServiceGap {
	service: string
	/** the node is up and runs the stack, but not this service */
	missingOn: string[]
	/** the node is not up, so its containers say nothing about the service */
	unknownOn: string[]
}

/**
 * Build the node summary of every stack in one pass.
 *
 * Records without a stack label are skipped: the table shows them as rows of their own, and a
 * summary of them would be a summary of unrelated containers. The expected nodes of a stack are
 * the nodes it currently runs on — the agent reports the stack and service labels of a running
 * container and nothing about desired replicas, so a node that stopped reporting simply leaves
 * the set instead of turning into a ghost.
 *
 * Statuses are deliberately not read here: a system is paused or goes down independently of the
 * container records, and the cell applies the current status on top (see {@link stackServiceGaps}).
 *
 * @param nodeName resolves a system id to its name, used to order the nodes
 */
export function summarizeStackNodes(
	sources: ContainerGroupSource[],
	nodeName: (node: string) => string = (node) => node
): Map<string, StackNodeSummary> {
	const summaries = new Map<string, StackNodeSummary>()
	const coverages = new Map<string, Map<string, StackServiceCoverage>>()
	for (const source of sources) {
		const stack = getStackValue(source)
		if (!stack) {
			continue
		}
		let summary = summaries.get(stack)
		let byService = coverages.get(stack)
		if (!summary || !byService) {
			summary = { stack, nodes: [], containersByNode: {}, services: [], total: 0 }
			byService = new Map<string, StackServiceCoverage>()
			summaries.set(stack, summary)
			coverages.set(stack, byService)
		}
		const node = getNodeValue(source)
		summary.containersByNode[node] = (summary.containersByNode[node] ?? 0) + 1
		summary.total++
		const service = getServiceValue(source)
		if (!service) {
			continue
		}
		let coverage = byService.get(service)
		if (!coverage) {
			coverage = { service, countByNode: {}, total: 0, absentOn: [] }
			byService.set(service, coverage)
		}
		coverage.countByNode[node] = (coverage.countByNode[node] ?? 0) + 1
		coverage.total++
	}
	for (const summary of summaries.values()) {
		// by name, so the columns of the matrix do not jump around between polls; the id breaks
		// the tie, so two systems with the same name still get a stable order
		summary.nodes = Object.keys(summary.containersByNode).sort(
			(a, b) => compareString(nodeName(a), nodeName(b)) || compareString(a, b)
		)
		summary.services = [...(coverages.get(summary.stack)?.values() ?? [])].sort((a, b) =>
			compareString(a.service, b.service)
		)
		for (const coverage of summary.services) {
			coverage.absentOn = summary.nodes.filter((node) => !(node in coverage.countByNode))
		}
	}
	return summaries
}

/**
 * Split the nodes a service is absent from: a node that is up says the service is not running on
 * it, anything else — down, paused, pending, or a system that is not in the store at all — says
 * nothing at all and must not be reported as a dead service.
 */
export function splitAbsentNodes(
	absentOn: string[],
	nodeStatus: (node: string) => string | undefined
): { missingOn: string[]; unknownOn: string[] } {
	const missingOn: string[] = []
	const unknownOn: string[] = []
	for (const node of absentOn) {
		if (nodeStatus(node) === "up") {
			missingOn.push(node)
		} else {
			unknownOn.push(node)
		}
	}
	return { missingOn, unknownOn }
}

/** Gaps of a stack: one entry per service that is absent from a node of the stack */
export function stackServiceGaps(
	summary: StackNodeSummary,
	nodeStatus: (node: string) => string | undefined
): StackServiceGap[] {
	const gaps: StackServiceGap[] = []
	for (const coverage of summary.services) {
		if (coverage.absentOn.length === 0) {
			continue
		}
		const { missingOn, unknownOn } = splitAbsentNodes(coverage.absentOn, nodeStatus)
		gaps.push({ service: coverage.service, missingOn, unknownOn })
	}
	return gaps
}
