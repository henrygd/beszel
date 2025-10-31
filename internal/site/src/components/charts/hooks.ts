import { useMemo, useState } from "react"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData, SystemStats, SystemStatsRecord } from "@/types"

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
		// Calculate combined usage in a single pass
		const combinedUsage = new Map<string, number>()

		for (let i = 0; i < containerData.length; i++) {
			const stats = containerData[i]
			const containerNames = Object.keys(stats)

			for (let j = 0; j < containerNames.length; j++) {
				const containerName = containerNames[j]
				// Skip metadata field
				if (containerName === "created") {
					continue
				}

				const containerStats = stats[containerName]
				if (!containerStats) {
					continue
				}

				// Accumulate all metrics in one operation
				const cpu = containerStats.c ?? 0
				const memory = containerStats.m ?? 0
				const network = (containerStats.nr ?? 0) + (containerStats.ns ?? 0)
				const current = combinedUsage.get(containerName) ?? 0
				combinedUsage.set(containerName, current + cpu + memory + network)
			}
		}

		// Sort containers by combined usage to ensure consistent color assignment
		const sortedContainers = Array.from(combinedUsage.entries()).sort(([, a], [, b]) => b - a)
		const count = sortedContainers.length

		// Generate chart configurations with consistent colors
		const chartConfig = {} as Record<string, { label: string; color: string }>
		for (let i = 0; i < count; i++) {
			const [containerName] = sortedContainers[i]
			const hue = ((i * 360) / count) % 360
			chartConfig[containerName] = {
				label: containerName,
				color: `hsl(${hue}, 60%, 55%)`,
			}
		}

		// Return the same configuration for all chart types
		return {
			cpu: chartConfig,
			memory: chartConfig,
			network: chartConfig,
		}
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
			return sortedKeys.map((key) => ({
				label: key,
				dataKey: ({ stats }: SystemStatsRecord) => stats?.ni?.[key]?.[index],
				color: `hsl(${220 + (((sortedKeys.indexOf(key) * 360) / sortedKeys.length) % 360)}, 70%, 50%)`,

				opacity: 0.3,
			}))
		},
	}
}