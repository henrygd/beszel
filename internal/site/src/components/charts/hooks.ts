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
		const configs = {
			cpu: {} as ChartConfig,
			memory: {} as ChartConfig,
			network: {} as ChartConfig,
		}

		// Aggregate usage metrics for each container
		const totalUsage = {
			cpu: new Map<string, number>(),
			memory: new Map<string, number>(),
			network: new Map<string, number>(),
		}

		// Process each data point to calculate totals
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

				// Accumulate metrics for CPU, memory, and network
				const currentCpu = totalUsage.cpu.get(containerName) ?? 0
				const currentMemory = totalUsage.memory.get(containerName) ?? 0
				const currentNetwork = totalUsage.network.get(containerName) ?? 0

				totalUsage.cpu.set(containerName, currentCpu + (containerStats.c ?? 0))
				totalUsage.memory.set(containerName, currentMemory + (containerStats.m ?? 0))
				totalUsage.network.set(containerName, currentNetwork + (containerStats.nr ?? 0) + (containerStats.ns ?? 0))
			}
		}

		// Generate chart configurations for each metric type
		Object.entries(totalUsage).forEach(([chartType, usageMap]) => {
			const sortedContainers = Array.from(usageMap.entries()).sort(([, a], [, b]) => b - a)
			const chartConfig = {} as Record<string, { label: string; color: string }>
			const count = sortedContainers.length

			// Generate colors for each container
			for (let i = 0; i < count; i++) {
				const [containerName] = sortedContainers[i]
				const hue = ((i * 360) / count) % 360
				chartConfig[containerName] = {
					label: containerName,
					color: `hsl(${hue}, var(--chart-saturation), var(--chart-lightness))`,
				}
			}

			configs[chartType as keyof typeof configs] = chartConfig
		})

		return configs
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