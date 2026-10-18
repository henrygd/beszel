import { expect, test } from "bun:test"
import type { SystemStatsRecord } from "@/types"
import { fanControlChannels, fanControlColor, fanControlLevel } from "./fan-control"

const record = (fc?: Record<string, number>) => ({ stats: { fc } }) as SystemStatsRecord

test("historical channels survive a missing latest sample, including all-zero controls", () => {
	const records = [record({ pwm: 40, binary: 100, off: 0 }), record({ pwm: 60 }), record()]
	expect(fanControlChannels(records)).toEqual(["binary", "off", "pwm"])
	expect(fanControlLevel(records[0], "off")).toBe(0)
	expect(fanControlLevel(records[1], "binary")).toBeUndefined()
	expect(fanControlLevel(records[2], "pwm")).toBeUndefined()
	expect(fanControlChannels([record({ off: 0 })])).toEqual(["off"])
})

test("old records and malformed values do not create chart series or fabricate zero", () => {
	const invalid = record({ negative: -1, large: 101, nan: NaN, infinite: Infinity })
	expect(fanControlChannels([record(), invalid])).toEqual([])
	for (const key of ["negative", "large", "nan", "infinite", "missing"]) {
		expect(fanControlLevel(invalid, key)).toBeUndefined()
	}
})

test("fractional rollups are retained and PWM and GPIO channels have distinct colors", () => {
	expect(fanControlLevel(record({ binary: 66.67 }), "binary")).toBe(66.67)
	expect(fanControlColor("pwmfan_pwm1")).not.toBe(fanControlColor("gpio_fan_pwm1"))
})
