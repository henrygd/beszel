import { describe, expect, mock, test } from "bun:test"
import {
	createTable,
	getCoreRowModel,
	getExpandedRowModel,
	getGroupedRowModel,
	getSortedRowModel,
	type SortingState,
} from "@tanstack/react-table"
import type { ContainerRecord } from "../src/types"

mock.module("@lingui/core/macro", () => ({
	t: (strings: any) => (typeof strings === "string" ? strings : (strings?.message ?? strings?.[0] ?? "")),
	plural: (_count: number, forms: any) => forms.other ?? "",
}))

const { containerChartCols } = await import("../src/components/containers-table/containers-table-columns")
const { ContainerHealth } = await import("../src/lib/enums")

const ctr = (id: string, name: string, stack: string, cpu: number, health: number, updated = 1): ContainerRecord =>
	({
		id,
		name,
		stack,
		cpu,
		health,
		updated,
		system: "s1",
		image: "",
		ports: "",
		memory: 0,
		net: 0,
		status: "",
	}) as ContainerRecord

function groupedRows(data: ContainerRecord[], sorting: SortingState = [{ id: "name", desc: false }]) {
	const table = createTable<ContainerRecord>({
		data,
		columns: containerChartCols,
		getCoreRowModel: getCoreRowModel(),
		getGroupedRowModel: getGroupedRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getExpandedRowModel: getExpandedRowModel(),
		onStateChange: () => {},
		renderFallbackValue: null,
		state: { grouping: ["stack"], sorting, expanded: {}, columnPinning: {}, columnVisibility: {} },
	})
	return table.getRowModel().rows
}

describe("containers grouped by stack", () => {
	const data = [
		ctr("a1", "immich-server", "immich", 1.5, ContainerHealth.Healthy, 10),
		ctr("a2", "immich-ml", "immich", 2.5, ContainerHealth.Starting, 30),
		ctr("b1", "zz-solo", "", 3, ContainerHealth.Unhealthy),
		ctr("c1", "aa-solo", "", 1, ContainerHealth.None),
	]

	test("containers without a stack stay apart instead of forming one group", () => {
		const rows = groupedRows(data)
		expect(rows.map((row) => [row.original.stack || row.original.name, row.subRows.length])).toEqual([
			["aa-solo", 1],
			["immich", 2],
			["zz-solo", 1],
		])
	})

	test("stack rows aggregate their containers", () => {
		const stack = groupedRows(data).find((row) => row.original.stack === "immich")
		expect(stack?.getValue("cpu")).toBe(4)
		expect(stack?.getValue("updated")).toBe(30)
		// starting is worse than healthy
		expect(stack?.getValue("health")).toBe(ContainerHealth.Starting)
	})

	test("stacks sort by their totals", () => {
		// cpu inverts sorting, so ascending puts the highest first
		const rows = groupedRows(data, [{ id: "cpu", desc: false }])
		expect(rows.map((row) => row.original.stack || row.original.name)).toEqual(["immich", "zz-solo", "aa-solo"])
	})
})
