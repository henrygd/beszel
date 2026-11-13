import { beforeEach, describe, expect, mock, test } from "bun:test"
import type { UserSettings } from "../src/types"

// lib/api needs a browser and the lingui macro transform, and stores only uses pb for the auth record
mock.module("../src/lib/api", () => ({ pb: { authStore: { isValid: false, record: null } } }))

const { $maxValues, $userSettings, hydrateUserSettings } = await import("../src/lib/stores")

describe("default chart value from user settings", () => {
	beforeEach(() => {
		$maxValues.set(false)
	})

	test("hydration shows max values when the stored default is max", () => {
		hydrateUserSettings({ chartValue: "max", emails: [] } as UserSettings)
		expect($maxValues.get()).toBe(true)
	})

	test("hydration shows average values when the stored default is avg", () => {
		$maxValues.set(true)
		hydrateUserSettings({ chartValue: "avg", emails: [] } as UserSettings)
		expect($maxValues.get()).toBe(false)
	})

	test("hydration falls back to average when settings have no chart value", () => {
		$maxValues.set(true)
		hydrateUserSettings({ emails: [] } as UserSettings)
		expect($maxValues.get()).toBe(false)
	})

	test("manual switching and other settings writes don't change the saved default", () => {
		hydrateUserSettings({ chartValue: "avg", emails: [] } as UserSettings)
		$maxValues.set(true)
		$userSettings.setKey("grid", true)
		expect($userSettings.get().chartValue).toBe("avg")
		expect($maxValues.get()).toBe(true)
	})
})
