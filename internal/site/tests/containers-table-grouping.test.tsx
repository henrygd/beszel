import { describe, expect, mock, test } from "bun:test"
import type { ReactElement, ReactNode } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import type { CellContext, ColumnDef, Row, Table, TableState, Updater } from "@tanstack/react-table"
import type { ContainerRecord } from "../src/types"

mock.module("@lingui/core/macro", () => ({
	t: (strings: string | TemplateStringsArray | { message?: string }, ...values: unknown[]) => {
		if (typeof strings === "string") return strings
		// a tagged template has to be put back together: the node summary interpolates the service
		// and the node names into its messages, and dropping them would hide what the row claims
		if (Array.isArray(strings)) {
			return strings.reduce<string>(
				(text, part, index) => text + part + (index < values.length ? String(values[index]) : ""),
				""
			)
		}
		return (strings as { message?: string }).message ?? ""
	},
	plural: (_count: number, forms: { other?: string }) => forms.other ?? "",
}))
mock.module("@lingui/react/macro", () => ({
	Trans: ({ children }: { children?: ReactNode }) => children ?? null,
	Plural: ({ value, one, other }: { value: number; one: string; other: string }) =>
		(value === 1 ? one : other).replace("#", String(value)),
}))

const {
	createTable,
	getCoreRowModel,
	getExpandedRowModel,
	getFilteredRowModel,
	getPaginationRowModel,
	getSortedRowModel,
} = await import("@tanstack/react-table")
const { containerChartCols } = await import("../src/components/containers-table/containers-table-columns")
const { ContainerGroupRow } = await import("../src/components/containers-table/container-group-row")
const { getContainerGroupedRowModel } = await import("../src/components/containers-table/containers-grouped-row-model")
const {
	buildGroupingState,
	getServiceGroupingValue,
	getStackGroupingValue,
	groupRowId,
	hasStacks,
	summarizeStackNodes,
} = await import("../src/lib/container-grouping")
const { StackNodesMatrix } = await import("../src/components/containers-table/stack-nodes-cell")
const { $allSystemsById } = await import("../src/lib/stores")

const updatedAt = 1_760_000_000_000

// records as the table receives them, without the pocketbase fields nothing here reads
const data = [
	{
		id: "c1",
		system: "n1",
		name: "web.1",
		stack: "mentors",
		service: "web",
		image: "web:1",
		ports: "",
		cpu: 1,
		memory: 100,
		net: 0.5,
		health: 2,
		status: "Up 2 hours",
		updated: updatedAt,
	},
	{
		id: "c2",
		system: "n1",
		name: "web.2",
		stack: "mentors",
		service: "web",
		image: "web:1",
		ports: "",
		cpu: 2,
		memory: 200,
		net: 0.25,
		health: 3,
		status: "Up 1 hour",
		updated: updatedAt - 1000,
	},
	{
		id: "c3",
		system: "n1",
		name: "db.1",
		stack: "mentors",
		service: "db",
		image: "db:16",
		ports: "",
		cpu: 8,
		memory: 300,
		net: 1,
		health: 0,
		status: "Up 4 hours",
		updated: updatedAt - 2000,
	},
	{
		id: "c4",
		system: "n2",
		name: "web.3",
		stack: "mentors",
		service: "web",
		image: "web:1",
		ports: "",
		cpu: 4,
		memory: 50,
		net: 2,
		health: 2,
		status: "Up 3 hours",
		updated: updatedAt - 3000,
	},
	{
		id: "c5",
		system: "n1",
		name: "manual",
		image: "alpine",
		ports: "",
		cpu: 16,
		memory: 10,
		net: 0,
		health: 0,
		status: "Up 5 hours",
		updated: updatedAt - 4000,
	},
] as unknown as ContainerRecord[]

$allSystemsById.set({
	n1: { name: "node-1", status: "up" },
	n2: { name: "node-2", status: "up" },
	n3: { name: "node-3", status: "up" },
} as unknown as ReturnType<typeof $allSystemsById.get>)

const nodeName = (node: string) => $allSystemsById.get()[node]?.name ?? node

