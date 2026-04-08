import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { MoreHorizontalIcon } from "lucide-react"
import { memo, useRef, useState } from "react"
import AreaChartDefault from "@/components/charts/area-chart"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { DialogTitle } from "@/components/ui/dialog"
import { $userSettings } from "@/lib/stores"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import { ChartCard, SelectAvgMax } from "@/components/routes/system/chart-card"
import type { SystemData } from "@/components/routes/system/use-system-data"
import { diskDataFns, DiskUtilizationChart } from "./charts/disk-charts"
import { pinnedAxisDomain } from "@/components/ui/chart"

export default memo(function DiskIOSheet({
	systemData,
	extraFsName,
	title,
	description,
}: {
	systemData: SystemData
	extraFsName?: string
	title: string
	description: string
}) {
	const { chartData, grid, dataEmpty, showMax, maxValues, isLongerChart } = systemData
	const userSettings = useStore($userSettings)

	const [sheetOpen, setSheetOpen] = useState(false)

	const hasOpened = useRef(false)

	if (sheetOpen && !hasOpened.current) {
		hasOpened.current = true
	}

	// throughput functions, with extra fs variants if needed
	let readFn = showMax ? diskDataFns.readMax : diskDataFns.read
	let writeFn = showMax ? diskDataFns.writeMax : diskDataFns.write
	if (extraFsName) {
		readFn = showMax ? diskDataFns.extraReadMax(extraFsName) : diskDataFns.extraRead(extraFsName)
		writeFn = showMax ? diskDataFns.extraWriteMax(extraFsName) : diskDataFns.extraWrite(extraFsName)
	}

	// read and write time functions, with extra fs variants if needed
	let readTimeFn = showMax ? diskDataFns.readTimeMax : diskDataFns.readTime
	let writeTimeFn = showMax ? diskDataFns.writeTimeMax : diskDataFns.writeTime
	if (extraFsName) {
		readTimeFn = showMax ? diskDataFns.extraReadTimeMax(extraFsName) : diskDataFns.extraReadTime(extraFsName)
		writeTimeFn = showMax ? diskDataFns.extraWriteTimeMax(extraFsName) : diskDataFns.extraWriteTime(extraFsName)
	}

	// I/O await functions, with extra fs variants if needed
	let rAwaitFn = showMax ? diskDataFns.rAwaitMax : diskDataFns.rAwait
	let wAwaitFn = showMax ? diskDataFns.wAwaitMax : diskDataFns.wAwait
	if (extraFsName) {
		rAwaitFn = showMax ? diskDataFns.extraRAwaitMax(extraFsName) : diskDataFns.extraRAwait(extraFsName)
		wAwaitFn = showMax ? diskDataFns.extraWAwaitMax(extraFsName) : diskDataFns.extraWAwait(extraFsName)
	}

	// weighted I/O function, with extra fs variant if needed
	let weightedIOFn = showMax ? diskDataFns.weightedIOMax : diskDataFns.weightedIO
	if (extraFsName) {
		weightedIOFn = showMax ? diskDataFns.extraWeightedIOMax(extraFsName) : diskDataFns.extraWeightedIO(extraFsName)
	}

	// check for availability of I/O metrics
	let hasUtilization = false
	let hasAwait = false
	let hasWeightedIO = false
	for (const record of chartData.systemStats ?? []) {
		const dios = record.stats?.dios
		if ((dios?.at(2) ?? 0) > 0) hasUtilization = true
		if ((dios?.at(3) ?? 0) > 0) hasAwait = true
		if ((dios?.at(5) ?? 0) > 0) hasWeightedIO = true
		if (hasUtilization && hasAwait && hasWeightedIO) {
			break
		}
	}

	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null

	const chartProps = { syncId: "io" }

	const queueDepthTranslation = t({ message: "Queue Depth", context: "Disk I/O average queue depth" })

	return (
		<Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
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

					<ChartCard
						className="min-h-auto"
						empty={dataEmpty}
						grid={grid}
						title={title}
						description={description}
						cornerEl={maxValSelect}
						// legend={true}
					>
						<AreaChartDefault
							chartData={chartData}
							maxToggled={showMax}
							chartProps={chartProps}
							showTotal={true}
							domain={pinnedAxisDomain()}
							itemSorter={(a, b) => a.order - b.order}
							reverseStackOrder={true}
							dataPoints={[
								{
									label: t`Write`,
									dataKey: writeFn,
									color: 3,
									opacity: 0.4,
									stackId: 0,
									order: 0,
								},
								{
									label: t`Read`,
									dataKey: readFn,
									color: 1,
									opacity: 0.4,
									stackId: 0,
									order: 1,
								},
							]}
							tickFormatter={(val) => {
								const { value, unit } = formatBytes(val, true, userSettings.unitDisk, false)
								return `${toFixedFloat(value, value >= 10 ? 0 : 1)} ${unit}`
							}}
							contentFormatter={({ value }) => {
								const { value: convertedValue, unit } = formatBytes(value, true, userSettings.unitDisk, false)
								return `${decimalString(convertedValue, convertedValue >= 100 ? 1 : 2)} ${unit}`
							}}
						/>
					</ChartCard>

					{hasUtilization && <DiskUtilizationChart systemData={systemData} extraFsName={extraFsName} />}

					<ChartCard
						empty={dataEmpty}
						grid={grid}
						title={t({ message: "I/O Time", context: "Disk I/O total time spent on read/write" })}
						description={t({
							message: "Total time spent on read/write (can exceed 100%)",
							context: "Disk I/O",
						})}
						className="min-h-auto"
						cornerEl={maxValSelect}
					>
						<AreaChartDefault
							chartData={chartData}
							domain={pinnedAxisDomain()}
							tickFormatter={(val) => `${toFixedFloat(val, 2)}%`}
							contentFormatter={({ value }) => `${decimalString(value)}%`}
							maxToggled={showMax}
							chartProps={chartProps}
							showTotal={true}
							itemSorter={(a, b) => a.order - b.order}
							reverseStackOrder={true}
							dataPoints={[
								{
									label: t`Write`,
									dataKey: writeTimeFn,
									color: 3,
									opacity: 0.4,
									stackId: 0,
									order: 0,
								},
								{
									label: t`Read`,
									dataKey: readTimeFn,
									color: 1,
									opacity: 0.4,
									stackId: 0,
									order: 1,
								},
							]}
						/>
					</ChartCard>

					{hasWeightedIO && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={queueDepthTranslation}
							description={t`Average number of I/O operations waiting to be serviced`}
							className="min-h-auto"
							cornerEl={maxValSelect}
						>
							<AreaChartDefault
								chartData={chartData}
								domain={pinnedAxisDomain()}
								tickFormatter={(val) => `${toFixedFloat(val, 2)}`}
								contentFormatter={({ value }) => decimalString(value, value < 10 ? 3 : 2)}
								maxToggled={showMax}
								chartProps={chartProps}
								dataPoints={[
									{
										label: queueDepthTranslation,
										dataKey: weightedIOFn,
										color: 1,
										opacity: 0.4,
									},
								]}
							/>
						</ChartCard>
					)}

					{hasAwait && (
						<ChartCard
							empty={dataEmpty}
							grid={grid}
							title={t({ message: "I/O Await", context: "Disk I/O average operation time (iostat await)" })}
							description={t({
								message: "Average queue to completion time per operation",
								context: "Disk I/O average operation time (iostat await)",
							})}
							className="min-h-auto"
							cornerEl={maxValSelect}
							// legend={true}
						>
							<AreaChartDefault
								chartData={chartData}
								domain={pinnedAxisDomain()}
								tickFormatter={(val) => `${toFixedFloat(val, 2)} ms`}
								contentFormatter={({ value }) => `${decimalString(value)} ms`}
								maxToggled={showMax}
								chartProps={chartProps}
								dataPoints={[
									{
										label: t`Write`,
										dataKey: wAwaitFn,
										color: 3,
										opacity: 0.3,
									},
									{
										label: t`Read`,
										dataKey: rAwaitFn,
										color: 1,
										opacity: 0.3,
									},
								]}
							/>
						</ChartCard>
					)}
				</SheetContent>
			)}
		</Sheet>
	)
})
