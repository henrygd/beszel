import { useMemo, useState } from "react"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData, SystemStats, SystemStatsRecord } from "@/types"

/** Generates evenly distributed HSL colors for chart series */
function getChartColor(index: number, total: number, startHue = 0): string {
	const hue = (startHue + (index * 360) / total) % 360
	return `hsl(${hue}, 60%, 55%)`
}

/** Chart configurations for CPU, memory, and network usage charts */
export interface ContainerChartConfigs {
	cpu: ChartConfig
	memory: ChartConfig
	network: ChartConfig
}

/**
 * Generates chart configurations for container metrics visualization
 * @param containerData - Array of container statistics data points
 * @returns Chart configurations for CPU, memory, and network metrics
 */
export function useContainerChartConfigs(containerData: ChartData["containerData"]): ContainerChartConfigs {
	return useMemo(() => {
		if (!containerData.length) return { cpu: {}, memory: {}, network: {} }

		// Collect all containers and their total usage across all data points
		const usage = new Map<string, number>()
		for (const dataPoint of containerData) {
			for (const [name, s] of Object.entries(dataPoint)) {
				if (name === "created" || !s || typeof s !== "object") continue
				const stats = s as { c?: number; m?: number; b?: number[] }
				const value = (stats.c ?? 0) + (stats.m ?? 0) + (stats.b?.[0] ?? 0) + (stats.b?.[1] ?? 0)
				usage.set(name, (usage.get(name) ?? 0) + value)
			}
		}

		// Sort by total usage and generate config with consistent colors
		const sorted = [...usage.entries()].sort(([, a], [, b]) => b - a)
		const chartConfig: ChartConfig = {}
		for (let i = 0; i < sorted.length; i++) {
			chartConfig[sorted[i][0]] = { label: sorted[i][0], color: getChartColor(i, sorted.length) }
		}

		return { cpu: chartConfig, memory: chartConfig, network: chartConfig }
	}, [containerData])
}

/** Sets the correct width of the y axis in recharts based on the longest label */
export function useYAxisWidth() {
	const [yAxisWidth, setYAxisWidth] = useState(0)
	let maxChars = 0
	let timeout: ReturnType<typeof setTimeout>
	function updateYAxisWidth(str: string) {
		if (str.length > maxChars) {
			maxChars = str.length
			const div = document.createElement("div")
			div.className = "text-xs tabular-nums tracking-tighter table sr-only"
			div.innerHTML = str
			clearTimeout(timeout)
			timeout = setTimeout(() => {
				document.body.appendChild(div)
				const width = div.offsetWidth + 24
				if (width > yAxisWidth) {
					setYAxisWidth(div.offsetWidth + 24)
				}
				document.body.removeChild(div)
			})
		}
		return str
	}
	return { yAxisWidth, updateYAxisWidth }
}

// Assures consistent colors for network interfaces
export function useNetworkInterfaces(interfaces: SystemStats["ni"]) {
	const keys = Object.keys(interfaces ?? {})
	const sortedKeys = keys.sort((a, b) => (interfaces?.[b]?.[3] ?? 0) - (interfaces?.[a]?.[3] ?? 0))
	return {
		length: sortedKeys.length,
		data: (index = 3) => {
			return sortedKeys.map((key, i) => ({
				label: key,
				dataKey: ({ stats }: SystemStatsRecord) => stats?.ni?.[key]?.[index],
				color: getChartColor(i, sortedKeys.length, 220),
				opacity: 0.3,
			}))
		},
	}
}
