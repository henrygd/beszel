import { describe, expect, mock, test } from "bun:test"
import type { Row, SortingFn } from "@tanstack/react-table"
import type { ContainerRecord } from "../src/types"

mock.module("@lingui/core/macro", () => ({
	t: (value: string | TemplateStringsArray | { message: string }) =>
		typeof value === "string" ? value : "message" in value ? value.message : value[0],
	plural: (_count: number, forms: { other: string }) => forms.other,
}))

const { containerChartCols } = await import("../src/components/containers-table/containers-table-columns")
const sortStatus = containerChartCols.find((column) => column.id === "status")?.sortingFn as SortingFn<ContainerRecord>
const row = (status: string) => ({ original: { status } }) as Row<ContainerRecord>

describe("container status sorting", () => {
	test.each([
		"",
		"Created",
		"Paused",
		"Up",
		"Up invalid seconds",
	])("handles %j without throwing or returning NaN", (status) => {
		expect(Number.isFinite(sortStatus(row(status), row("Up 2 hours"), "status"))).toBe(true)
	})

	test("sorts years after months and weeks", () => {
		expect(sortStatus(row("Up 2 years"), row("Up 11 months"), "status")).toBeGreaterThan(0)
		expect(sortStatus(row("Up a year"), row("Up 11 months"), "status")).toBeGreaterThan(0)
		expect(sortStatus(row("Up 2 months"), row("Up 5 weeks"), "status")).toBeGreaterThan(0)
	})

	test.each([
		["Up a minute", "Up 59 seconds"],
		["Up an hour", "Up 59 minutes"],
		["Up 2 days (Paused)", "Up 47 hours"],
		["Up 2 months", "Up 59 days"],
	])("preserves duration ordering for %s and %s", (older, newer) => {
		expect(sortStatus(row(older), row(newer), "status")).toBeGreaterThan(0)
	})
})
