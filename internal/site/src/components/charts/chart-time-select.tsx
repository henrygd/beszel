import { useStore } from "@nanostores/react"
import { CalendarIcon, HistoryIcon } from "lucide-react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { $chartTime, $customRange } from "@/lib/stores"
import { chartTimeData, cn, compareSemVer, getChartTypeForDuration, parseSemVer } from "@/lib/utils"
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
	const customRange = useStore($customRange)
	const [maxRetentionDays, setMaxRetentionDays] = useState(Number.POSITIVE_INFINITY)
	const [customFrom, setCustomFrom] = useState("")
	const [customTo, setCustomTo] = useState("")

	useEffect(() => {
		let mounted = true
		pb.send<{ retention: string }>("/api/beszel/retention", {})
			.then((res) => {
				if (!mounted) return
				setMaxRetentionDays(retentionDaysMap[res.retention] ?? 30)
			})
			.catch(() => {
				// fallback for older hub without endpoint: read collection directly
				pb.collection("hub_settings")
					.getFirstListItem("", { fields: "retention" })
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
		if (key === "custom") return false
		if (minVersion) {
			if (compareSemVer(agentVersion, parseSemVer(minVersion)) < 0) return false
		}
		const days = chartDaysMap[key]
		if (days !== undefined && days > maxRetentionDays) return false
		return true
	})

	useEffect(() => {
		if (chartTime === "custom" && customRange) {
			setCustomFrom(customRange.from.slice(0, 16))
			setCustomTo(customRange.to.slice(0, 16))
		}
	}, [chartTime, customRange])

	function handleValueChange(value: ChartTimes) {
		if (value === "custom") {
			$chartTime.set("custom")
			return
		}
		$chartTime.set(value)
	}

	function applyCustom() {
		if (!customFrom || !customTo) return
		let from = new Date(customFrom)
		const to = new Date(customTo)
		if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime()) || from >= to) return
		if (to.getTime() > Date.now()) to.setTime(Date.now())
		// clamp from to retention window (#3)
		if (Number.isFinite(maxRetentionDays)) {
			const retentionMs = maxRetentionDays * 24 * 60 * 60 * 1000
			const earliest = Date.now() - retentionMs
			if (from.getTime() < earliest) from = new Date(earliest)
			if (from >= to) return
		}
		$customRange.set({ from: from.toISOString(), to: to.toISOString() })
		$chartTime.set("custom")
	}

	return (
		<div className="flex flex-wrap gap-2 items-end">
			<Select defaultValue="1h" value={chartTime} onValueChange={handleValueChange}>
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
					<SelectItem value="custom">
						<span className="flex items-center gap-2">
							<CalendarIcon className="h-4 w-4" /> Custom range…
						</span>
					</SelectItem>
				</SelectContent>
			</Select>
			{chartTime === "custom" && (
				<div className="flex flex-wrap gap-2 items-end rounded-md border p-2 bg-muted/20">
					<div className="grid gap-1">
						<Label htmlFor="custom-from" className="text-xs">
							From
						</Label>
						<Input
							id="custom-from"
							type="datetime-local"
							value={customFrom}
							onChange={(e) => setCustomFrom(e.target.value)}
							className="h-8 w-44"
						/>
					</div>
					<div className="grid gap-1">
						<Label htmlFor="custom-to" className="text-xs">
							To
						</Label>
						<Input
							id="custom-to"
							type="datetime-local"
							value={customTo}
							onChange={(e) => setCustomTo(e.target.value)}
							className="h-8 w-44"
						/>
					</div>
					<Button type="button" size="sm" onClick={applyCustom} className="h-8">
						Apply
					</Button>
					{customRange && chartTime === "custom" && (
						<span className="text-xs text-muted-foreground">
							{getChartTypeForDuration(new Date(customRange.to).getTime() - new Date(customRange.from).getTime())} •{" "}
							{customRange.from.slice(0, 10)} → {customRange.to.slice(0, 10)}
						</span>
					)}
				</div>
			)}
		</div>
	)
})
