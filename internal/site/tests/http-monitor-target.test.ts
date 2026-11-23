import { describe, expect, mock, test } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	t: (value: string | TemplateStringsArray) => (typeof value === "string" ? value : value[0]),
	plural: (_count: number, forms: { other: string }) => forms.other,
}))

mock.module("@lingui/react/macro", () => ({
	Trans: () => null,
	useLingui: () => {
		throw new Error("The normalization tests should not render UI")
	},
}))

await import("../src/lib/utils")
const { normalizeHttpTarget } = await import("../src/components/network-monitors-table/monitor-dialog")

describe("HTTP monitor target normalization", () => {
	test.each([
		["example.com/health", 8080, "https://example.com:8080/health"],
		["example.com?ready=1", 8080, "https://example.com:8080/?ready=1"],
		["example.com/#probe", 8080, "https://example.com:8080/#probe"],
		["example.com:9090/health", 8080, "https://example.com:9090/health"],
		["[::1]/health", 8080, "https://[::1]:8080/health"],
		["example.com:9090", 8080, "https://example.com:9090"],
		["https://example.com/health", 8080, "https://example.com:8080/health"],
		["example.com", 8080, "https://example.com:8080"],
		["example.com", 80, "http://example.com"],
		["example.com/health", 80, "http://example.com/health"],
		["example.com/health", 0, "https://example.com/health"],
		["http://example.com:8090", 0, "http://example.com:8090"],
		["https://example.com:9090/health", 8080, "https://example.com:9090/health"],
	] as const)("normalizes %s with port %d", (target, port, expected) => {
		expect(normalizeHttpTarget(target, port)).toBe(expected)
	})
})
