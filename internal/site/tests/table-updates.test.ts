import { beforeEach, describe, expect, test } from "bun:test"
import { $tableUpdates, markTableUpdate } from "../src/components/charts/table-updates"

beforeEach(() => $tableUpdates.set(new Set()))

describe("pending table updates", () => {
	test("new data marks the relevant table", () => {
		markTableUpdate("cpu", true)
		expect([...$tableUpdates.get()]).toEqual(["cpu"])
	})

	test("repeating the same state does not notify subscribers again", () => {
		markTableUpdate("cpu", true)
		const current = $tableUpdates.get()
		markTableUpdate("cpu", true)
		expect($tableUpdates.get()).toBe(current)
	})

	test("refreshing or unmounting one table preserves other pending tables", () => {
		markTableUpdate("cpu", true)
		markTableUpdate("memory", true)
		markTableUpdate("cpu", false)
		expect([...$tableUpdates.get()]).toEqual(["memory"])
	})

	test("the announcement can clear after the last table refresh", () => {
		markTableUpdate("cpu", true)
		markTableUpdate("cpu", false)
		expect($tableUpdates.get().size).toBe(0)
		const empty = $tableUpdates.get()
		markTableUpdate("cpu", false)
		expect($tableUpdates.get()).toBe(empty)
	})
})
