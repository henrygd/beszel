import { describe, expect, test } from "bun:test"

import { ContainerHealth } from "../src/lib/enums"
import {
	buildGroupingState,
	compareGroupAwareName,
	comparePorts,
	compareStatus,
	countLeaves,
	getServiceGroupingValue,
	getServiceValue,
	getStackGroupingValue,
	getStackValue,
	groupRowId,
	hasStacks,
	makeSystemComparator,
	rowGroupLabel,
	sharedValue,
	shouldUnwrapContainerGroup,
	sumValues,
	UNGROUPED,
	unwrapGroups,
	worstHealth,
} from "../src/lib/container-grouping"

type FakeRow = {
	id: string
	depth: number
	parentId?: string
	subRows: FakeRow[]
	groupingColumnId?: string
	groupingValue?: unknown
	values: Record<string, unknown>
	getIsGrouped: () => boolean
	getValue: <TValue>(columnId: string) => TValue
}

function leaf(id: string, values: Record<string, unknown> = {}): FakeRow {
	const row: FakeRow = {
		id,
		depth: 0,
		parentId: undefined,
		subRows: [],
		values,
		getIsGrouped: () => false,
		getValue: (<TValue>(columnId: string) => row.values[columnId] as TValue) as FakeRow["getValue"],
	}
	return row
}

function group(columnId: string, groupingValue: string, subRows: FakeRow[], values: Record<string, unknown> = {}) {
	const row: FakeRow = {
		id: groupRowId(columnId, groupingValue),
		depth: 0,
		parentId: undefined,
		subRows,
		groupingColumnId: columnId,
		groupingValue,
		values,
		getIsGrouped: () => true,
		getValue: (<TValue>(columnId_: string) => row.values[columnId_] as TValue) as FakeRow["getValue"],
	}
	return row
}

function model(rows: FakeRow[]) {
	const flatRows: FakeRow[] = []
	const rowsById: Record<string, FakeRow> = {}
	const walk = (list: FakeRow[]) => {
		for (const row of list) {
			flatRows.push(row)
			rowsById[row.id] = row
			walk(row.subRows)
		}
	}
	walk(rows)
	return { rows, flatRows, rowsById }
}

const container = (id: string, overrides: Record<string, unknown> = {}) => ({
	id,
	system: "node1",
	name: id,
	stack: "myapp",
	service: "web",
	...overrides,
})

describe("group aggregates", () => {
	test("sums cpu, memory and net over the containers of a group", () => {
		expect(sumValues([1.5, 2.25, 0.25])).toBe(4)
		expect(sumValues([1.5, 2.25, 0.25].reverse())).toBe(4)
		const memory = [128, 256.5, 64]
		const net = [0.5, 1.25, 0]
		expect(sumValues(memory)).toBe(448.5)
		expect(sumValues(net)).toBe(1.75)
		// values that are missing or not numbers are skipped instead of poisoning the sum
		expect(sumValues([1, undefined, 2])).toBe(3)
	})

	test("the number of containers of a group is the number of its leaves", () => {
		const leaves = ["a", "b", "c", "d", "e"].map((id) => leaf(id))
		// both levels receive the real containers as leafRows, services included
		expect(countLeaves(leaves)).toBe(5)
		expect(countLeaves(leaves.slice(0, 2))).toBe(2)
	})

	test("the worst health is picked by severity, not by the number of the enum", () => {
		expect(worstHealth([ContainerHealth.Healthy, ContainerHealth.Unhealthy])).toBe(ContainerHealth.Unhealthy)
		expect(worstHealth([ContainerHealth.Healthy, ContainerHealth.Starting])).toBe(ContainerHealth.Starting)
		expect(worstHealth([ContainerHealth.Unhealthy, ContainerHealth.Starting])).toBe(ContainerHealth.Unhealthy)
		expect(worstHealth([ContainerHealth.Healthy, ContainerHealth.Healthy])).toBe(ContainerHealth.Healthy)
		// a container without a healthcheck is weaker than a healthy one:
		// a group reports None only when none of its containers has a healthcheck
		expect(worstHealth([ContainerHealth.Healthy, ContainerHealth.None])).toBe(ContainerHealth.Healthy)
		expect(worstHealth([ContainerHealth.None, ContainerHealth.None])).toBe(ContainerHealth.None)
		expect(worstHealth([])).toBe(ContainerHealth.None)
	})

	test("the system of a group is shared only when every container agrees", () => {
		expect(sharedValue(["node1", "node1"])).toBe("node1")
		expect(sharedValue(["node1", "node2"])).toBeUndefined()
		expect(sharedValue<string>([])).toBeUndefined()
	})
})

