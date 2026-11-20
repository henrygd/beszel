import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { atom } from "nanostores"
import { useEffect, useMemo, useState } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { ChartCard } from "@/components/routes/system/chart-card"
import { appendData, cache, getStats, makeContainerData } from "@/components/routes/system/chart-data"
import { Unit } from "@/lib/enums"
import { $allSystemsById, $chartTime, $direction, $userSettings } from "@/lib/stores"
import { chartTimeData, decimalString, formatBytes, parseSemVer, toFixedFloat } from "@/lib/utils"
import type { ChartData, ChartTimes, ContainerRecord, ContainerStatsRecord } from "@/types"

type ContainerStats = ContainerStatsRecord["stats"][number]

/** Historical CPU, memory and network charts for a single container */
export function ContainerCharts({ container }: { container: ContainerRecord }) {
	// local chart time so changing it here doesn't affect the system page. Realtime is not supported.
	const chartTimeStore = useMemo(() => atom<ChartTimes>($chartTime.get() === "1m" ? "1h" : $chartTime.get()), [])
	const chartTime = useStore(chartTimeStore)
	const direction = useStore($direction)
	const userSettings = useStore($userSettings)
	const [containerData, setContainerData] = useState<ChartData["containerData"]>([])
	const [loading, setLoading] = useState(true)

	const systemId = container.system
	const name = container.name
	const agentVersion = useMemo(() => parseSemVer($allSystemsById.get()[systemId]?.info?.v), [systemId])

	// fetch container stats, sharing the cache with the system page
	useEffect(() => {
		let cancelled = false
		const { expectedInterval } = chartTimeData[chartTime]
		const cacheKey = `${systemId}_${chartTime}_container_stats`
		const cached = cache.get(cacheKey) as ChartData["containerData"] | undefined

		if (cached?.length) {
			setContainerData(cached)
			setLoading(false)
			const lastCreated = cached.at(-1)?.created
			if (lastCreated && Date.now() - lastCreated < expectedInterval * 0.9) {
				return
			}
		} else {
			setContainerData([])
			setLoading(true)
		}

		getStats<ContainerStatsRecord>("container_stats", systemId, chartTime, cached)
			.then((records) => {
				if (cancelled) return
				let data = (cache.get(cacheKey) || []) as ChartData["containerData"]
				if (records.length) {
					data = appendData(data, makeContainerData(records), expectedInterval, 100)
					cache.set(cacheKey, data)
				}
				setContainerData(data)
			})
			.catch(console.error)
			.finally(() => !cancelled && setLoading(false))

		return () => {
			cancelled = true
		}
	}, [systemId, chartTime])

	const chartData: ChartData = useMemo(
		() => ({
			systemStats: [],
			containerData,
			chartTime,
			orientation: direction === "rtl" ? "right" : "left",
			agentVersion,
		}),
		[containerData, chartTime, direction, agentVersion]
	)

	const dataEmpty = !loading && containerData.length === 0
	const stats = (data: ChartData["containerData"][number]) => data[name] as ContainerStats | undefined

	return (
		<div className="w-full flex flex-col gap-3">
			<div className="flex items-center w-full">
				<ChartTimeSelect
					className="ms-auto w-full sm:w-44"
					agentVersion={agentVersion}
					chartTimeStore={chartTimeStore}
					allowRealtime={false}
				/>
			</div>
			<div className="grid gap-3 w-full">
				<ChartCard empty={dataEmpty} grid={false} title={t`CPU Usage`} description={t`Average CPU utilization`}>
					<AreaChartDefault
						chartData={chartData}
						customData={containerData}
						dataPoints={[
							{
								label: t`CPU Usage`,
								dataKey: (data) => stats(data)?.c ?? null,
								color: 1,
								opacity: 0.4,
							},
						]}
						tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
						contentFormatter={({ value }) => `${decimalString(value)}%`}
					/>
				</ChartCard>
				<ChartCard empty={dataEmpty} grid={false} title={t`Memory Usage`} description={t`Memory usage of container`}>
					<AreaChartDefault
						chartData={chartData}
						customData={containerData}
						dataPoints={[
							{
								label: t`Memory Usage`,
								dataKey: (data) => stats(data)?.m ?? null,
								color: 2,
								opacity: 0.4,
							},
						]}
						tickFormatter={(val) => {
							const { value, unit } = formatBytes(val, false, Unit.Bytes, true)
							return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
						}}
						contentFormatter={({ value }) => {
							const { value: convertedValue, unit } = formatBytes(value, false, Unit.Bytes, true)
							return `${decimalString(convertedValue)} ${unit}`
						}}
					/>
				</ChartCard>
				<ChartCard empty={dataEmpty} grid={false} title={t`Network I/O`} description={t`Network traffic of container`}>
					<AreaChartDefault
						chartData={chartData}
						customData={containerData}
						dataPoints={[
							{
								label: t`Sent`,
								dataKey: (data) => {
									const s = stats(data)
									return s ? (s.b?.[0] ?? (s.ns ?? 0) * 1024 * 1024) : null
								},
								color: 5,
								opacity: 0.2,
							},
							{
								label: t`Received`,
								dataKey: (data) => {
									const s = stats(data)
									return s ? (s.b?.[1] ?? (s.nr ?? 0) * 1024 * 1024) : null
								},
								color: 2,
								opacity: 0.2,
							},
						]}
						tickFormatter={(val) => {
							const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
							return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
						}}
						contentFormatter={({ value }) => {
							const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitNet, false)
							return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
						}}
						showTotal={true}
					/>
				</ChartCard>
			</div>
		</div>
	)
}
