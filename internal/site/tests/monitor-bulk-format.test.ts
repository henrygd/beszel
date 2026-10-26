import { describe, expect, test } from "bun:test"
import { formatMonitorFields, parseMonitorFields } from "../src/lib/monitor-bulk-format"

describe("bulk monitor rows", () => {
	test.each(
		[
			["1.1.1.1", "icmp"],
			["example.com", "tcp", "8080", "60"],
			["example.com", "dns", "", "", "1.1.1.1"],
			["https://example.com/status?ids=1,2", "http", "", "60"],
			['https://example.com/status?q="a,b"', "http"],
			["", "http", ""],
		].map((fields) => ({ fields }))
	)("round-trips $fields", ({ fields }) => {
		expect(parseMonitorFields(formatMonitorFields(fields))).toEqual(fields)
	})

	test("keeps the existing unquoted format for ordinary entries", () => {
		expect(formatMonitorFields(["example.com", "tcp", "", "60"])).toBe("example.com,tcp,,60")
		expect(parseMonitorFields("example.com,tcp,,60")).toEqual(["example.com", "tcp", "", "60"])
	})

	test("does not treat a quoted target's comma as its protocol separator", () => {
		expect(parseMonitorFields('"https://example.com/status?ids=1,2",http')).toEqual([
			"https://example.com/status?ids=1,2",
			"http",
		])
	})

	test("rejects malformed quoted rows instead of shifting fields", () => {
		expect(() => parseMonitorFields('"https://example.com,status,http')).toThrow("Unclosed quoted field")
		expect(() => parseMonitorFields('"example.com"extra,tcp')).toThrow("Expected comma after quoted field")
	})
})
