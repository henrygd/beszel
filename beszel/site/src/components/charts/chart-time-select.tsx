import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { $chartTime } from "@/lib/stores"
import { chartTimeData, cn } from "@/lib/utils"
import { ChartTimes } from "@/types"
import { useStore } from "@nanostores/react"
import { HistoryIcon } from "lucide-react"

export default function ChartTimeSelect({ className }: { className?: string }) {
	const chartTime = useStore($chartTime)

	return (
		<Select defaultValue="1h" value={chartTime} onValueChange={(value: ChartTimes) => $chartTime.set(value)}>
			<SelectTrigger className={cn(className, "relative ps-10 pe-5")}>
				<HistoryIcon className="h-4 w-4 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				{Object.entries(chartTimeData).map(([value, { label }]) => (
					<SelectItem key={value} value={value}>
						{label()}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	)
}
