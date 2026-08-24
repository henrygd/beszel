import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { batteryStateTranslations } from "@/lib/i18n"
import { $fanFilter, $temperatureFilter, $userSettings } from "@/lib/stores"
import { cn, decimalString, formatTemperature, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemRecord, SystemStatsRecord } from "@/types"
import { ChartCard, FilterBar } from "../chart-card"
import LineChartDefault from "@/components/charts/line-chart"
import { useStore } from "@nanostores/react"
import { useRef, useMemo, useState, useEffect } from "react"

export function BatteryChart({
	chartData,
	grid,
	dataEmpty,
	maxValues,
	system,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	maxValues: boolean
	system: SystemRecord
}) {
	const batteryNames = useMemo(() => {
		const names = new Set<string>()
		for (const record of chartData.systemStats) {
			for (const name in record.stats?.bats ?? {}) {
				names.add(name)
			}
		}
		return [...names].sort()
	}, [chartData.systemStats])
	const hasNamedBatteries = batteryNames.length > 0
	const showBatteryChart = hasNamedBatteries || chartData.systemStats.some((record) => record.stats?.bat)

	if (!showBatteryChart) {
		return null
	}

	if (hasNamedBatteries) {
		const dataPoints = batteryNames.map((name, index) => ({
			label: name,
			dataKey: ({ stats }: SystemStatsRecord) => stats?.bats?.[name],
			color: `hsl(${(index * 360 + 226) / batteryNames.length}, 65%, 52%)`,
		}))
		return (
			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`Battery`}
				description={`${t({
					message: "Current state",
					comment: "Context: Battery state",
				})}: ${batteryStateTranslations[system.info.bat?.[1] ?? 0]()}`}
			>
				<LineChartDefault
					chartData={chartData}
					maxToggled={maxValues}
					dataPoints={dataPoints}
					domain={[0, 100]}
					legend={true}
					tickFormatter={(val) => `${val}%`}
					contentFormatter={({ value }) => `${value}%`}
					itemSorter={(a, b) => b.value - a.value}
				/>
			</ChartCard>
		)
	}

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Battery`}
			description={`${t({
				message: "Current state",
				comment: "Context: Battery state",
			})}: ${batteryStateTranslations[system.info.bat?.[1] ?? 0]()}`}
		>
			<AreaChartDefault
				chartData={chartData}
				maxToggled={maxValues}
				dataPoints={[
					{
						label: t`Charge`,
						dataKey: ({ stats }) => stats?.bat?.[0],
						color: 1,
						opacity: 0.35,
					},
				]}
				domain={[0, 100]}
				tickFormatter={(val) => `${val}%`}
				contentFormatter={({ value }) => `${value}%`}
			/>
		</ChartCard>
	)
}

export function TemperatureChart({
	chartData,
	grid,
	dataEmpty,
	setPageBottomExtraMargin,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	setPageBottomExtraMargin?: (margin: number) => void
}) {
	const showTempChart = chartData.systemStats.at(-1)?.stats.t

	const filter = useStore($temperatureFilter)
	const userSettings = useStore($userSettings)

	const statsRef = useRef(chartData.systemStats)
	statsRef.current = chartData.systemStats

	// Derive sensor names key from latest data point
	let sensorNamesKey = ""
	for (let i = chartData.systemStats.length - 1; i >= 0; i--) {
		const t = chartData.systemStats[i].stats?.t
		if (t) {
			sensorNamesKey = Object.keys(t).sort().join("\0")
			break
		}
	}

	// Only recompute colors and dataKey functions when sensor names change
	const { colorMap, dataKeys, sortedKeys } = useMemo(() => {
		const stats = statsRef.current
		const tempSums = {} as Record<string, number>
		for (const data of stats) {
			const t = data.stats?.t
			if (!t) continue
			for (const key of Object.keys(t)) {
				tempSums[key] = (tempSums[key] ?? 0) + t[key]
			}
		}
		const sorted = Object.keys(tempSums).sort((a, b) => tempSums[b] - tempSums[a])
		const colorMap = {} as Record<string, string>
		const dataKeys = {} as Record<string, (d: SystemStatsRecord) => number | undefined>
		for (let i = 0; i < sorted.length; i++) {
			const key = sorted[i]
			colorMap[key] = `hsl(${((i * 360) / sorted.length) % 360}, 60%, 55%)`
			dataKeys[key] = (d: SystemStatsRecord) => d.stats?.t?.[key]
		}
		return { colorMap, dataKeys, sortedKeys: sorted }
	}, [sensorNamesKey])

	const dataPoints = useMemo(() => {
		return sortedKeys.map((key) => {
			const filterTerms = filter
				? filter
						.toLowerCase()
						.split(" ")
						.filter((term) => term.length > 0)
				: []
			const filtered = filterTerms.length > 0 && !filterTerms.some((term) => key.toLowerCase().includes(term))
			const strokeOpacity = filtered ? 0.1 : 1
			return {
				label: key,
				dataKey: dataKeys[key],
				color: colorMap[key],
				strokeOpacity,
				activeDot: !filtered,
			}
		})
	}, [sortedKeys, filter, dataKeys, colorMap])

	// test with lots of data points
	// const totalPoints = 50
	// if (dataPoints.length > 0 && dataPoints.length < totalPoints) {
	// 	let i = 0
	// 	while (dataPoints.length < totalPoints) {
	// 		dataPoints.push({
	// 			label: `Test ${++i}`,
	// 			dataKey: () => 0,
	// 			color: "red",
	// 			strokeOpacity: 1,
	// 		})
	// 	}
	// }

	const chartRef = useRef<HTMLDivElement>(null)
	const [addMargin, setAddMargin] = useState(false)
	const marginPx = (dataPoints.length - 13) * 18

	useEffect(() => {
		if (setPageBottomExtraMargin && dataPoints.length > 13 && chartRef.current) {
			const checkPosition = () => {
				if (!chartRef.current) return
				const rect = chartRef.current.getBoundingClientRect()
				const actualScrollHeight = addMargin
					? document.documentElement.scrollHeight - marginPx
					: document.documentElement.scrollHeight
				const distanceToBottom = actualScrollHeight - (rect.bottom + window.scrollY)

				if (distanceToBottom < 250) {
					setAddMargin(true)
					setPageBottomExtraMargin(marginPx)
				} else {
					setAddMargin(false)
					setPageBottomExtraMargin(0)
				}
			}
			checkPosition()
			const timer = setTimeout(checkPosition, 500)
			return () => {
				clearTimeout(timer)
			}
		} else if (addMargin) {
			setAddMargin(false)
			if (setPageBottomExtraMargin) setPageBottomExtraMargin(0)
		}
	}, [dataPoints.length, addMargin, marginPx, setPageBottomExtraMargin])

	if (!showTempChart) {
		return null
	}

	const legend = dataPoints.length < 12

	return (
		<div ref={chartRef} className={cn("odd:last-of-type:col-span-full", { "col-span-full": !grid })}>
			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`Temperature`}
				description={t`Temperatures of system sensors`}
				cornerEl={<FilterBar store={$temperatureFilter} />}
				legend={legend}
			>
				<LineChartDefault
					chartData={chartData}
					itemSorter={(a, b) => b.value - a.value}
					domain={["auto", "auto"]}
					legend={legend}
					tickFormatter={(val) => {
						const { value, unit } = formatTemperature(val, userSettings.unitTemp)
						return `${toFixedFloat(value, 2)} ${unit}`
					}}
					contentFormatter={(item) => {
						const { value, unit } = formatTemperature(item.value, userSettings.unitTemp)
						return `${decimalString(value)} ${unit}`
					}}
					dataPoints={dataPoints}
					filter={filter}
				></LineChartDefault>
			</ChartCard>
		</div>
	)
}

export function FanChart({ chartData, grid, dataEmpty }: { chartData: ChartData; grid: boolean; dataEmpty: boolean }) {
	const showFanChart = chartData.systemStats.at(-1)?.stats.f

	const filter = useStore($fanFilter)

	const statsRef = useRef(chartData.systemStats)
	statsRef.current = chartData.systemStats

	// Stable key derived from current sensor names (sorted) so the memo only
	// recomputes when the set of fans changes — not on every data point.
	let sensorNamesKey = ""
	for (let i = chartData.systemStats.length - 1; i >= 0; i--) {
		const f = chartData.systemStats[i].stats?.f
		if (f) {
			sensorNamesKey = Object.keys(f).sort().join("\0")
			break
		}
	}

	const { colorMap, dataKeys, sortedKeys } = useMemo(() => {
		const stats = statsRef.current
		const sums = {} as Record<string, number>
		for (const data of stats) {
			const f = data.stats?.f
			if (!f) continue
			for (const key of Object.keys(f)) {
				sums[key] = (sums[key] ?? 0) + f[key]
			}
		}
		const sorted = Object.keys(sums).sort((a, b) => sums[b] - sums[a])
		const colorMap = {} as Record<string, string>
		const dataKeys = {} as Record<string, (d: SystemStatsRecord) => number | undefined>
		for (let i = 0; i < sorted.length; i++) {
			const key = sorted[i]
			colorMap[key] = `hsl(${((i * 360) / sorted.length) % 360}, 60%, 55%)`
			dataKeys[key] = (d: SystemStatsRecord) => d.stats?.f?.[key]
		}
		return { colorMap, dataKeys, sortedKeys: sorted }
	}, [sensorNamesKey])

	const dataPoints = useMemo(() => {
		return sortedKeys.map((key) => {
			const filterTerms = filter
				? filter
						.toLowerCase()
						.split(" ")
						.filter((term) => term.length > 0)
				: []
			const filtered = filterTerms.length > 0 && !filterTerms.some((term) => key.toLowerCase().includes(term))
			const strokeOpacity = filtered ? 0.1 : 1
			return {
				label: key,
				dataKey: dataKeys[key],
				color: colorMap[key],
				strokeOpacity,
				activeDot: !filtered,
			}
		})
	}, [sortedKeys, filter, dataKeys, colorMap])

	if (!showFanChart) {
		return null
	}

	const legend = dataPoints.length < 12

	return (
		<div className={cn("odd:last-of-type:col-span-full", { "col-span-full": !grid })}>
			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`Fans`}
				description={t`System fan speeds (RPM)`}
				cornerEl={<FilterBar store={$fanFilter} />}
				legend={legend}
			>
				<LineChartDefault
					chartData={chartData}
					itemSorter={(a, b) => b.value - a.value}
					domain={["auto", "auto"]}
					legend={legend}
					tickFormatter={(val) => `${toFixedFloat(val, 0)}`}
					contentFormatter={(item) => `${decimalString(item.value, 0)} RPM`}
					dataPoints={dataPoints}
					filter={filter}
				></LineChartDefault>
			</ChartCard>
		</div>
	)
}
