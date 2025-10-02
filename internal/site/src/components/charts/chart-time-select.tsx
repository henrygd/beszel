import { useStore } from "@nanostores/react"
import { HistoryIcon } from "lucide-react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { $chartTime } from "@/lib/stores"
import { chartTimeData, cn, compareSemVer, parseSemVer } from "@/lib/utils"
import type { ChartTimes, SemVer } from "@/types"
import { memo } from "react"

export default memo(function ChartTimeSelect({
	className,
	agentVersion,
}: {
	className?: string
	agentVersion: SemVer
}) {
	const chartTime = useStore($chartTime)

	// remove chart times that are not supported by the system agent version
	const availableChartTimes = Object.entries(chartTimeData).filter(([_, { minVersion }]) => {
		if (!minVersion) {
			return true
		}
		return compareSemVer(agentVersion, parseSemVer(minVersion)) >= 0
	})

	return (
		<Select defaultValue="1h" value={chartTime} onValueChange={(value: ChartTimes) => $chartTime.set(value)}>
			<SelectTrigger className={cn(className, "relative ps-10 pe-5")}>
				<HistoryIcon className="h-4 w-4 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				{availableChartTimes.map(([value, { label }]) => (
					<SelectItem key={value} value={value}>
						{label()}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	)
})
