import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useMemo, useState } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import { useContainerDataPoints } from "@/components/charts/hooks"
import { Separator } from "@/components/ui/separator"
import { pinnedAxisDomain, type ChartConfig } from "@/components/ui/chart"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData } from "@/types"
import { ChartCard, FilterBar } from "../chart-card"

export type ContainerDiskIoMetric = "total" | "read" | "write"

type ContainerDiskStats = { d?: [number, number] }

export function getContainerDiskIoValue(
	stats: ContainerDiskStats | undefined,
	metric: ContainerDiskIoMetric
): number | undefined {
	if (stats?.d === undefined) return undefined
	if (metric === "read") return stats.d[0]
	if (metric === "write") return stats.d[1]
	return stats.d[0] + stats.d[1]
}

export function hasContainerDiskIoData(containerData: ChartData["containerData"]): boolean {
	return containerData.some((point) =>
		Object.entries(point).some(
			([key, stats]) => key !== "created" && getContainerDiskIoValue(stats as ContainerDiskStats, "total") !== undefined
		)
	)
}

export function ContainerDiskIoChart({
	chartData,
	grid,
	dataEmpty,
	diskConfig,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	diskConfig: ChartConfig
}) {
	const [metric, setMetric] = useState<ContainerDiskIoMetric>("total")
	const userSettings = $userSettings.get()
	const metricConfig = useMemo(() => ({ ...diskConfig }), [diskConfig, metric])
	const { filter, dataPoints, filteredKeys } = useContainerDataPoints(metricConfig, (key, data) =>
		getContainerDiskIoValue(data[key], metric)
	)

	const contentFormatter = useMemo(() => {
		const formatReadWrite = (stats: ContainerDiskStats | undefined) => {
			if (stats?.d === undefined) return null
			const { value: readValue, unit: readUnit } = formatBytes(stats.d[0], true, userSettings.unitDisk, false)
			const { value: writeValue, unit: writeUnit } = formatBytes(stats.d[1], true, userSettings.unitDisk, false)
			return (
				<span className="flex">
					{decimalString(readValue)} {readUnit}
					<span className="opacity-70 ms-0.5"> {t`Read`} </span>
					<Separator orientation="vertical" className="h-3 mx-1.5 bg-primary/40" />
					{decimalString(writeValue)} {writeUnit}
					<span className="opacity-70 ms-0.5"> {t`Write`}</span>
				</span>
			)
		}

		// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item
		return (item: any, key: string) => {
			if (key !== "__total__") return formatReadWrite(item?.payload?.[key])
			let read = 0
			let write = 0
			let supported = false
			for (const [containerKey, stats] of Object.entries(item?.payload ?? {})) {
				if (filteredKeys.has(containerKey)) continue
				const disk = (stats as ContainerDiskStats | undefined)?.d
				if (disk === undefined) continue
				supported = true
				read += disk[0]
				write += disk[1]
			}
			return supported ? formatReadWrite({ d: [read, write] }) : null
		}
	}, [filteredKeys, userSettings.unitDisk])

	if (!hasContainerDiskIoData(chartData.containerData)) return null

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Container Disk I/O`}
			description={t`Disk read and write rates of containers`}
			cornerEl={
				<div className="flex gap-2">
					<Select value={metric} onValueChange={(value) => setMetric(value as ContainerDiskIoMetric)}>
						<SelectTrigger className="w-28" aria-label={t`Container Disk I/O`}>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="total">
								<Trans>Total</Trans>
							</SelectItem>
							<SelectItem value="read">
								<Trans>Read</Trans>
							</SelectItem>
							<SelectItem value="write">
								<Trans>Write</Trans>
							</SelectItem>
						</SelectContent>
					</Select>
					<div className="relative">
						<FilterBar />
					</div>
				</div>
			}
		>
			<AreaChartDefault
				key={metric}
				chartData={chartData}
				customData={chartData.containerData}
				dataPoints={dataPoints}
				tickFormatter={(value) => {
					const formatted = formatBytes(value, true, userSettings.unitDisk, false)
					return `${toFixedFloat(formatted.value, formatted.value >= 10 ? 0 : 1)} ${formatted.unit}`
				}}
				contentFormatter={contentFormatter}
				domain={pinnedAxisDomain()}
				showTotal={true}
				reverseStackOrder={true}
				filter={filter}
				truncate={true}
				itemSorter={(a, b) => b.value - a.value}
			/>
		</ChartCard>
	)
}
