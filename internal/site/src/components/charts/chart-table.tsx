import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { createContext, type ReactNode, useContext, useEffect, useId, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { $userSettings } from "@/lib/stores"
import { chartTimeData } from "@/lib/utils"
import type { ChartTimes } from "@/types"
import {
	formatTableTime,
	pageRange,
	snapshotKey,
	sourceSignature,
	type TableCell,
	type TablePoint,
	tableRows,
} from "./table-model"
import { markTableUpdate } from "./table-updates"

export const ChartTitleContext = createContext("")

/** The record shapes the shared renderers already accept: system stats, container data or monitor stats. */
export type ChartRecord = { created: string | number | null }

export type ChartTableProps<T extends ChartRecord> = {
	chartData: { chartTime: ChartTimes; dataScope?: string; systemStats?: readonly T[] }
	customData?: readonly T[]
	/** Pre-filtered rows for callers whose chart data is a derived series (e.g. avg/min/max monitors). */
	tableData?: readonly T[]
	dataPoints?: readonly TablePoint<T>[]
	contentFormatter: (item: TableCell<T>, key: string) => ReactNode
	maxToggled?: boolean
	filter?: string
	showTotal?: boolean
}

const EMPTY: readonly never[] = []

/** Native table markup: no SVG, no grid role, no cell tabindex and no keyboard interception. */
export function ChartTable<T extends ChartRecord>(props: ChartTableProps<T>) {
	const title = useContext(ChartTitleContext)
	const { t, i18n } = useLingui()
	const settings = useStore($userSettings)
	const source = props.tableData ?? props.customData ?? props.chartData.systemStats ?? EMPTY
	const points = props.dataPoints ?? EMPTY
	const key = snapshotKey({
		scope: props.chartData.dataScope ?? location.pathname,
		time: props.chartData.chartTime,
		filter: props.filter,
		max: props.maxToggled,
		units: [settings.unitNet, settings.unitDisk, settings.unitTemp],
		locale: i18n.locale,
		title,
	})
	// Share the content signature across cards without tying it to callback identities.
	const signature = useMemo(() => sourceSignature(source), [source])
	const capture = () => ({
		key,
		signature,
		points: points.filter((p) => p.activeDot !== false).map((p) => p.label),
		rows: tableRows(source, points, !!props.showTotal),
		formatter: props.contentFormatter,
		total: !!props.showTotal,
		page: 0,
	})
	const [snapshot, setSnapshot] = useState(capture)
	// Selection changes are deliberate; live arrivals are not. Empty initial queries may prime once.
	if (snapshot.key !== key || (!snapshot.rows.length && source.some((r) => r.created != null))) {
		setSnapshot(capture())
	}
	const changed = snapshot.signature !== signature
	const updateId = useId()
	useEffect(() => {
		markTableUpdate(updateId, changed)
		return () => markTableUpdate(updateId, false)
	}, [updateId, changed])
	const range = pageRange(snapshot.rows.length, snapshot.page)
	const rows = snapshot.rows.slice(range.start, range.end)
	const noData = t`No data`
	const caption = title || t`Measurements`
	const refresh = t`Refresh table`
	// Interpolated in one message so translators keep the row and page numbers in a natural order.
	const rangeMessage = t`Rows ${snapshot.rows.length ? range.start + 1 : 0}–${range.end} of ${snapshot.rows.length}; page ${range.page + 1} of ${range.pages}`
	// Same translated range name the time period selector shows, not the raw key.
	const period = chartTimeData[props.chartData.chartTime]?.label() ?? props.chartData.chartTime
	return (
		<div className="space-y-3 min-w-0" data-metric-table>
			<div className="flex flex-wrap gap-3 items-center justify-between">
				<p className="text-sm" data-table-update>
					{changed ? (
						<Trans>New data available. The table stays unchanged until you refresh it.</Trans>
					) : (
						<Trans>Stable table snapshot.</Trans>
					)}
				</p>
				<Button
					type="button"
					variant="outline"
					aria-label={`${refresh}: ${caption}`}
					onClick={() => setSnapshot(capture())}
				>
					{refresh}
				</Button>
			</div>
			<p className="text-sm text-muted-foreground">
				<Trans>Units are shown with each value. Total sums the available series; missing values are not zero.</Trans>
			</p>
			{/* biome-ignore lint/a11y/noNoninteractiveTabindex: a horizontally scrollable region must be reachable
			    and scrollable with the keyboard alone; screen reader users navigate the table inside it. */}
			<section className="overflow-x-auto rounded-md border" aria-label={caption} tabIndex={0}>
				<table className="w-full text-sm text-start border-collapse">
					<caption className="text-start px-3 py-2 font-semibold">
						{caption} ({period})
					</caption>
					<thead className="bg-muted">
						<tr>
							<th scope="col" className="p-3 text-start">
								<Trans>Measurement time (local time zone)</Trans>
							</th>
							{snapshot.points.map((label, i) => (
								<th scope="col" className="p-3 text-start" key={`${label}:${i}`}>
									{label}
								</th>
							))}
							{snapshot.total && (
								<th scope="col" className="p-3 text-start">
									<Trans context="Metric table total">Total</Trans>
								</th>
							)}
						</tr>
					</thead>
					<tbody>
						{rows.map((row, index) => (
							<tr key={`${row.created}:${range.start + index}`} className="border-t even:bg-muted/30">
								<th scope="row" className="p-3 text-start font-normal whitespace-nowrap">
									{formatTableTime(row.created, i18n.locale) ?? noData}
								</th>
								{row.cells.map((cell, i) => (
									<td className="p-3 tabular-nums whitespace-nowrap" key={`${cell.key}:${i}`}>
										{cell.value === null ? noData : (snapshot.formatter(cell, cell.key) ?? noData)}
									</td>
								))}
							</tr>
						))}
					</tbody>
				</table>
			</section>
			{!snapshot.rows.length && <p>{noData}</p>}
			<nav className="flex flex-wrap gap-3 items-center" aria-label={`${t`Table pages`}: ${caption}`}>
				<Button
					type="button"
					variant="outline"
					aria-disabled={range.page === 0}
					onClick={() => range.page > 0 && setSnapshot((s) => ({ ...s, page: range.page - 1 }))}
				>
					<Trans>Previous page</Trans>
				</Button>
				<output>{rangeMessage}</output>
				<Button
					type="button"
					variant="outline"
					aria-disabled={range.page === range.pages - 1}
					onClick={() => range.page < range.pages - 1 && setSnapshot((s) => ({ ...s, page: range.page + 1 }))}
				>
					<Trans>Next page</Trans>
				</Button>
			</nav>
		</div>
	)
}
