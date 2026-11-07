import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState } from "react"
import AreaChartDefault, { type DataPoint } from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { useNetworkInterfaces } from "@/components/charts/hooks"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData } from "@/types"
import { ChartCard } from "./chart-card"

export const packetTickFormatter = (val: number) => `${toFixedFloat(val, val >= 10 ? 0 : 2)} p/s`
// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item interop
export const packetContentFormatter = ({ value }: any) => `${decimalString(value, value >= 100 ? 0 : 2)} p/s`

interface SheetChart {
	title: string
	description: string
	dataPoints: DataPoint[]
	tickFormatter: (value: number) => string
	// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item interop
	contentFormatter: (item: any) => string
	/** sort tooltip items by value */
	sortByValue?: boolean
	/** chart has max values and should re-render when the max toggle changes */
	hasMax?: boolean
}

interface SheetProps {
	chartData: ChartData
	dataEmpty: boolean
	grid: boolean
	maxValues: boolean
}

/** Button that opens a sheet with per-interface charts */
function InterfacesSheet({
	title,
	chartData,
	dataEmpty,
	grid,
	maxValues,
	interfaceCount,
	charts,
}: SheetProps & { title: string; interfaceCount: number; charts: SheetChart[] }) {
	const [open, setOpen] = useState(false)
	const hasOpened = useRef(false)
	const showLegend = interfaceCount > 0 && interfaceCount < 15

	if (open && !hasOpened.current) {
		hasOpened.current = true
	}

	if (!interfaceCount) {
		return null
	}

	return (
		<Sheet open={open} onOpenChange={setOpen}>
			<DialogTitle className="sr-only">{title}</DialogTitle>
			<SheetTrigger asChild>
				<Button
					title={t`View more`}
					variant="outline"
					size="icon"
					className="shrink-0 max-sm:absolute max-sm:top-0 max-sm:end-0"
				>
					<MoreHorizontalIcon />
				</Button>
			</SheetTrigger>
			{hasOpened.current && (
				<SheetContent aria-describedby={undefined} className="overflow-auto w-200 !max-w-full p-4 sm:p-6">
					<ChartTimeSelect className="w-[calc(100%-2em)] bg-card" agentVersion={chartData.agentVersion} />
					{charts.map((chart) => (
						<ChartCard
							key={chart.title}
							empty={dataEmpty}
							grid={grid}
							title={chart.title}
							description={chart.description}
							legend={showLegend}
							className="min-h-auto"
						>
							<AreaChartDefault
								chartData={chartData}
								maxToggled={chart.hasMax ? maxValues : undefined}
								itemSorter={chart.sortByValue ? (a, b) => b.value - a.value : undefined}
								legend={showLegend}
								dataPoints={chart.dataPoints}
								tickFormatter={chart.tickFormatter}
								contentFormatter={chart.contentFormatter}
							/>
						</ChartCard>
					))}
				</SheetContent>
			)}
		</Sheet>
	)
}

export default memo(function NetworkSheet(props: SheetProps) {
	const userSettings = useStore($userSettings)
	const netInterfaces = useNetworkInterfaces(props.chartData.systemStats.at(-1)?.stats?.ni ?? {})

	const bytesFormatters = (perSecond: boolean) => ({
		tickFormatter: (val: number) => {
			const { value, unit } = formatBytes(val, perSecond, userSettings.unitNet, false)
			return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
		},
		// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip item interop
		contentFormatter: ({ value }: any) => {
			const { value: convertedValue, unit } = formatBytes(value, perSecond, userSettings.unitNet, false)
			return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
		},
	})

	return (
		<InterfacesSheet
			{...props}
			title={t`Network traffic of public interfaces`}
			interfaceCount={netInterfaces.length}
			charts={[
				{
					title: t`Download`,
					description: t`Network traffic of public interfaces`,
					dataPoints: netInterfaces.data(1),
					sortByValue: true,
					hasMax: true,
					...bytesFormatters(true),
				},
				{
					title: t`Upload`,
					description: t`Network traffic of public interfaces`,
					dataPoints: netInterfaces.data(0),
					sortByValue: true,
					hasMax: true,
					...bytesFormatters(true),
				},
				{
					title: t`Cumulative Download`,
					description: t`Total data received for each interface`,
					dataPoints: netInterfaces.data(3),
					...bytesFormatters(false),
				},
				{
					title: t`Cumulative Upload`,
					description: t`Total data sent for each interface`,
					dataPoints: netInterfaces.data(2),
					...bytesFormatters(false),
				},
			]}
		/>
	)
})

export const PacketsSheet = memo(function PacketsSheet(props: SheetProps) {
	const netInterfaces = useNetworkInterfaces(props.chartData.systemStats.at(-1)?.stats?.ni ?? {})
	const formatters = { tickFormatter: packetTickFormatter, contentFormatter: packetContentFormatter }

	return (
		<InterfacesSheet
			{...props}
			title={t`Packets per interface`}
			interfaceCount={netInterfaces.length}
			charts={[
				{
					title: t`Packets Received`,
					description: t`Packets received per second on each interface`,
					dataPoints: netInterfaces.packets(1),
					sortByValue: true,
					...formatters,
				},
				{
					title: t`Packets Sent`,
					description: t`Packets sent per second on each interface`,
					dataPoints: netInterfaces.packets(0),
					sortByValue: true,
					...formatters,
				},
			]}
		/>
	)
})

export const ErrorsSheet = memo(function ErrorsSheet(props: SheetProps) {
	const netInterfaces = useNetworkInterfaces(props.chartData.systemStats.at(-1)?.stats?.ni ?? {})
	const formatters = { tickFormatter: packetTickFormatter, contentFormatter: packetContentFormatter }

	return (
		<InterfacesSheet
			{...props}
			title={t`Errors and discards per interface`}
			interfaceCount={netInterfaces.length}
			charts={[
				{
					title: t`Errors Received`,
					description: t`Errors on received packets per second on each interface`,
					dataPoints: netInterfaces.packets(3),
					sortByValue: true,
					...formatters,
				},
				{
					title: t`Errors Sent`,
					description: t`Errors on sent packets per second on each interface`,
					dataPoints: netInterfaces.packets(2),
					sortByValue: true,
					...formatters,
				},
				{
					title: t`Discards Received`,
					description: t`Received packets discarded per second on each interface`,
					dataPoints: netInterfaces.packets(5),
					sortByValue: true,
					...formatters,
				},
				{
					title: t`Discards Sent`,
					description: t`Sent packets discarded per second on each interface`,
					dataPoints: netInterfaces.packets(4),
					sortByValue: true,
					...formatters,
				},
			]}
		/>
	)
})