describe("no stacks in the data", () => {
	test("without stacks the grouping and the column are off", () => {
		const plain = [container("a", { stack: undefined, service: undefined }), container("b", { stack: "" })]
		expect(hasStacks(plain)).toBe(false)
		expect(buildGroupingState({ hasStacks: false, enabled: true })).toEqual([])
		expect(buildGroupingState({ hasStacks: true, enabled: false })).toEqual([])
		expect(buildGroupingState({ hasStacks: true, enabled: true })).toEqual(["stack", "service"])
	})

	test("one container with a stack is enough to turn the grouping on", () => {
		expect(hasStacks([container("a", { stack: "" }), container("b")])).toBe(true)
	})
})

describe("containers without labels", () => {
	test("they do not fall into one shared group", () => {
		const first = container("one", { stack: undefined, service: undefined })
		const second = container("two", { stack: undefined, service: undefined })
		const firstValue = getStackGroupingValue(first)
		const secondValue = getStackGroupingValue(second)
		expect(firstValue.startsWith(UNGROUPED)).toBe(true)
		expect(secondValue.startsWith(UNGROUPED)).toBe(true)
		expect(firstValue).not.toBe(secondValue)
		expect(getServiceGroupingValue(first)).not.toBe(getServiceGroupingValue(second))
	})

	test("the wrapper groups are unwrapped back into top level rows", () => {
		const stackLeafA = leaf("stack-a")
		const stackLeafB = leaf("stack-b")
		const loneA = leaf("lone-a")
		const loneB = leaf("lone-b")
		const stackGroup = group("stack", "myapp", [group("service", "node1\u0000web", [stackLeafA, stackLeafB])])
		const unwrapped = unwrapGroups(
			model([
				stackGroup,
				group("stack", `${UNGROUPED}node1/lone-a`, [group("service", `${UNGROUPED}node1/lone-a`, [loneA])]),
				group("stack", `${UNGROUPED}node1/lone-b`, [group("service", `${UNGROUPED}node1/lone-b`, [loneB])]),
			]),
			shouldUnwrapContainerGroup
		)
		expect(unwrapped.rows).toHaveLength(3)
		expect(unwrapped.rows[0]).toBe(stackGroup)
		expect(unwrapped.rows[1]).toBe(loneA)
		expect(unwrapped.rows[2]).toBe(loneB)
		for (const row of [loneA, loneB]) {
			expect(row.depth).toBe(0)
			expect(row.parentId).toBeUndefined()
		}
		// the real stack keeps its service level and the containers below it
		expect(stackGroup.subRows).toHaveLength(1)
		expect(stackGroup.subRows[0].subRows).toEqual([stackLeafA, stackLeafB])
		expect(stackLeafA.depth).toBe(2)
		expect(stackLeafA.parentId).toBe(stackGroup.subRows[0].id)
		expect(unwrapped.rowsById[loneA.id]).toBe(loneA)
		expect(unwrapped.rowsById).not.toHaveProperty(`stack:${UNGROUPED}node1/lone-a`)
	})

	test("a model with nothing to unwrap is returned unchanged", () => {
		const input = model([group("stack", "myapp", [leaf("a"), leaf("b")])])
		expect(unwrapGroups(input, shouldUnwrapContainerGroup)).toBe(input)
	})
})

describe("services", () => {
	test("replicas of one service on one node share a group", () => {
		const replicas = ["a", "b", "c"].map((id) => container(id, { service: "web" }))
		const values = new Set(replicas.map(getServiceGroupingValue))
		expect(values.size).toBe(1)
		// the same service on another node is a group of its own
		expect(getServiceGroupingValue(container("d", { system: "node2" }))).not.toBe([...values][0])
		// a different service on the same node too
		expect(getServiceGroupingValue(container("e", { service: "db" }))).not.toBe([...values][0])
	})

	test("a single replica does not add a level", () => {
		const single = group("service", "node1\u0000web", [leaf("only")])
		const several = group("service", "node1\u0000db", [leaf("one"), leaf("two")])
		expect(shouldUnwrapContainerGroup(single)).toBe(true)
		expect(shouldUnwrapContainerGroup(several)).toBe(false)
		// a stack group with a single container keeps its row: the stack stays visible
		expect(shouldUnwrapContainerGroup(group("stack", "myapp", [leaf("only")]))).toBe(false)

		const onlyLeaf = leaf("only")
		const unwrapped = unwrapGroups(
			model([group("stack", "myapp", [group("service", "node1\u0000web", [onlyLeaf]), several])]),
			shouldUnwrapContainerGroup
		)
		expect(unwrapped.rows[0].subRows[0]).toBe(onlyLeaf)
		expect(onlyLeaf.depth).toBe(1)
		expect(unwrapped.rows[0].subRows[1]).toBe(several)
	})

	test("the label of a service row drops the node prefix", () => {
		expect(rowGroupLabel(group("service", "node1\u0000web", []))).toBe("web")
		expect(rowGroupLabel(group("stack", "myapp", []))).toBe("myapp")
		expect(rowGroupLabel(group("stack", `${UNGROUPED}node1/lone`, []))).toBe("")
		expect(rowGroupLabel(leaf("a"))).toBe("")
	})
})