/** The table of the containers page, with the same options the component passes */
function makeTable(
	rows: ContainerRecord[],
	columns: ColumnDef<ContainerRecord>[] = containerChartCols
): Table<ContainerRecord> {
	let state: TableState
	const table = createTable<ContainerRecord>({
		data: rows,
		columns,
		// the summary is built from the full data, the same way the page builds it
		meta: { stackSummaries: summarizeStackNodes(rows, nodeName) },
		state: {} as TableState,
		onStateChange: (updater: Updater<TableState>) => {
			state = typeof updater === "function" ? updater(state) : updater
			table.setOptions((prev) => ({ ...prev, state: { ...state } }))
		},
		renderFallbackValue: null,
		getCoreRowModel: getCoreRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		getGroupedRowModel: getContainerGroupedRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getExpandedRowModel: getExpandedRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		autoResetPageIndex: false,
		autoResetExpanded: false,
		groupedColumnMode: false,
		paginateExpandedRows: false,
		defaultColumn: { sortUndefined: "last", size: 100, minSize: 0 },
	})
	state = table.initialState
	table.setOptions((prev) => ({ ...prev, state: { ...state } }))
	table.setPageSize(100)
	table.setSorting([{ id: "name", desc: false }])
	table.setGrouping(buildGroupingState({ hasStacks: hasStacks(rows), enabled: true }))
	return table
}

const stackId = groupRowId("stack", getStackGroupingValue(data[0]))
const serviceId = groupRowId("service", getServiceGroupingValue(data[0]), stackId)

const visible = (table: Table<ContainerRecord>) =>
	table
		.getRowModel()
		.rows.map((row) => `${row.getIsGrouped() ? `group ${row.id}` : `leaf ${row.original.name}`}@${row.depth}`)

const findRow = (table: Table<ContainerRecord>, id: string) => {
	const row = table.getRowModel().rows.find((candidate) => candidate.id === id)
	if (!row) throw new Error(`row ${id} is not visible`)
	return row
}

const groupRowHtml = (row: Row<ContainerRecord>) => renderToStaticMarkup(<ContainerGroupRow row={row} />)

/** Cells the group row actually drew, in order */
const groupRowCells = (row: Row<ContainerRecord>) =>
	[...groupRowHtml(row).matchAll(/<td\b/g)].length

/** Render one cell of a leaf row through the `cell` of its column */
const leafCellHtml = (row: Row<ContainerRecord>, columnId: string) => {
	const cell = row.getVisibleCells().find((candidate) => candidate.column.id === columnId)
	if (!cell) throw new Error(`no ${columnId} cell`)
	const render = cell.column.columnDef.cell as (context: CellContext<ContainerRecord, unknown>) => ReactNode
	return renderToStaticMarkup(render(cell.getContext()) as ReactElement)
}

describe("group rows keep the formatting of their column", () => {
	test("a group row draws the columns the table has visible, not the set it drew before", () => {
		// what the maintainer of the upstream project hit: turning a column on under Visible
		// Fields left the group rows on the previous set of cells, so their values sat under the
		// wrong headers until the next poll replaced the rows. The row has to follow the table.
		const table = makeTable(data)
		const visibleColumns = () => table.getVisibleLeafColumns().length
		const before = visibleColumns()
		expect(groupRowCells(findRow(table, stackId))).toBe(before)

		table.getColumn("service")?.toggleVisibility(false)
		const after = visibleColumns()
		expect(after).toBeLessThan(before)
		expect(groupRowCells(findRow(table, stackId))).toBe(after)

		table.getColumn("service")?.toggleVisibility(true)
		expect(visibleColumns()).toBe(before)
		expect(groupRowCells(findRow(table, stackId))).toBe(before)
	})

	test("no column falls back to the aggregated cell the library installs", () => {
		const table = makeTable(data)
		// the grouping feature gives every column a default aggregatedCell that prints
		// `getValue()?.toString()`. A column that does not declare one of its own silently loses
		// its formatter on the group row, and `aggregatedCell ?? cell` never reaches the fallback.
		const libraryDefault = (
			table as unknown as { _getDefaultColumnDef: () => { aggregatedCell?: unknown } }
		)._getDefaultColumnDef().aggregatedCell
		expect(typeof libraryDefault).toBe("function")
		for (const column of table.getAllColumns()) {
			expect(column.columnDef.aggregatedCell).toBeDefined()
			expect(column.columnDef.aggregatedCell).not.toBe(libraryDefault)
		}
		for (const columnDef of containerChartCols) {
			expect(typeof columnDef.aggregatedCell).toBe("function")
		}
	})

	test("the stack row shows formatted aggregates, not raw values", () => {
		const table = makeTable(data)
		table.setExpanded({ [stackId]: true })
		const html = groupRowHtml(findRow(table, stackId))

		// label and number of containers
		expect(html).toContain("mentors")
		expect(html).toContain("4 containers")
		// cpu 1 + 2 + 8 + 4 = 15, with the percent sign of the column instead of a bare number
		expect(html).toContain("%")
		expect(html).not.toContain(">15<")
		// memory and net go through formatBytes, so they carry a unit
		expect(html).toMatch(/\d\s(B|KB|MB|GB|TB)/)
		expect(html).not.toContain(">650<")
		// the worst health is the badge of the column, not the number of the enum
		expect(html).toContain("Unhealthy")
		expect(html).not.toContain(">3<")
		// the freshest update is a time, not an epoch in milliseconds
		expect(html).not.toContain(String(updatedAt))
		// a stack over several nodes counts them instead of naming one of them
		expect(html).toContain("2 nodes")
		expect(html).not.toContain("Multiple nodes")
		expect(html).not.toContain("node-1")
	})

	test("the service row shows its own sums", () => {
		const table = makeTable(data)
		table.setExpanded({ [stackId]: true, [serviceId]: true })
		const html = groupRowHtml(findRow(table, serviceId))
		// the two web replicas on n1, not the whole stack
		expect(html).toContain("web")
		expect(html).toContain("2 containers")
		expect(html).toContain("node-1")
		expect(html).toContain("Unhealthy")
		expect(html).toContain("%")
	})

	test("the chevron of a collapsed group points at its row and says so", () => {
		const table = makeTable(data)
		const collapsed = groupRowHtml(findRow(table, stackId))
		expect(collapsed).toContain('aria-expanded="false"')
		expect(collapsed).toContain('data-expanded="false"')
		expect(collapsed).toContain("-rotate-90")
		table.setExpanded({ [stackId]: true })
		const expanded = groupRowHtml(findRow(table, stackId))
		expect(expanded).toContain('aria-expanded="true"')
		expect(expanded).toContain('data-expanded="true"')
		expect(expanded).not.toContain("-rotate-90")
	})
})

