import { t } from "@lingui/core/macro"
import type { Ref } from "react"
import Spinner from "@/components/spinner"

/**
 * Shown in place of a chart whose records contain no values (e.g. every probe failed).
 * The opacity-100 class hides ChartCard's loading spinner, same as a rendered chart.
 * Takes the chart's intersection observer ref so the chart keeps updating once values arrive.
 */
export function ChartNoValues({ ref }: { ref?: Ref<HTMLDivElement> }) {
	return (
		<div ref={ref} className="absolute inset-0">
			<Spinner msg={t`Waiting for enough records to display`} className="bg-card opacity-100" />
		</div>
	)
}
