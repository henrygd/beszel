import { useStore } from "@nanostores/react"
import { HistoryIcon } from "lucide-react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { $chartTime } from "@/lib/stores"
import { chartTimeData, cn, compareSemVer, parseSemVer } from "@/lib/utils"
import { pb } from "@/lib/api"
import type { ChartTimes, SemVer } from "@/types"
import { memo, useEffect, useState } from "react"

const retentionDaysMap: Record<string, number> = {
	"30d": 30,
	"60d": 60,
	"90d": 90,
	"180d": 180,
	"365d": 365,
	"730d": 730,
	"1095d": 1095,
	"1825d": 1825,
	never: Number.POSITIVE_INFINITY,
}
const chartDaysMap: Record<string, number> = {
	"30d": 30,
	"60d": 60,
	"90d": 90,
	"180d": 180,
	"365d": 365,
	"730d": 730,
	"1095d": 1095,
	"1825d": 1825,
}

export default memo(function ChartTimeSelect({
	className,
	agentVersion,
}: {
	className?: string
	agentVersion: SemVer
}) {
	const chartTime = useStore($chartTime)
	const [maxRetentionDays, setMaxRetentionDays] = useState<number>(Number.POSITIVE_INFINITY)

	useEffect(() => {
		let mounted = true
		pb.collection("hub_settings")
			.getFirstListItem("", { fields: "retention" })
			.then((rec) => {
				if (!mounted) return
				const r = (rec as unknown as { retention: string }).retention
				setMaxRetentionDays(retentionDaysMap[r] ?? 30)
			})
			.catch(() => {
				pb.collection("hub_settings")
					.getOne("hubsettings0001", { fields: "retention" })
					.then((rec) => {
						if (!mounted) return
						const r = (rec as unknown as { retention: string }).retention
						setMaxRetentionDays(retentionDaysMap[r] ?? 30)
					})
					.catch(() => {
						if (mounted) setMaxRetentionDays(30)
					})
			})
		return () => {
			mounted = false
		}
	}, [])

	// remove chart times that are not supported by the system agent version or beyond retention
	const availableChartTimes = Object.entries(chartTimeData).filter(([key, { minVersion }]) => {
		if (minVersion) {
			if (compareSemVer(agentVersion, parseSemVer(minVersion)) < 0) return false
		}
		const days = chartDaysMap[key]
		if (days !== undefined && days > maxRetentionDays) return false
		return true
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
