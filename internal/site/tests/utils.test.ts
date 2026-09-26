import { describe, expect, mock, test } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	t: (strings: any) => (typeof strings === "string" ? strings : strings?.[0] ?? ""),
	plural: (count: number, forms: any) => forms.other ?? "",
}))

const { formatMicroseconds } = await import("../src/lib/utils")

describe("formatMicroseconds", () => {
	test("formats with fixedDigits = true (default)", () => {
		expect(formatMicroseconds(500)).toBe("500μs")
		expect(formatMicroseconds(6000)).toBe("6.00ms")
		expect(formatMicroseconds(6500)).toBe("6.50ms")
		expect(formatMicroseconds(12000)).toBe("12.0ms")
		expect(formatMicroseconds(12500)).toBe("12.5ms")
		expect(formatMicroseconds(1_000_000)).toBe("1.00s")
		expect(formatMicroseconds(1_500_000)).toBe("1.50s")
		expect(formatMicroseconds(12_000_000)).toBe("12.0s")
	})

	test("formats with fixedDigits = false (used for chart y-axis ticks)", () => {
		expect(formatMicroseconds(500, false)).toBe("500μs")
		expect(formatMicroseconds(500.5, false)).toBe("500.5μs")
		expect(formatMicroseconds(6000, false)).toBe("6ms")
		expect(formatMicroseconds(6200, false)).toBe("6.2ms")
		expect(formatMicroseconds(6500, false)).toBe("6.5ms")
		expect(formatMicroseconds(6750, false)).toBe("6.75ms")
		expect(formatMicroseconds(7000, false)).toBe("7ms")
		expect(formatMicroseconds(12000, false)).toBe("12ms")
		expect(formatMicroseconds(12500, false)).toBe("12.5ms")
		expect(formatMicroseconds(1_000_000, false)).toBe("1s")
		expect(formatMicroseconds(1_500_000, false)).toBe("1.5s")
		expect(formatMicroseconds(12_000_000, false)).toBe("12s")
	})

	test("prevents duplicate tick values for intermediate intervals (#2373)", () => {
		const tickInputs = [6000, 6200, 6400, 6600, 6800, 7000]
		const formattedTicks = tickInputs.map((val) => formatMicroseconds(val, false))
		expect(formattedTicks).toEqual(["6ms", "6.2ms", "6.4ms", "6.6ms", "6.8ms", "7ms"])
		// Verify all formatted ticks are unique
		const uniqueTicks = new Set(formattedTicks)
		expect(uniqueTicks.size).toBe(tickInputs.length)
	})

	test("handles invalid / non-finite values", () => {
		expect(formatMicroseconds(Number.NaN)).toBe("-")
		expect(formatMicroseconds(Number.POSITIVE_INFINITY)).toBe("-")
		expect(formatMicroseconds(Number.NEGATIVE_INFINITY)).toBe("-")
	})
})
