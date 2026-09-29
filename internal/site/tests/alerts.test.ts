import { describe, expect, mock, test } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	t: (strings: any) => (typeof strings === "string" ? strings : strings?.[0] ?? ""),
}))
// alertManager grabs the alerts collection at import time; stores only uses pb for the auth record
mock.module("../src/lib/api", () => ({
	pb: { authStore: { isValid: false, record: null }, collection: () => ({}) },
}))

const { alertInfo, alertInputWidth, clampAlertValue } = await import("../src/lib/alerts")

describe("bandwidth threshold range", () => {
	const bandwidth = alertInfo.Bandwidth

	test("max covers a saturated full-duplex 10GbE link", () => {
		expect(bandwidth.max).toBe(2500)
	})

	test("values within range are kept", () => {
		expect(clampAlertValue(bandwidth, 1800)).toBe(1800)
		expect(clampAlertValue(bandwidth, 2500)).toBe(2500)
	})

	test("values above the max are clamped", () => {
		expect(clampAlertValue(bandwidth, 3000)).toBe(2500)
	})
})

describe("clampAlertValue", () => {
	test("respects an explicit min", () => {
		expect(clampAlertValue(alertInfo.LoadAvg1, 0)).toBe(0.1)
	})

	test("leaves values alone when the alert defines no bounds", () => {
		// Temperature has no max, so typed values above the slider range stay allowed
		expect(clampAlertValue(alertInfo.Temperature, 150)).toBe(150)
	})
})

describe("alertInputWidth", () => {
	test("fits four digits for bandwidth", () => {
		expect(alertInputWidth(alertInfo.Bandwidth)).toBe("max(4rem, 7ch)")
	})

	test("keeps the previous w-16 floor for percentage alerts", () => {
		expect(alertInputWidth(alertInfo.CPU)).toBe("max(4rem, 5ch)")
	})

	test("counts decimal places from the step", () => {
		// max 99.9 with step 0.1 -> "99.9"
		expect(alertInputWidth(alertInfo.NetworkMonitorLoss)).toBe("max(4rem, 7ch)")
	})
})
