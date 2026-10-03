import { t } from "@lingui/core/macro"
import Spinner from "@/components/spinner"

/**
 * Shown in place of a chart whose records contain no values (e.g. every probe failed).
 * The opacity-100 class hides ChartCard's loading spinner, same as a rendered chart.
 */
export function ChartNoValues() {
	return <Spinner msg={t`Waiting for enough records to display`} className="bg-card opacity-100" />
}
