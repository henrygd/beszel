import { expect, test } from "bun:test"
import { temperatureSensorNames, temperatureThresholdInput } from "./temperature-alerts"

test("discovers sensors while retaining missing saved selections", () => {
	expect(temperatureSensorNames({ missing: 75, cpu: 70 }, { cpu: 50, gpu: 60 })).toEqual(["cpu", "gpu", "missing"])
})

test("empty or missing samples retain saved thresholds", () => {
	expect(temperatureSensorNames({ cpu: 80 })).toEqual(["cpu"])
	expect(temperatureSensorNames({}, {})).toEqual([])
	expect(temperatureSensorNames({})).toEqual([])
})

test("sensor names are literal keys, including prototype-looking names", () => {
	expect(temperatureSensorNames(JSON.parse('{"__proto__":80}'), { constructor: 30 })).toEqual([
		"__proto__", "constructor",
	])
})

test("threshold inputs clamp, preserve decimals, and reject invalid numbers", () => {
	expect(temperatureThresholdInput("70.5")).toBe(70.5)
	expect(temperatureThresholdInput("0")).toBe(1)
	expect(temperatureThresholdInput("120")).toBe(99)
	expect(temperatureThresholdInput("120", 0, 150)).toBe(120)
	for (const value of ["", " ", "NaN", "Infinity", "-Infinity", "1e999", "80bad"]) {
		expect(temperatureThresholdInput(value)).toBeUndefined()
	}
})
