import { useMemo, useState } from "react"
import { useStore } from "@nanostores/react"
import type { ChartConfig } from "@/components/ui/chart"
import type { ChartData, SeriesLevel, SystemStats, SystemStatsRecord } from "@/types"
import type { DataPoint } from "./area-chart"
import { type ContainerChartConfigs, containerChartConfigs } from "@/lib/container-series"
import { $containerFilter } from "@/lib/stores"

export type { ContainerChartConfigs }

/**
 * Chart configurations for the container charts, memoized.
 *
 * The computation itself lives in lib/container-series.ts: it is a pure function of the points
 * and the level, which is what makes the ranking and the colours testable without a renderer.
 *
 * @param containerData - container stats points, already folded to `level`
 * @param level - series level the points were folded to
 */
export function useContainerChartConfigs(
	containerData: ChartData["containerSeries"],
	level: SeriesLevel = "container"
): ContainerChartConfigs {
	return useMemo(() => containerChartConfigs(containerData, level), [containerData, level])
}

/**
 * Whether any series has a numeric value in the data. Without one, recharts renders no y-axis
 * ticks, so the y-axis width is never measured and the chart would stay hidden.
 */
export function hasChartValues<T>(data: T[], dataPoints?: { dataKey: (data: T) => unknown }[]) {
	return !!dataPoints?.some(({ dataKey }) => data.some((record) => typeof dataKey(record) === "number"))
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
			div.textContent = str
			clearTimeout(timeout)
			timeout = setTimeout(() => {
				document.body.appendChild(div)
				const width = div.offsetWidth + 20 
				if (width > yAxisWidth) {
					setYAxisWidth(width)
				}
				document.body.removeChild(div)
			})
		}
		return str
	}
	return { yAxisWidth, updateYAxisWidth }
}

/** Subscribes to the container filter store and returns filtered DataPoints for container charts */
export function useContainerDataPoints(
	chartConfig: ChartConfig,
	// biome-ignore lint/suspicious/noExplicitAny: container data records have dynamic keys
	dataFn: (key: string, data: Record<string, any>) => number | null
) {
	const filter = useStore($containerFilter)
	const { dataPoints, filteredKeys } = useMemo(() => {
		const filterTerms = filter
			? filter
					.toLowerCase()
					.split(" ")
					.filter((term) => term.length > 0)
			: []
		const filtered = new Set<string>()
		const points = Object.keys(chartConfig).map((key) => {
			const isFiltered = filterTerms.length > 0 && !filterTerms.some((term) => key.toLowerCase().includes(term))
			if (isFiltered) filtered.add(key)
			return {
				label: key,
				// biome-ignore lint/suspicious/noExplicitAny: container data records have dynamic keys
				dataKey: (data: Record<string, any>) => dataFn(key, data),
				color: chartConfig[key].color ?? "",
				opacity: isFiltered ? 0.05 : 0.4,
				strokeOpacity: isFiltered ? 0.1 : 1,
				activeDot: !isFiltered,
				stackId: "a",
			}
		})
		return {
			// biome-ignore lint/suspicious/noExplicitAny: container data records have dynamic keys
			dataPoints: points as DataPoint<Record<string, any>>[],
			filteredKeys: filtered,
		}
	}, [chartConfig, filter])
	return { filter, dataPoints, filteredKeys }
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
