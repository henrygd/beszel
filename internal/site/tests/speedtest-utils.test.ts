import { describe, expect, test } from "bun:test"
import { formatSpeedtestInterval, getSpeedtestServerLabel } from "../src/lib/speedtest-utils"

describe("formatSpeedtestInterval", () => {
	test("uses the largest whole unit", () => {
		expect([15, 30, 60, 180, 360, 720, 1440].map(formatSpeedtestInterval)).toEqual([
			"15m",
			"30m",
			"1h",
			"3h",
			"6h",
			"12h",
			"1d",
		])
		expect(formatSpeedtestInterval(90)).toBe("90m")
		expect(formatSpeedtestInterval(2880)).toBe("2d")
	})
})

describe("getSpeedtestServerLabel", () => {
	test("shows name and location, and the ID only when the name is unknown", () => {
		expect(getSpeedtestServerLabel({ server_id: 42, server_name: "Example", server_location: "Amsterdam" })).toBe(
			"Example — Amsterdam"
		)
		expect(getSpeedtestServerLabel({ server_id: 42, server_name: "", server_location: "" })).toBe("#42")
		expect(getSpeedtestServerLabel({ server_id: 0, server_name: "Example", server_location: "" })).toBe("Example")
		expect(getSpeedtestServerLabel({ server_id: 0, server_name: "", server_location: "" })).toBe("")
	})

	test("appends a non-default interface", () => {
		expect(
			getSpeedtestServerLabel({
				server_id: 42,
				server_name: "Example",
				server_location: "Amsterdam",
				interface: "eth1",
			})
		).toBe("Example — Amsterdam (eth1)")
		expect(getSpeedtestServerLabel({ server_id: 42, server_name: "", server_location: "", interface: "eth1" })).toBe(
			"#42 (eth1)"
		)
		expect(getSpeedtestServerLabel({ server_id: 0, server_name: "", server_location: "", interface: "eth1" })).toBe("")
	})
})
