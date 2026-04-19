import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
import { pb } from "@/lib/api"
import { useStore } from "@nanostores/react"
import { $chartTime } from "@/lib/stores"
import { chartTimeData, cn, toFixedFloat, decimalString, getVisualStringWidth } from "@/lib/utils"
import { Card, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { useToast } from "@/components/ui/use-toast"
import { appendData } from "./chart-data"
import { AddProbeDialog } from "./probe-dialog"
import { ChartCard } from "./chart-card"
import LineChartDefault, { type DataPoint } from "@/components/charts/line-chart"
import { pinnedAxisDomain } from "@/components/ui/chart"
import type { ChartData, NetworkProbeRecord, NetworkProbeStatsRecord, SystemRecord } from "@/types"
import {
	type Row,
	type SortingState,
	flexRender,
	getCoreRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table"
import { useVirtualizer, type VirtualItem } from "@tanstack/react-virtual"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { getProbeColumns, type ProbeRow } from "./network-probes-columns"

function probeKey(p: NetworkProbeRecord) {
	if (p.protocol === "tcp") return `${p.protocol}:${p.target}:${p.port}`
	return `${p.protocol}:${p.target}`
}

export default function NetworkProbes({
	system,
	chartData,
	grid,
	realtimeProbeStats,
}: {
	system: SystemRecord
	chartData: ChartData
	grid: boolean
	realtimeProbeStats?: NetworkProbeStatsRecord[]
}) {
	const systemId = system.id
	const [probes, setProbes] = useState<NetworkProbeRecord[]>([])
	const [stats, setStats] = useState<NetworkProbeStatsRecord[]>([])
	const [latestResults, setLatestResults] = useState<Record<string, { avg: number; loss: number }>>({})
	const chartTime = useStore($chartTime)
	const { toast } = useToast()
	const { t } = useLingui()

	const fetchProbes = useCallback(() => {
		pb.collection<NetworkProbeRecord>("network_probes")
			.getList(0, 2000, {
				fields: "id,name,target,protocol,port,interval,enabled,updated",
				filter: systemId ? pb.filter("system={:system}", { system: systemId }) : undefined,
			})
			.then((res) => setProbes(res.items))
			.catch(() => setProbes([]))
	}, [systemId])

	useEffect(() => {
		fetchProbes()
	}, [fetchProbes])

	// Build set of current probe keys to filter out deleted probes from stats
	const activeProbeKeys = useMemo(() => new Set(probes.map(probeKey)), [probes])

	// Use realtime probe stats when in 1m mode
	useEffect(() => {
		if (chartTime !== "1m" || !realtimeProbeStats) {
			return
		}
		// Filter stats to only include currently active probes, preserving gap markers
		const data: NetworkProbeStatsRecord[] = realtimeProbeStats.map((r) => {
			if (!r.stats) {
				return r // preserve gap markers from appendData
			}
			const filtered: NetworkProbeStatsRecord["stats"] = {}
			for (const [key, val] of Object.entries(r.stats)) {
				if (activeProbeKeys.has(key)) {
					filtered[key] = val
				}
			}
			return { stats: filtered, created: r.created }
		})
		setStats(data)
		// Use last non-gap entry for latest results
		for (let i = data.length - 1; i >= 0; i--) {
			if (data[i].stats) {
				const latest: Record<string, { avg: number; loss: number }> = {}
				for (const [key, val] of Object.entries(data[i].stats)) {
					latest[key] = { avg: val.avg, loss: val.loss }
				}
				setLatestResults(latest)
				break
			}
		}
	}, [chartTime, realtimeProbeStats, activeProbeKeys])

	// Fetch probe stats based on chart time (skip in realtime mode)
	useEffect(() => {
		if (probes.length === 0) {
			setStats([])
			setLatestResults({})
			return
		}
		if (chartTime === "1m") {
			return
		}
		const controller = new AbortController()
		const { type: statsType = "1m", expectedInterval } = chartTimeData[chartTime] ?? {}

		pb.send<{ stats: NetworkProbeStatsRecord["stats"]; created: string }[]>("/api/beszel/network-probe-stats", {
			query: { system: systemId, type: statsType },
			signal: controller.signal,
		})
			.then((raw) => {
				// Filter stats to only include currently active probes
				const mapped: NetworkProbeStatsRecord[] = raw.map((r) => {
					const filtered: NetworkProbeStatsRecord["stats"] = {}
					for (const [key, val] of Object.entries(r.stats)) {
						if (activeProbeKeys.has(key)) {
							filtered[key] = val
						}
					}
					return { stats: filtered, created: new Date(r.created).getTime() }
				})
				// Apply gap detection — inserts null markers where data is missing
				const data = appendData([] as NetworkProbeStatsRecord[], mapped, expectedInterval)
				setStats(data)
				if (mapped.length > 0) {
					const last = mapped[mapped.length - 1].stats
					const latest: Record<string, { avg: number; loss: number }> = {}
					for (const [key, val] of Object.entries(last)) {
						latest[key] = { avg: val.avg, loss: val.loss }
					}
					setLatestResults(latest)
				}
			})
			.catch(() => setStats([]))

		return () => controller.abort()
	}, [system, chartTime, probes, activeProbeKeys])

	const deleteProbe = useCallback(
		async (id: string) => {
			try {
				await pb.collection("network_probes").delete(id)
				// fetchProbes()
			} catch (err: unknown) {
				toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
			}
		},
		[systemId, t]
	)

	const dataPoints: DataPoint<NetworkProbeStatsRecord>[] = useMemo(() => {
		const count = probes.length
		return probes.map((p, i) => {
			const key = probeKey(p)
			return {
				label: p.name || p.target,
				dataKey: (record: NetworkProbeStatsRecord) => record.stats?.[key]?.avg ?? null,
				color: count <= 5 ? i + 1 : `hsl(${(i * 360) / count}, var(--chart-saturation), var(--chart-lightness))`,
			}
		})
	}, [probes])

	const { longestName, longestTarget } = useMemo(() => {
		let longestName = 0
		let longestTarget = 0
		for (const p of probes) {
			longestName = Math.max(longestName, getVisualStringWidth(p.name || p.target))
			longestTarget = Math.max(longestTarget, getVisualStringWidth(p.target))
		}
		return { longestName, longestTarget }
	}, [probes])

	const columns = useMemo(
		() => getProbeColumns(deleteProbe, longestName, longestTarget),
		[deleteProbe, longestName, longestTarget]
	)

	const tableData: ProbeRow[] = useMemo(
		() =>
			probes.map((p) => {
				const key = probeKey(p)
				const result = latestResults[key]
				return { ...p, key, latency: result?.avg, loss: result?.loss }
			}),
		[probes, latestResults]
	)

	const [sorting, setSorting] = useState<SortingState>([{ id: "name", desc: false }])

	const table = useReactTable({
		data: tableData,
		columns,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		onSortingChange: setSorting,
		defaultColumn: {
			sortUndefined: "last",
			size: 100,
			minSize: 0,
		},
		state: { sorting },
	})

	const rows = table.getRowModel().rows
	const visibleColumns = table.getVisibleLeafColumns()

	// if (probes.length === 0 && stats.length === 0) {
	// 	return (
	// 		<Card className="w-full px-3 py-5 sm:py-6 sm:px-6">
	// 			<CardHeader className="p-0 mb-3 sm:mb-4">
	// 				<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
	// 					<div className="px-2 sm:px-1">
	// 						<CardTitle className="mb-2">
	// 							<Trans>Network Probes</Trans>
	// 						</CardTitle>
	// 						<CardDescription>
	// 							<Trans>ICMP/TCP/HTTP latency monitoring from this agent</Trans>
	// 						</CardDescription>
	// 					</div>
	// 					{/* <div className="relative ms-auto w-full max-w-full md:w-64"> */}
	// 					<AddProbeDialog systemId={systemId} onCreated={fetchProbes} />
	// 					{/* </div> */}
	// 				</div>
	// 			</CardHeader>
	// 		</Card>
	// 	)
	// }

	return (
		<div className="grid gap-4">
			<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
				<CardHeader className="p-0 mb-3 sm:mb-4">
					<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end md:justify-between">
						<div className="px-2 sm:px-1">
							<CardTitle>
								<Trans>Network Probes</Trans>
							</CardTitle>
							<CardDescription className="mt-1.5">
								<Trans>ICMP/TCP/HTTP latency monitoring from this agent</Trans>
							</CardDescription>
						</div>
						<AddProbeDialog systemId={systemId} onCreated={fetchProbes} />
					</div>
				</CardHeader>

				<ProbesTable table={table} rows={rows} colLength={visibleColumns.length} />
			</Card>

			{stats.length > 0 && (
				<ChartCard title={t`Latency`} description={t`Average round-trip time (ms)`} grid={grid}>
					<LineChartDefault
						chartData={chartData}
						customData={stats}
						dataPoints={dataPoints}
						domain={pinnedAxisDomain()}
						connectNulls
						tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
						contentFormatter={({ value }) => `${decimalString(value, 2)} ms`}
						legend
					/>
				</ChartCard>
			)}
		</div>
	)
}

const ProbesTable = memo(function ProbesTable({
	table,
	rows,
	colLength,
}: {
	table: ReturnType<typeof useReactTable<ProbeRow>>
	rows: Row<ProbeRow>[]
	colLength: number
}) {
	const scrollRef = useRef<HTMLDivElement>(null)

	const virtualizer = useVirtualizer<HTMLDivElement, HTMLTableRowElement>({
		count: rows.length,
		estimateSize: () => 54,
		getScrollElement: () => scrollRef.current,
		overscan: 5,
	})
	const virtualRows = virtualizer.getVirtualItems()

	const paddingTop = Math.max(0, virtualRows[0]?.start ?? 0 - virtualizer.options.scrollMargin)
	const paddingBottom = Math.max(0, virtualizer.getTotalSize() - (virtualRows[virtualRows.length - 1]?.end ?? 0))

	return (
		<div
			className={cn(
				"h-min max-h-[calc(100dvh-17rem)] w-full relative overflow-auto rounded-md border",
				(!rows.length || rows.length > 2) && "min-h-50"
			)}
			ref={scrollRef}
		>
			<div style={{ height: `${virtualizer.getTotalSize() + 48}px`, paddingTop, paddingBottom }}>
				<table className="w-full text-sm text-nowrap">
					<ProbesTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return <ProbesTableRow key={row.id} row={row} virtualRow={virtualRow} />
							})
						) : (
							<TableRow>
								<TableCell colSpan={colLength} className="h-37 text-center pointer-events-none">
									<Trans>No results.</Trans>
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</table>
			</div>
		</div>
	)
})

function ProbesTableHead({ table }: { table: ReturnType<typeof useReactTable<ProbeRow>> }) {
	return (
		<TableHeader className="sticky top-0 z-50 w-full border-b-2">
			{table.getHeaderGroups().map((headerGroup) => (
				<tr key={headerGroup.id}>
					{headerGroup.headers.map((header) => (
						<TableHead className="px-2" key={header.id}>
							{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
						</TableHead>
					))}
				</tr>
			))}
		</TableHeader>
	)
}

const ProbesTableRow = memo(function ProbesTableRow({
	row,
	virtualRow,
}: {
	row: Row<ProbeRow>
	virtualRow: VirtualItem
}) {
	return (
		<TableRow>
			{row.getVisibleCells().map((cell) => (
				<TableCell key={cell.id} className="py-0" style={{ height: virtualRow.size }}>
					{flexRender(cell.column.columnDef.cell, cell.getContext())}
				</TableCell>
			))}
		</TableRow>
	)
})
