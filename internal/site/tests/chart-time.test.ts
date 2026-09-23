import { beforeEach, describe, expect, mock, test } from "bun:test"
import type { UserSettings } from "../src/types"

// lib/api needs a browser and the lingui macro transform, and stores only uses pb for the auth record
mock.module("../src/lib/api", () => ({ pb: { authStore: { isValid: false, record: null } } }))

const { $chartTime, $userSettings, defaultChartTime, getUserChartTime } = await import("../src/lib/stores")

describe("chart time from user settings", () => {
	beforeEach(() => {
		$chartTime.set(defaultChartTime)
	})

	test("uses the stored chart time", () => {
		$userSettings.set({ chartTime: "24h", emails: [] } as UserSettings)
		expect($chartTime.get()).toBe("24h")
	})

	test("falls back to the default when settings have no chart time", () => {
		$userSettings.set({ emails: [] } as UserSettings)
		expect($chartTime.get()).toBe(defaultChartTime)
		expect(getUserChartTime()).toBe(defaultChartTime)
	})

	test("falls back to the default when the stored chart time is empty", () => {
		$userSettings.set({ chartTime: "", emails: [] } as unknown as UserSettings)
		expect($chartTime.get()).toBe(defaultChartTime)
		expect(getUserChartTime()).toBe(defaultChartTime)
	})
})
