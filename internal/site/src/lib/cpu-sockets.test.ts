import { expect, test } from "bun:test"
import { cpuSocketDisplay } from "./cpu-sockets"

const model = "Intel(R) Xeon(R) 6530P"

test("multi-socket systems get the model and tooltip prefix", () => {
	expect(cpuSocketDisplay(model, 2, "2 CPUs")).toEqual({ model: `2x ${model}`, labelPrefix: "2 CPUs / " })
})

test("single-socket and unknown counts are unchanged", () => {
	for (const sockets of [1, 0, undefined]) {
		expect(cpuSocketDisplay(model, sockets, "x")).toEqual({ model, labelPrefix: "" })
	}
})

test("no prefix without a model", () => {
	expect(cpuSocketDisplay(undefined, 2, "2 CPUs")).toEqual({ model: undefined, labelPrefix: "" })
})