describe("fields of records and of stats entries", () => {
	test("the same getters read the table record and the chart stats entry", () => {
		expect(getStackValue({ stack: "myapp" })).toBe("myapp")
		expect(getServiceValue({ service: "web" })).toBe("web")
		// container stats keep the same fields under the short json keys of the agent
		expect(getStackValue({ st: "myapp" })).toBe("myapp")
		expect(getServiceValue({ sv: "web" })).toBe("web")
		expect(getStackValue({})).toBe("")
		expect(getServiceValue({})).toBe("")
		expect(getServiceGroupingValue({ n: "mentors_web.1", system: "node1", st: "mentors", sv: "web" })).toBe(
			getServiceGroupingValue({ name: "mentors_web.2", system: "node1", stack: "mentors", service: "web" })
		)
	})
})

describe("saved state", () => {
	test("group row ids stay the same between data updates", () => {
		const snapshot = (cpu: number) => [
			container("a", { cpu }),
			container("b", { cpu: cpu * 2, service: "db" }),
			container("c", { system: "node2" }),
		]
		const ids = (records: ReturnType<typeof snapshot>) =>
			records.map((record) => {
				const stackId = groupRowId("stack", getStackGroupingValue(record))
				return groupRowId("service", getServiceGroupingValue(record), stackId)
			})
		const first = ids(snapshot(1))
		const second = ids(snapshot(99).reverse())
		expect(new Set(first)).toEqual(new Set(second))
		expect(first[0]).toBe("stack:myapp>service:node1\u0000web")
	})

	// that the ids built here really are the ids of the rows, and that a saved state applies to
	// the model, is covered against the real table in containers-table-grouping.test.tsx
})

describe("comparators", () => {
	test("group rows sort by their label, leaf rows by the container name", () => {
		const groupA = group("stack", "alpha", [], { name: 7 })
		const groupB = group("stack", "beta", [], { name: 2 })
		// the aggregated value of the name column is the container count, so only the label counts
		expect(compareGroupAwareName(groupA, groupB, "name")).toBeLessThan(0)
		expect(compareGroupAwareName(groupB, groupA, "name")).toBeGreaterThan(0)

		const container1 = leaf("1", { name: "alpine" })
		const container2 = leaf("2", { name: "zebra" })
		expect(compareGroupAwareName(container1, container2, "name")).toBeLessThan(0)
		// a stack row next to an unlabeled container compares label against name
		expect(compareGroupAwareName(groupB, container1, "name")).toBeGreaterThan(0)
	})

	test("the system comparator falls back to the name and stays group aware", () => {
		const compare = makeSystemComparator((value) => (value === "node1" ? "alpha" : "beta"))
		const onNode1 = leaf("1", { system: "node1", name: "a" })
		const onNode2 = leaf("2", { system: "node2", name: "a" })
		expect(compare(onNode1, onNode2, "system")).toBeLessThan(0)
		const sameNode = leaf("3", { system: "node1", name: "b" })
		expect(compare(onNode1, sameNode, "system")).toBeLessThan(0)
		const stackRow = group("stack", "zzz", [], { system: "node1", name: 3 })
		expect(compare(stackRow, onNode1, "system")).toBeGreaterThan(0)
	})

	test("status and ports are compared by value, not by string", () => {
		expect(
			compareStatus(leaf("1", { status: "Up 2 hours" }), leaf("2", { status: "Up 10 minutes" }), "status")
		).toBeGreaterThan(0)
		expect(
			compareStatus(leaf("1", { status: "Up a minute" }), leaf("2", { status: "Up 30 seconds" }), "status")
		).toBeGreaterThan(0)
		// a group row has no status at all and must not throw
		expect(compareStatus(group("stack", "myapp", []), leaf("2", { status: "Up 2 hours" }), "status")).toBeLessThan(0)
		expect(
			comparePorts(leaf("1", { ports: "127.0.0.1:8080" }), leaf("2", { ports: "80, 443" }), "ports")
		).toBeGreaterThan(0)
		expect(comparePorts(group("stack", "myapp", []), leaf("2", { ports: "80" }), "ports")).toBeLessThan(0)
	})
})
