import { afterEach, describe, expect, mock, test } from "bun:test"
import type { ReactNode } from "react"
import type { ChartData, CustomMetricMeta, SystemRecord } from "../src/types"

// What bun cannot load without a build: lib/api, which pulls in the compiled translations
// (stores only uses pb for the auth record), and the lingui macros the build compiles away.
// tests/setup.ts provides the browser storage stores reads on import.
mock.module("../src/lib/api", () => ({ pb: { authStore: { isValid: false, record: null } } }))
mock.module("@lingui/core/macro", () => ({
	t: (strings: TemplateStringsArray | string) => (typeof strings === "string" ? strings : strings[0]),
	plural: (_: number, forms: { other?: string }) => forms.other ?? "",
}))

// The card shows its chart only once it scrolls into view, and Recharts draws nothing
// outside a browser, so both are stand-ins that record what the component passes them.
type CardProps = { title: string; description: string; legend: boolean; children: ReactNode }
type ChartProps = {
	customData: unknown
	legend: boolean
	tickFormatter: (value: number) => string
	contentFormatter: (item: { value: number }, label: string) => string
	dataPoints: { label: string; color: string }[]
}
const cards: Omit<CardProps, "children">[] = []
const charts: ChartProps[] = []
mock.module("../src/components/routes/system/chart-card", () => ({
	ChartCard: ({ children, ...props }: CardProps) => {
		cards.push(props)
		return children
	},
}))
mock.module("../src/components/charts/line-chart", () => ({
	default: (props: ChartProps) => {
		charts.push(props)
		return null
	},
	isolatedDot: () => null,
}))

const { renderToStaticMarkup } = await import("react-dom/server")
const { createElement } = await import("react")
const { CustomMetricCharts } = await import("../src/components/routes/system/charts/custom-metric-charts")
const { $userSettings } = await import("../src/lib/stores")
const { Unit } = await import("../src/lib/enums")

/** Renders the charts for this metadata, with one record holding these values, if any. */
function render(cmm: Record<string, CustomMetricMeta>, values?: Record<string, number>) {
	cards.length = 0
	charts.length = 0
	const system = { info: { cmm } } as unknown as SystemRecord
	const customData = values ? [{ created: 1, stats: values }] : []
	const chartData = { customData } as unknown as ChartData
	renderToStaticMarkup(createElement(CustomMetricCharts, { system, chartData, grid: true, dataEmpty: false }))
	return customData
}

const settings = $userSettings.get()
afterEach(() => $userSettings.set(settings))

describe("CustomMetricCharts", () => {
	test("draws nothing without custom metric data in range", () => {
		render({ a: { c: "Power" } })
		render({ a: { c: "Power" } }, {})
		expect(cards).toHaveLength(0)
		expect(charts).toHaveLength(0)
	})

	test("draws one card per chart, in title order, each with its subtitle and legend", () => {
		const customData = render(
			{
				board: { u: "watts", l: "Board", c: "Power", cd: "Measured board power" },
				wall: { u: "watts", l: "Wall", c: "Power" },
				queue: { h: "Messages waiting.", c: "Queue depth" },
				bare: { c: "Accumulated" },
			},
			{ board: 1.8, wall: 2.6, queue: 4, bare: 9 }
		)
		expect(cards).toEqual(
			[
				{ title: "Accumulated", description: "Values reported by scripts and exporters on this system", legend: false },
				{ title: "Power", description: "Measured board power", legend: true },
				{ title: "Queue depth", description: "Messages waiting", legend: false },
			].map((card) => expect.objectContaining(card))
		)
		expect(charts.map((chart) => chart.dataPoints.map((line) => line.label))).toEqual([
			["bare"],
			["Board", "Wall"],
			["queue"],
		])
		expect(charts.map((chart) => chart.legend)).toEqual([false, true, false])
		expect(charts.every((chart) => chart.customData === customData)).toBe(true)
	})

	test("formats ticks and tooltips by each series' unit and the user's unit settings", () => {
		$userSettings.setKey("unitTemp", Unit.Fahrenheit)
		$userSettings.setKey("unitDisk", Unit.Bits)
		render(
			{
				cpu: { u: "celsius", l: "CPU", c: "Temperatures" },
				disk: { u: "celsius", l: "Disk", c: "Temperatures" },
				read: { u: "bytes/s", l: "Read", c: "Mixed" },
				size: { u: "bytes", l: "Size", c: "Mixed" },
			},
			{ cpu: 45.25, disk: 30, read: 12345678, size: 123456789012 }
		)
		const [mixed, temperatures] = charts
		expect(temperatures.tickFormatter(45.25)).toBe("113.45 °F")
		expect(temperatures.contentFormatter({ value: 30 }, "Disk")).toBe("86 °F")
		expect(mixed.tickFormatter(5)).toBe("5")
		expect(mixed.contentFormatter({ value: 12345678 }, "Read")).toBe("98.77 Mbps")
		expect(mixed.contentFormatter({ value: 123456789012 }, "Size")).toBe("114.98 GB")
	})
})
