import { useEffect, useMemo, useState } from "react"
import { useLingui } from "@lingui/react/macro"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ResponseChart, LossChart } from "@/components/routes/system/charts/probes-charts"
import { chartTimeData } from "@/lib/utils"
import type { ChartData, ChartTimes, NetworkProbeRecord } from "@/types"
import { type ComparisonResult, fetchTargetComparison } from "@/lib/use-network-probes"
import { HistoryIcon } from "lucide-react"
import { useStore } from "@nanostores/react"
import { $direction } from "@/lib/stores"
import { parseSemVer } from "@/lib/utils"

const CHART_TIMES: ChartTimes[] = ["1h", "12h", "24h", "1w", "30d"]

const ZERO_VERSION = parseSemVer("0.0.0")

interface Props {
	target: string | null
	onClose: () => void
}

export default function TargetComparisonSheet({ target, onClose }: Props) {
	const { t } = useLingui()
	const [chartTime, setChartTime] = useState<ChartTimes>("1h")
	const [result, setResult] = useState<ComparisonResult>({ syntheticProbes: [], stats: [] })
	const direction = useStore($direction)

	useEffect(() => {
		if (!target) return
		setResult({ syntheticProbes: [], stats: [] })
		fetchTargetComparison(target, chartTime).then(setResult)
	}, [target, chartTime])

	const chartData: ChartData = useMemo(
		() => ({
			agentVersion: ZERO_VERSION,
			chartTime,
			orientation: direction === "rtl" ? "right" : "left",
		}),
		[chartTime, direction]
	)

	// Synthetic NetworkProbeRecord objects — charts only use id, name||target, and resAvg1h
	const syntheticProbes = useMemo(
		() =>
			result.syntheticProbes.map(
				(p) =>
					({
						id: p.id,
						name: p.name,
						target: p.name,
						resAvg1h: 0,
					}) as unknown as NetworkProbeRecord
			),
		[result.syntheticProbes]
	)

	const empty = result.stats.length < 2

	return (
		<Sheet open={!!target} onOpenChange={(open) => !open && onClose()}>
			<SheetContent className="w-full sm:max-w-220 overflow-auto p-4 sm:p-6">
				<SheetHeader className="mb-0 border-b p-0 pb-4">
					<SheetTitle className="truncate">
						{t`Compare`} — {target}
					</SheetTitle>
					<Select value={chartTime} onValueChange={(v) => setChartTime(v as ChartTimes)}>
						<SelectTrigger className="w-36 ps-9 relative">
							<HistoryIcon className="h-4 w-4 absolute start-3 top-1/2 -translate-y-1/2 opacity-70" />
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{CHART_TIMES.map((ct) => (
								<SelectItem key={ct} value={ct}>
									{chartTimeData[ct].label()}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</SheetHeader>

				{result.syntheticProbes.length === 0 && (
					<p className="text-sm text-muted-foreground py-6 text-center">{t`No data found for this target.`}</p>
				)}

				{syntheticProbes.length > 0 && (
					<div className="grid gap-4 pt-4">
						<ResponseChart
							probeStats={result.stats}
							probes={syntheticProbes}
							chartData={chartData}
							empty={empty}
							grid={false}
						/>
						<LossChart
							probeStats={result.stats}
							probes={syntheticProbes}
							chartData={chartData}
							empty={empty}
							grid={false}
						/>
					</div>
				)}
			</SheetContent>
		</Sheet>
	)
}
