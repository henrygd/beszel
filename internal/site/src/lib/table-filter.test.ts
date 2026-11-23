import { expect, test } from "bun:test"
import { matchesTableFilter } from "./table-filter"

test("empty or whitespace filter matches everything", () => {
	expect(matchesTableFilter("", "nginx")).toBe(true)
	expect(matchesTableFilter("  ,  ", "nginx")).toBe(true)
})

test("space-separated terms are AND'd, case-insensitively", () => {
	expect(matchesTableFilter("NGINX run", "nginx", "Running")).toBe(true)
	expect(matchesTableFilter("nginx stopped", "nginx", "Running")).toBe(false)
})

test("comma-separated groups are OR'd", () => {
	expect(matchesTableFilter("redis, nginx", "nginx")).toBe(true)
	expect(matchesTableFilter("redis, postgres", "nginx")).toBe(false)
})

test("terms don't match across field boundaries", () => {
	expect(matchesTableFilter("ab", "a", "b")).toBe(false)
})

test("nullish fields are skipped", () => {
	expect(matchesTableFilter("web", null, undefined, "web-1")).toBe(true)
	expect(matchesTableFilter("null", null)).toBe(false)
})
