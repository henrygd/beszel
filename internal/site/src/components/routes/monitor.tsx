import { useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { memo, useEffect, useState } from "react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import Spinner from "@/components/spinner"
import { $router, Link } from "@/components/router"
import { getPagePath } from "@nanostores/router"
import { pb } from "@/lib/api"
import type { MonitorCheckRecord, MonitorRecord, MonitorStatus } from "@/types"

const STATUS_STYLES: Record<MonitorStatus, string> = {
	up: "bg-green-500",
	down: "bg-red-500",
	warn: "bg-yellow-500",
	paused: "bg-primary/40",
	pending: "bg-yellow-500",
}

function formatTime(iso: string): string {
	const d = new Date(iso)
	return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

export default memo(({ id }: { id: string }) => {
	const { t } = useLingui()
	const [monitor, setMonitor] = useState<MonitorRecord | null>(null)
	const [checks, setChecks] = useState<MonitorCheckRecord[] | null>(null)
	const [error, setError] = useState("")
	const router = useStore($router)

	useEffect(() => {
		let cancelled = false
		pb.collection<MonitorRecord>("monitors")
			.getOne(id)
			.then((m) => {
				if (cancelled) {
					return
				}
				setMonitor(m)
				document.title = `${m.name} / Beszel`
			})
			.catch((e: unknown) => {
				if (!cancelled) {
					setError(e instanceof Error ? e.message : String(e))
				}
			})
		pb.send<MonitorCheckRecord[]>(`/api/beszel/monitors/${id}/checks?limit=200`, {})
			.then((rows) => {
				if (!cancelled) {
					setChecks(rows)
				}
			})
			.catch(() => {
				if (!cancelled) {
					setChecks([])
				}
			})
		return () => {
			cancelled = true
		}
	}, [id, router])

	if (error) {
		return (
			<Card>
				<CardContent className="py-10 text-center text-destructive">{error}</CardContent>
			</Card>
		)
	}
	if (!monitor) {
		return (
			<div className="relative h-40">
				<Spinner />
			</div>
		)
	}

	const latencies = (checks ?? [])
		.filter((c) => c.latency_ms > 0)
		.slice(-120)
		.reverse()
		.map((c) => ({ time: formatTime(c.created), ms: c.latency_ms }))

	return (
		<>
			<div className="flex flex-col gap-4">
				<ActiveAlerts />
				<div className="flex items-center gap-3">
					<Link href={getPagePath($router, "monitors")} className="text-sm text-muted-foreground hover:underline">
						{t`Monitors`}
					</Link>
					<span className="text-sm text-muted-foreground">/</span>
					<h1 className="text-xl font-semibold">{monitor.name}</h1>
					<Badge className={STATUS_STYLES[monitor.status]}>{monitor.status}</Badge>
				</div>
				<div className="grid gap-4 md:grid-cols-3">
					<Card>
						<CardHeader className="pb-2">
							<CardTitle className="text-sm font-medium">{t`Target`}</CardTitle>
						</CardHeader>
						<CardContent className="font-mono text-xs break-all">{monitor.target}</CardContent>
					</Card>
					<Card>
						<CardHeader className="pb-2">
							<CardTitle className="text-sm font-medium">{t`Uptime 24h`}</CardTitle>
						</CardHeader>
						<CardContent className="text-2xl font-semibold">
							{monitor.uptime_24h > 0 ? `${monitor.uptime_24h.toFixed(1)}%` : "—"}
						</CardContent>
					</Card>
					<Card>
						<CardHeader className="pb-2">
							<CardTitle className="text-sm font-medium">{t`Last latency`}</CardTitle>
						</CardHeader>
						<CardContent className="text-2xl font-semibold">
							{monitor.last_latency_ms > 0 ? `${monitor.last_latency_ms.toFixed(0)} ms` : "—"}
						</CardContent>
					</Card>
				</div>
				{(monitor.type === "tls" || monitor.target.startsWith("https")) && monitor.cert_days >= 0 && (
					<Card>
						<CardHeader className="pb-2">
							<CardTitle className="text-sm font-medium">{t`Certificate`}</CardTitle>
						</CardHeader>
						<CardContent className="text-sm text-muted-foreground">
							{t`Expires in ${monitor.cert_days.toFixed(0)} days`}
						</CardContent>
					</Card>
				)}
				{latencies.length > 1 && (
					<Card>
						<CardHeader className="pb-2">
							<CardTitle className="text-sm font-medium">{t`Latency (ms)`}</CardTitle>
						</CardHeader>
						<CardContent className="h-48">
							<ResponsiveContainer width="100%" height="100%">
								<AreaChart data={latencies}>
									<CartesianGrid strokeDasharray="3 3" />
									<XAxis dataKey="time" tick={false} />
									<YAxis width={40} />
									<Tooltip />
									<Area type="monotone" dataKey="ms" stroke="#22c55e" fill="#22c55e33" />
								</AreaChart>
							</ResponsiveContainer>
						</CardContent>
					</Card>
				)}
				<Card>
					<CardHeader className="pb-2">
						<CardTitle className="text-sm font-medium">{t`Recent checks`}</CardTitle>
					</CardHeader>
					<CardContent>
						{!checks ? (
							<div className="relative h-24">
								<Spinner />
							</div>
						) : checks.length === 0 ? (
							<p className="py-4 text-center text-sm text-muted-foreground">{t`No checks recorded yet.`}</p>
						) : (
							<div className="max-h-96 overflow-auto">
								<table className="w-full text-sm">
									<thead className="sticky top-0 bg-background text-left text-muted-foreground">
										<tr>
											<th className="py-1 pr-2 font-medium">{t`Time`}</th>
											<th className="py-1 pr-2 font-medium">{t`Status`}</th>
											<th className="py-1 pr-2 font-medium">{t`Latency`}</th>
											<th className="py-1 font-medium">{t`Message`}</th>
										</tr>
									</thead>
									<tbody>
										{checks.map((c) => (
											<tr key={c.id} className="border-t">
												<td className="py-1 pr-2 whitespace-nowrap text-xs">{formatTime(c.created)}</td>
												<td className="py-1 pr-2">
													<Badge className={STATUS_STYLES[c.status]}>{c.status}</Badge>
												</td>
												<td className="py-1 pr-2">{c.latency_ms > 0 ? `${c.latency_ms.toFixed(0)} ms` : "—"}</td>
												<td className="py-1 text-xs text-muted-foreground">{c.message}</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>
						)}
					</CardContent>
				</Card>
			</div>
			<FooterRepoLink />
		</>
	)
})
