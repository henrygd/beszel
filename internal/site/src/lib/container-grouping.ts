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
