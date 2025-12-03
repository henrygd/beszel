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

// Connection stats data points for charting - main view (TCP & UDP totals)
export function useConnectionStatsMain() {
	return {
		tcp: {
			label: "TCP",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tt ?? 0,
			color: "hsl(220, 70%, 50%)",
			opacity: 0.3,
		},
		tcp6: {
			label: "TCP6",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tt6 ?? 0,
			color: "hsl(250, 70%, 50%)",
			opacity: 0.3,
		},
		udp: {
			label: "UDP",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.u ?? 0,
			color: "hsl(180, 70%, 50%)",
			opacity: 0.3,
		},
		udp6: {
			label: "UDP6",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.u6 ?? 0,
			color: "hsl(160, 70%, 50%)",
			opacity: 0.3,
		},
	}
}

// Connection stats data points for detailed sheet (all TCP states)
export function useConnectionStatsDetailed() {
	return {
		established: {
			label: "Established",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.te ?? 0,
			color: "hsl(142, 70%, 50%)",
			opacity: 0.3,
		},
		listening: {
			label: "Listening",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tl ?? 0,
			color: "hsl(280, 70%, 50%)",
			opacity: 0.3,
		},
		timeWait: {
			label: "Time Wait",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tw ?? 0,
			color: "hsl(30, 70%, 50%)",
			opacity: 0.3,
		},
		closeWait: {
			label: "Close Wait",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tcw ?? 0,
			color: "hsl(0, 70%, 50%)",
			opacity: 0.3,
		},
		finWait1: {
			label: "FIN Wait 1",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tf1 ?? 0,
			color: "hsl(320, 70%, 50%)",
			opacity: 0.3,
		},
		finWait2: {
			label: "FIN Wait 2",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tf2 ?? 0,
			color: "hsl(260, 70%, 50%)",
			opacity: 0.3,
		},
		synSent: {
			label: "SYN Sent",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.ts ?? 0,
			color: "hsl(200, 70%, 50%)",
			opacity: 0.3,
		},
		synRecv: {
			label: "SYN Recv",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tsr ?? 0,
			color: "hsl(160, 70%, 50%)",
			opacity: 0.3,
		},
		closing: {
			label: "Closing",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tcl ?? 0,
			color: "hsl(340, 70%, 50%)",
			opacity: 0.3,
		},
		lastAck: {
			label: "Last ACK",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tla ?? 0,
			color: "hsl(380, 70%, 50%)",
			opacity: 0.3,
		},
	}
}

// IPv6 Connection stats data points for detailed sheet (all TCP6 states)
export function useConnectionStatsIPv6() {
	return {
		established: {
			label: "Established",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.te6 ?? 0,
			color: "hsl(142, 70%, 50%)",
			opacity: 0.3,
		},
		listening: {
			label: "Listening",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tl6 ?? 0,
			color: "hsl(280, 70%, 50%)",
			opacity: 0.3,
		},
		timeWait: {
			label: "Time Wait",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tw6 ?? 0,
			color: "hsl(30, 70%, 50%)",
			opacity: 0.3,
		},
		closeWait: {
			label: "Close Wait",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tcw6 ?? 0,
			color: "hsl(0, 70%, 50%)",
			opacity: 0.3,
		},
		finWait1: {
			label: "FIN Wait 1",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tf16 ?? 0,
			color: "hsl(320, 70%, 50%)",
			opacity: 0.3,
		},
		finWait2: {
			label: "FIN Wait 2",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tf26 ?? 0,
			color: "hsl(260, 70%, 50%)",
			opacity: 0.3,
		},
		synSent: {
			label: "SYN Sent",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.ts6 ?? 0,
			color: "hsl(200, 70%, 50%)",
			opacity: 0.3,
		},
		synRecv: {
			label: "SYN Recv",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tsr6 ?? 0,
			color: "hsl(160, 70%, 50%)",
			opacity: 0.3,
		},
		closing: {
			label: "Closing",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tcl6 ?? 0,
			color: "hsl(340, 70%, 50%)",
			opacity: 0.3,
		},
		lastAck: {
			label: "Last ACK",
			dataKey: ({ stats }: SystemStatsRecord) => stats?.nc?.["_total"]?.tla6 ?? 0,
			color: "hsl(380, 70%, 50%)",
			opacity: 0.3,
		},
	}
}