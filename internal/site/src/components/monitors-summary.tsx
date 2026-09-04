import { useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { memo } from "react"
import { Card, CardContent } from "@/components/ui/card"
import { $router, Link } from "@/components/router"
import { getPagePath } from "@nanostores/router"
import { $monitorsSummary } from "@/lib/monitors"

export const MonitorsSummary = memo(() => {
	const { t } = useLingui()
	const summary = useStore($monitorsSummary)
	if (!summary) {
		return null
	}
	const total = Object.values(summary.counts).reduce((a, b) => a + b, 0)
	if (total === 0) {
		return null
	}
	return (
		<Card>
			<CardContent className="flex flex-wrap items-center gap-x-4 gap-y-1 py-3 text-sm">
				<Link href={getPagePath($router, "monitors")} className="font-medium hover:underline">
					{t`Monitors`}
				</Link>
				{summary.counts.up > 0 && (
					<span className="text-green-600">{t`${summary.counts.up} up`}</span>
				)}
				{summary.counts.down > 0 && (
					<span className="font-medium text-red-600">{t`${summary.counts.down} down`}</span>
				)}
				{summary.counts.warn > 0 && <span className="text-yellow-600">{t`${summary.counts.warn} warning`}</span>}
				{summary.counts.paused > 0 && <span className="text-muted-foreground">{t`${summary.counts.paused} paused`}</span>}
			</CardContent>
		</Card>
	)
})