describe("saved expanded state", () => {
	test("the ids built from groupRowId are the ids of the rows", () => {
		const table = makeTable(data)
		expect(visible(table)).toEqual(["leaf manual@0", `group ${stackId}@0`])

		// what sessionStorage holds after a click on the stack
		table.setExpanded({ [stackId]: true })
		expect(visible(table)).toEqual([
			"leaf manual@0",
			`group ${stackId}@0`,
			"leaf db.1@1",
			`group ${serviceId}@1`,
			"leaf web.3@1",
		])

		// and after a click on the service inside it
		table.setExpanded({ [stackId]: true, [serviceId]: true })
		expect(visible(table)).toEqual([
			"leaf manual@0",
			`group ${stackId}@0`,
			"leaf db.1@1",
			`group ${serviceId}@1`,
			"leaf web.1@2",
			"leaf web.2@2",
			"leaf web.3@1",
		])

		// a state that only names the service keeps it closed: its parent is collapsed
		table.setExpanded({ [serviceId]: true })
		expect(visible(table)).toEqual(["leaf manual@0", `group ${stackId}@0`])
	})

	test("new data does not change the ids, so the state still applies", () => {
		const table = makeTable(data)
		table.setExpanded({ [stackId]: true, [serviceId]: true })
		const before = visible(table)
		// the next poll brings the same containers with new metrics, in another order
		const next = [...data]
			.reverse()
			.map((record) => ({ ...record, cpu: record.cpu * 3, updated: record.updated + 60_000 }))
		table.setOptions((prev) => ({ ...prev, data: next }))
		expect(visible(table)).toEqual(before)
	})
})

describe("rows below a group", () => {
	test("a container inside a group is indented and does not repeat the stack", () => {
		const table = makeTable(data)
		table.setExpanded({ [stackId]: true, [serviceId]: true })
		const rows = table.getRowModel().rows.filter((row) => !row.getIsGrouped())
		const replica = rows.find((row) => row.original.name === "web.1")
		const lonely = rows.find((row) => row.original.name === "manual")
		if (!replica || !lonely) throw new Error("rows are missing")
		expect(replica.depth).toBe(2)
		expect(lonely.depth).toBe(0)

		// the replica is indented by its depth, the container that stands on its own is not
		expect(leafCellHtml(replica, "name")).toContain("padding-inline-start:36px")
		expect(leafCellHtml(lonely, "name")).not.toContain("padding-inline-start:36px")

		// the group row above already carries the stack, so the replica leaves the cell empty
		expect(leafCellHtml(replica, "stack")).toBe("")
		// the container without labels keeps its own cell, with the dash of the column
		expect(leafCellHtml(lonely, "stack")).toContain("-")
	})

	test("without stacks the table is exactly the flat list it is today", () => {
		const plain = data.map((record) => ({ ...record, stack: undefined, service: undefined }))
		const table = makeTable(plain)
		expect(hasStacks(plain)).toBe(false)
		expect(table.getState().grouping).toEqual([])
		expect(visible(table)).toEqual(["leaf db.1@0", "leaf manual@0", "leaf web.1@0", "leaf web.2@0", "leaf web.3@0"])
		expect(table.getRowModel().rows.every((row) => !row.getIsGrouped())).toBe(true)
		expect(leafCellHtml(table.getRowModel().rows[0], "name")).not.toContain("padding-inline-start")
	})
})

