import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { useNetworkInterfaces } from "@/components/charts/hooks"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData } from "@/types"
import { ChartCard } from "../system"

export default memo(function NetworkSheet({
	chartData,
	dataEmpty,
	grid,
	maxValues,
}: {
	chartData: ChartData
	dataEmpty: boolean
	grid: boolean
	maxValues: boolean
}) {
	const [netInterfacesOpen, setNetInterfacesOpen] = useState(false)
	const userSettings = useStore($userSettings)
	const netInterfaces = useNetworkInterfaces(chartData.systemStats.at(-1)?.stats?.ni ?? {})
	const showNetLegend = netInterfaces.length > 0
	const hasOpened = useRef(false)

	if (netInterfacesOpen && !hasOpened.current) {
		hasOpened.current = true
	}

	if (!netInterfaces.length) {
		return null
	}

	return (
		<Sheet open={netInterfacesOpen} onOpenChange={setNetInterfacesOpen}>
			<SheetTrigger asChild>
				<Button
					variant="outline"
					size="icon"
					className="shrink-0 absolute top-3 end-3 sm:inline-flex sm:top-0 sm:end-0"
				>
					<MoreHorizontalIcon />
				</Button>
			</SheetTrigger>
			{hasOpened.current && (
				<SheetContent className="overflow-auto w-200 !max-w-full p-4 sm:p-6">
					<ChartTimeSelect className="w-[calc(100%-2em)]" />
					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Download`}
						description={t`Network traffic of public interfaces`}
						legend={showNetLegend}
						className="min-h-auto"
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={maxValues}
							itemSorter={(a, b) => b.value - a.value}
							dataPoints={netInterfaces.data(1)}
							legend={showNetLegend}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
								return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
							}}
							contentFormatter={({ value }) => {
								const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitNet, false)
								return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
							}}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Upload`}
						description={t`Network traffic of public interfaces`}
						legend={showNetLegend}
						className="min-h-auto"
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={maxValues}
							itemSorter={(a, b) => b.value - a.value}
							legend={showNetLegend}
							dataPoints={netInterfaces.data(0)}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
								return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
							}}
							contentFormatter={({ value }) => {
								const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitNet, false)
								return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
							}}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Cumulative Download`}
						description={t`Total data received for each interface`}
						legend={showNetLegend}
						className="min-h-auto"
					>
						<AreaChartDefault
							chartData={chartData}
							legend={showNetLegend}
							dataPoints={netInterfaces.data(3)}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, false, userSettings.unitNet, false)
								return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
							}}
							contentFormatter={({ value }) => {
								const { value: convertedValue, unit } = formatBytes(value, false, userSettings.unitNet, false)
								return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
							}}
						/>
					</ChartCard>

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t`Cumulative Upload`}
						description={t`Total data sent for each interface`}
						legend={showNetLegend}
						className="min-h-auto"
					>
						<AreaChartDefault
							chartData={chartData}
							legend={showNetLegend}
							dataPoints={netInterfaces.data(2)}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, false, userSettings.unitNet, false)
								return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
							}}
							contentFormatter={({ value }) => {
								const { value: convertedValue, unit } = formatBytes(value, false, userSettings.unitNet, false)
								return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
							}}
						/>
					</ChartCard>
				</SheetContent>
			)}
		</Sheet>
	)
})
