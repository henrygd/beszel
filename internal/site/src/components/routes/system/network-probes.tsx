import { useCallback, useEffect, useMemo, useState } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
import { pb } from "@/lib/api"
import { useStore } from "@nanostores/react"
import { $chartTime } from "@/lib/stores"
import { chartTimeData, cn, toFixedFloat, decimalString } from "@/lib/utils"
import { Card, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Trash2Icon } from "lucide-react"
import { useToast } from "@/components/ui/use-toast"
import { AddProbeDialog } from "./probe-dialog"
import { ChartCard } from "./chart-card"
import LineChartDefault, { type DataPoint } from "@/components/charts/line-chart"
import { pinnedAxisDomain } from "@/components/ui/chart"
import type { ChartData, NetworkProbeRecord, NetworkProbeStatsRecord } from "@/types"

function probeKey(p: NetworkProbeRecord) {
	if (p.protocol === "tcp") return `${p.protocol}:${p.target}:${p.port}`
	return `${p.protocol}:${p.target}`
}

export default function NetworkProbes({
	systemId,
	chartData,
	grid,
}: {
	systemId: string
	chartData: ChartData
	grid: boolean
}) {
	const [probes, setProbes] = useState<NetworkProbeRecord[]>([])
	const [stats, setStats] = useState<NetworkProbeStatsRecord[]>([])
	const [latestResults, setLatestResults] = useState<Record<string, { avg: number; loss: number }>>({})
	const chartTime = useStore($chartTime)
	const { toast } = useToast()
	const { t } = useLingui()

	const fetchProbes = useCallback(() => {
		pb.send<NetworkProbeRecord[]>("/api/beszel/network-probes", {
			query: { system: systemId },
		})
			.then(setProbes)
			.catch(() => setProbes([]))
	}, [systemId])

	useEffect(() => {
		fetchProbes()
	}, [fetchProbes])

	// Fetch probe stats based on chart time
	useEffect(() => {
		if (probes.length === 0) return
		const controller = new AbortController()
		const statsType = chartTimeData[chartTime]?.type ?? "1m"

		pb.send<{ stats: NetworkProbeStatsRecord["stats"]; created: string }[]>("/api/beszel/network-probe-stats", {
			query: { system: systemId, type: statsType },
			signal: controller.signal,
		})
			.then((raw) => {
				const data: NetworkProbeStatsRecord[] = raw.map((r) => ({
					stats: r.stats,
					created: new Date(r.created).getTime(),
				}))
				setStats(data)
				if (data.length > 0) {
					const last = data[data.length - 1].stats
					const latest: Record<string, { avg: number; loss: number }> = {}
					for (const [key, val] of Object.entries(last)) {
						latest[key] = { avg: val.avg, loss: val.loss }
					}
					setLatestResults(latest)
				}
			})
			.catch(() => setStats([]))

		return () => controller.abort()
	}, [systemId, chartTime, probes])

	const deleteProbe = async (id: string) => {
		try {
			await pb.send("/api/beszel/network-probes", {
				method: "DELETE",
				query: { id },
			})
			fetchProbes()
		} catch (err: any) {
			toast({ variant: "destructive", title: t`Error`, description: err?.message })
		}
	}

	const dataPoints: DataPoint<NetworkProbeStatsRecord>[] = useMemo(() => {
		return probes.map((p, i) => {
			const key = probeKey(p)
			return {
				label: p.name || p.target,
				dataKey: (record: NetworkProbeStatsRecord) => record.stats[key]?.avg ?? null,
				color: (i % 10) + 1,
			}
		})
	}, [probes])

	if (probes.length === 0 && stats.length === 0) {
		return (
			<Card className="px-3 py-5 sm:py-6 sm:px-6">
				<CardHeader className="p-0">
					<div className="flex items-center justify-between">
						<div>
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
			</Card>
		)
	}

	const protocolBadge = (protocol: string) => {
		const colors: Record<string, string> = {
			icmp: "bg-blue-500/15 text-blue-400",
			tcp: "bg-purple-500/15 text-purple-400",
			http: "bg-green-500/15 text-green-400",
		}
		return (
			<span className={cn("px-2 py-0.5 rounded text-xs font-medium uppercase", colors[protocol] ?? "")}>
				{protocol}
			</span>
		)
	}

	return (
		<div className="grid gap-4">
			<Card className="px-3 py-5 sm:py-6 sm:px-6">
				<CardHeader className="p-0 mb-4">
					<div className="flex items-center justify-between">
						<div>
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

				<div className="overflow-x-auto -mx-3 sm:-mx-6">
					<table className="w-full text-sm">
						<thead>
							<tr className="border-b text-muted-foreground">
								<th className="text-left font-medium px-3 sm:px-6 py-2">
									<Trans>Name</Trans>
								</th>
								<th className="text-left font-medium px-3 py-2">
									<Trans>Target</Trans>
								</th>
								<th className="text-left font-medium px-3 py-2">
									<Trans>Protocol</Trans>
								</th>
								<th className="text-left font-medium px-3 py-2">
									<Trans>Interval</Trans>
								</th>
								<th className="text-left font-medium px-3 py-2">
									<Trans>Latency</Trans>
								</th>
								<th className="text-left font-medium px-3 py-2">
									<Trans>Loss</Trans>
								</th>
								<th className="text-right font-medium px-3 sm:px-6 py-2"></th>
							</tr>
						</thead>
						<tbody>
							{probes.map((p) => {
								const key = probeKey(p)
								const result = latestResults[key]
								return (
									<tr key={p.id} className="border-b last:border-0">
										<td className="px-3 sm:px-6 py-2.5 text-muted-foreground">{p.name || p.target}</td>
										<td className="px-3 py-2.5 font-mono text-xs">{p.target}</td>
										<td className="px-3 py-2.5">{protocolBadge(p.protocol)}</td>
										<td className="px-3 py-2.5">{p.interval}s</td>
										<td className="px-3 py-2.5">
											{result ? (
												<span className={result.avg > 100 ? "text-yellow-400" : "text-green-400"}>
													{toFixedFloat(result.avg, 1)} ms
												</span>
											) : (
												<span className="text-muted-foreground">-</span>
											)}
										</td>
										<td className="px-3 py-2.5">
											{result ? (
												<span className={result.loss > 0 ? "text-red-400" : "text-green-400"}>
													{toFixedFloat(result.loss, 1)}%
												</span>
											) : (
												<span className="text-muted-foreground">-</span>
											)}
										</td>
										<td className="px-3 sm:px-6 py-2.5 text-right">
											<Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => deleteProbe(p.id)}>
												<Trash2Icon className="h-3.5 w-3.5 text-destructive" />
											</Button>
										</td>
									</tr>
								)
							})}
						</tbody>
					</table>
				</div>
			</Card>

			{stats.length > 0 && (
				<ChartCard
					title={t`Latency`}
					description={t`Average round-trip time (ms)`}
					grid={grid}
				>
					<LineChartDefault
						chartData={chartData}
						customData={stats}
						dataPoints={dataPoints}
						domain={pinnedAxisDomain()}
						tickFormatter={(value) => `${toFixedFloat(value, value >= 10 ? 0 : 1)} ms`}
						contentFormatter={({ value }) => `${decimalString(value, 2)} ms`}
						legend
					/>
				</ChartCard>
			)}
		</div>
	)
}