describe("stack row over nodes", () => {
	// the same stack over three nodes: n3 runs web only, so db is absent from exactly one node
	const onNode = (id: string, name: string, system: string, service: string) =>
		({ ...data[0], id, name, system, service }) as ContainerRecord
	const spread = [
		data[0],
		data[1],
		data[2],
		onNode("c6", "web.3", "n2", "web"),
		onNode("c7", "db.2", "n2", "db"),
		onNode("c8", "web.4", "n3", "web"),
		onNode("c9", "web.5", "n3", "web"),
		data[4],
	]
	const summaryOf = (rows: ContainerRecord[]) => {
		const summary = summarizeStackNodes(rows, nodeName).get("mentors")
		if (!summary) throw new Error("no summary for mentors")
		return summary
	}

	test('the stack row counts its nodes instead of saying "multiple"', () => {
		const table = makeTable(spread)
		const html = groupRowHtml(findRow(table, stackId))
		expect(html).toContain("3 nodes")
		expect(html).not.toContain("Multiple nodes")
		// the service row is on one node and still names it
		table.setExpanded({ [stackId]: true, [serviceId]: true })
		expect(groupRowHtml(findRow(table, serviceId))).toContain("node-1")
	})

	test("a service absent on a live node is flagged on the stack row", () => {
		const table = makeTable(spread)
		const html = groupRowHtml(findRow(table, stackId))
		expect(html).toContain("No containers of db on node-3")
		expect(html).not.toContain("node-1")

		// the same node paused says nothing about the service instead of accusing it
		$allSystemsById.setKey("n3", { name: "node-3", status: "paused" } as unknown as ReturnType<
			typeof $allSystemsById.get
		>["n3"])
		const paused = groupRowHtml(findRow(makeTable(spread), stackId))
		expect(paused).toContain("No data from node-3")
		expect(paused).toContain("Node is paused")
		expect(paused).not.toContain("No containers of")
		$allSystemsById.setKey("n3", { name: "node-3", status: "up" } as unknown as ReturnType<
			typeof $allSystemsById.get
		>["n3"])
	})

	test("a stack on one node says so and is not flagged", () => {
		const alone = spread.filter((record) => record.system === "n1")
		const html = groupRowHtml(findRow(makeTable(alone), stackId))
		expect(html).toContain("1 node")
		expect(html).not.toContain("No containers of")
		expect(html).not.toContain("No data from")
	})

	test("without the system column there is no node summary", () => {
		// the page of a single system drops the column, and the stack row must not grow a summary
		const table = makeTable(
			spread,
			containerChartCols.filter((column) => column.id !== "system")
		)
		const html = groupRowHtml(findRow(table, stackId))
		expect(html).toContain("mentors")
		expect(html).not.toContain("nodes")
		expect(html).not.toContain("No containers of")
	})

	test("a filter changes which rows are listed, not what the stack row says", () => {
		// the summary answers "where does this stack run", so it is built from the whole data set.
		// Reading it off the visible rows instead would quietly disagree with the matrix.
		const table = makeTable(spread)
		table.setGlobalFilter("db")
		const listed = visible(table)
		expect(listed.some((entry) => entry.includes("web"))).toBe(false)
		const html = groupRowHtml(findRow(table, stackId))
		expect(html).toContain("3 nodes")
		expect(html).toContain("No containers of")
	})

	test("a stack row without its summary stays empty instead of counting the visible rows", () => {
		// same table, but the page passed no summary at all
		const table = makeTable(spread)
		table.setOptions((prev) => ({ ...prev, meta: {} }))
		const html = groupRowHtml(findRow(table, stackId))
		expect(html).toContain("mentors")
		expect(html).not.toContain("nodes")
	})

	test("the matrix of the tooltip counts every service on every node", () => {
		const html = renderToStaticMarkup(<StackNodesMatrix summary={summaryOf(spread)} nodeLabel={nodeName} />)
		for (const name of ["node-1", "node-2", "node-3", "db", "web"]) {
			expect(html).toContain(name)
		}
		// db runs on two of the three nodes, web on all three, and the empty pair reads as a dash
		expect(html).toContain("7 containers")
		expect(html).toContain("—")
		// two body rows, then the totals per node of the footer
		const cells = [...html.matchAll(/<td[^>]*>([^<]*)<\/td>/g)].map((match) => match[1])
		expect(cells).toEqual(["db", "1", "1", "—", "web", "2", "1", "2", "3", "2", "2"])
	})
})
