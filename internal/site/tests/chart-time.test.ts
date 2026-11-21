import { beforeEach, describe, expect, mock, test } from "bun:test"
import type { UserSettings } from "../src/types"

// lib/api needs a browser and the lingui macro transform, and stores only uses pb for the auth record
mock.module("../src/lib/api", () => ({ pb: { authStore: { isValid: false, record: null } } }))

const { $chartTime, $userSettings, defaultChartTime, getUserChartTime, hydrateUserSettings } = await import(
	"../src/lib/stores"
)

describe("chart time from user settings", () => {
	beforeEach(() => {
		$chartTime.set(defaultChartTime)
	})

	test("hydration uses the stored chart time", () => {
		hydrateUserSettings({ chartTime: "24h", emails: [] } as UserSettings)
		expect($chartTime.get()).toBe("24h")
	})

	test("hydration falls back to the default when settings have no chart time", () => {
		$chartTime.set("24h")
		hydrateUserSettings({ emails: [] } as UserSettings)
		expect($chartTime.get()).toBe(defaultChartTime)
		expect(getUserChartTime()).toBe(defaultChartTime)
	})

	test("hydration falls back to the default when the stored chart time is empty", () => {
		$chartTime.set("24h")
		hydrateUserSettings({ chartTime: "", emails: [] } as unknown as UserSettings)
		expect($chartTime.get()).toBe(defaultChartTime)
		expect(getUserChartTime()).toBe(defaultChartTime)
	})

	test("other settings writes don't change the active chart time", () => {
		$chartTime.set("12h")
		$userSettings.set({ chartTime: "24h", emails: [], grid: true } as UserSettings)
		$userSettings.setKey("chartTime", "7d")
		expect($chartTime.get()).toBe("12h")
	})
})
