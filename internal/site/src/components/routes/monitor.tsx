import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { memo, useCallback, useEffect, useMemo, useState } from "react"
import { Link } from "../router"
import { AlertCircleIcon, ArrowLeftIcon, CheckIcon, GlobeIcon, NetworkIcon, PauseIcon, XIcon, ZapIcon } from "lucide-react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { pb } from "@/lib/api"
import { isReadOnlyUser } from "@/lib/api"
import { $allMonitorsById } from "@/lib/stores"
import { SystemStatus } from "@/lib/enums"
import { cn, decimalString, formatShortDate, toFixedFloat } from "@/lib/utils"
import type { MonitorCheckRecord, MonitorRecord } from "@/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { MonitorDialog } from "@/components/add-monitor"

const MONITOR_TYPE_ICONS: Record<string, typeof GlobeIcon> = {
	http: GlobeIcon,
	tcp: NetworkIcon,
	ping: ZapIcon,
}

function formatMs(ms: number) {
	if (ms >= 1000) {
		return `${decimalString(ms / 1000)}s`
	}
	return `${decimalString(ms)}ms`
}

function StatusBadge({ status }: { status: string }) {
	const config = {
		[`${SystemStatus.Up}`]: { label: t`Up`, icon: CheckIcon, className: "bg-success text-white border-transparent" },
		[`${SystemStatus.Down}`]: { label: t`Down`, icon: XIcon, className: "bg-destructive text-white border-transparent" },
		[`${SystemStatus.Paused}`]: { label: t`Paused`, icon: PauseIcon, className: "bg-secondary text-secondary-foreground border-transparent" },
		[`${SystemStatus.Pending}`]: { label: t`Pending`, icon: AlertCircleIcon, className: "bg-secondary text-secondary-foreground border-transparent" },
	}[status] ?? { label: status, icon: AlertCircleIcon, className: "bg-secondary text-secondary-foreground border-transparent" }

	const Icon = config.icon
	return (
		<Badge variant="default" className={cn("flex gap-1 items-center", config.className)}>
			<Icon className="h-3 w-3" />
			{config.label}
		</Badge>
	)
}

function UptimeChart({ checks }: { checks: MonitorCheckRecord[] }) {
	const data = useMemo(() => {
		return checks
			.slice()
			.reverse()
			.map((c) => ({
				time: new Date(c.created).getTime(),
				ms: c.ms ?? null,
				up: c.up ? 1 : 0,
			}))
	}, [checks])

	if (!data.length) {
		return (
			<div className="h-64 flex items-center justify-center text-muted-foreground">
				<Trans>No check data available yet</Trans>
			</div>
		)
	}

	return (
		<ResponsiveContainer width="100%" height={260}>
			<AreaChart data={data} margin={{ top: 10, right: 10, bottom: 0, left: 0 }}>
				<defs>
					<linearGradient id="msGradient" x1="0" y1="0" x2="0" y2="1">
						<stop offset="5%" stopColor="var(--chart-1)" stopOpacity={0.4} />
						<stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0} />
					</linearGradient>
				</defs>
				<CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--border)" />
				<XAxis
					dataKey="time"
					type="number"
					scale="time"
					domain={["dataMin", "dataMax"]}
					tickFormatter={(val) => formatShortDate(String(val))}
					tickLine={false}
					axisLine={false}
				/>
				<YAxis
					tickFormatter={(val) => formatMs(val)}
					tickLine={false}
					axisLine={false}
					width={60}
				/>
				<Tooltip
					labelFormatter={(val) => new Date(val as number).toLocaleString()}
					formatter={(value) => [formatMs(value as number), "Response time"]}
				/>
				<Area
					type="stepAfter"
					dataKey="ms"
					stroke="var(--chart-1)"
					fill="url(#msGradient)"
					name="Response time"
				/>
			</AreaChart>
		</ResponsiveContainer>
	)
}

function ChecksTable({ checks }: { checks: MonitorCheckRecord[] }) {
	if (!checks.length) {
		return (
			<div className="py-8 text-center text-muted-foreground">
				<Trans>No checks yet</Trans>
			</div>
		)
	}

	return (
		<div className="divide-y">
			{checks.map((check) => (
				<div key={check.id} className="py-3 flex items-start justify-between gap-4">
					<div className="flex items-start gap-3 min-w-0">
						<div
							className={cn(
								"mt-1 h-2 w-2 rounded-full shrink-0",
								check.up ? "bg-success" : "bg-destructive"
							)}
						/>
						<div className="min-w-0">
							<div className="text-sm">
								{check.up ? (
									<Trans>Up</Trans>
								) : (
									<Trans>Down</Trans>
								)}
								{check.msg ? <span className="text-muted-foreground"> — {check.msg}</span> : null}
							</div>
							<div className="text-xs text-muted-foreground">{new Date(check.created).toLocaleString()}</div>
						</div>
					</div>
					<div className="text-sm tabular-nums shrink-0">{check.ms != null ? formatMs(check.ms) : "—"}</div>
				</div>
			))}
		</div>
	)
}

export default memo(function MonitorDetail({ id }: { id: string }) {
	const [monitor, setMonitor] = useState<MonitorRecord | null>($allMonitorsById.get()[id] ?? null)
	const [checks, setChecks] = useState<MonitorCheckRecord[]>([])
	const [editOpen, setEditOpen] = useState(false)
	const [checking, setChecking] = useState(false)

	useEffect(() => {
		return () => {
			document.title = "Beszel"
		}
	}, [])

	// keep monitor in sync with store
	useEffect(() => {
		const unsub = $allMonitorsById.listen((m) => {
			const found = m[id]
			if (found) {
				setMonitor(found)
				document.title = `${found.name} / Beszel`
			}
		})
		return unsub
	}, [id])

	// fetch initial monitor if not in store (e.g. navigated directly)
	useEffect(() => {
		if (monitor) {
			return
		}
		pb.collection<MonitorRecord>("monitors")
			.getOne(id)
			.then((m) => {
				setMonitor(m)
				$allMonitorsById.setKey(id, m)
			})
			.catch(() => {
				/* not found — leave null */
			})
	}, [id, monitor])

	// fetch checks (most recent first) + subscribe to realtime
	useEffect(() => {
		let cancelled = false

		pb
			.collection<MonitorCheckRecord>("monitor_checks")
			.getFullList({
				filter: pb.filter("monitor = {:id}", { id }),
				sort: "-created",
				totalItems: 0,
				batch: 1000,
			})
			.then((recs) => {
				if (cancelled) return
				setChecks(recs.slice(0, 500))
			})
			.catch(() => {})

		let unsubscribe: (() => void) | undefined
		try {
			pb
				.collection("monitor_checks")
				.subscribe(
					"*",
					(e) => {
						const rec = e.record as unknown as MonitorCheckRecord
						if (!rec || rec.monitor !== id) {
							return
						}
						setChecks((prev) => {
							const idx = prev.findIndex((c) => c.id === rec.id)
							if (idx >= 0) {
								const next = [...prev]
								next[idx] = rec
								return next
							}
							return [rec, ...prev].slice(0, 500)
						})
					},
					{ fields: "id,monitor,up,ms,msg,created" }
				)
				.then((unsub) => {
					if (cancelled) {
						unsub()
						return
					}
					unsubscribe = unsub
				})
		} catch (e) {
			/* realtime unavailable */
		}

		return () => {
			cancelled = true
			unsubscribe?.()
		}
	}, [id])

	const handleCheckNow = useCallback(async () => {
		if (!monitor) {
			return
		}
		setChecking(true)
		try {
			await pb.send(`/api/beszel/uptime/check-now?monitor=${monitor.id}`, {})
		} catch {
			/* ignore */
		} finally {
			setChecking(false)
		}
	}, [monitor])

	if (!monitor) {
		return null
	}

	const TypeIcon = MONITOR_TYPE_ICONS[monitor.type] ?? GlobeIcon
	const targetLabel =
		monitor.type === "http"
			? monitor.url
			: monitor.type === "tcp"
				? `${monitor.host}:${monitor.port ?? ""}`
				: monitor.host

	const upChecks = checks.filter((c) => c.up).length
	const totalChecks = checks.length
	const uptimePct = totalChecks ? (upChecks / totalChecks) * 100 : null

	return (
		<>
			{editOpen && <MonitorDialog monitor={monitor} setOpen={setEditOpen} />}
			<div className="flex flex-col gap-4">
				<div className="flex items-start justify-between gap-4">
					<div className="flex items-center gap-3 min-w-0">
						<Button
							variant="ghost"
							size="icon"
							aria-label="Back"
							className="shrink-0"
							asChild
						>
							<Link href="/monitors">
								<ArrowLeftIcon className="h-4 w-4" />
							</Link>
						</Button>
						<div className="h-10 w-10 rounded-lg bg-muted flex items-center justify-center shrink-0">
							<TypeIcon className="h-5 w-5" />
						</div>
						<div className="min-w-0">
							<div className="flex items-center gap-2 flex-wrap">
								<h1 className="text-lg font-semibold truncate">{monitor.name}</h1>
								<StatusBadge status={monitor.status} />
							</div>
							<div className="text-sm text-muted-foreground truncate">{targetLabel}</div>
						</div>
					</div>

					<div className="flex gap-2 shrink-0">
						{!isReadOnlyUser() && (
							<>
								<Button
									variant="outline"
									onClick={handleCheckNow}
									disabled={checking}
									className="gap-1"
								>
									<ZapIcon className="h-4 w-4" />
									<Trans>Check now</Trans>
								</Button>
								<Button variant="outline" onClick={() => setEditOpen(true)} className="gap-1">
									<Trans>Edit</Trans>
								</Button>
							</>
						)}
					</div>
				</div>

				<div className="grid gap-4 sm:grid-cols-3">
					<Card>
						<CardHeader>
							<CardTitle>
								<Trans>Uptime</Trans>
							</CardTitle>
							<CardDescription>
								<Trans>Across the {totalChecks} most recent checks</Trans>
							</CardDescription>
						</CardHeader>
						<CardContent>
							<div className="text-2xl font-semibold tabular-nums">
								{uptimePct == null ? "—" : `${toFixedFloat(uptimePct, 2)}%`}
							</div>
						</CardContent>
					</Card>
					<Card>
						<CardHeader>
							<CardTitle>
								<Trans>Type</Trans>
							</CardTitle>
							<CardDescription>
								<Trans>Monitor type</Trans>
							</CardDescription>
						</CardHeader>
						<CardContent>
							<div className="text-2xl font-semibold uppercase">{monitor.type}</div>
						</CardContent>
					</Card>
					<Card>
						<CardHeader>
							<CardTitle>
								<Trans>Interval</Trans>
							</CardTitle>
							<CardDescription>
								<Trans>Check frequency</Trans>
							</CardDescription>
						</CardHeader>
						<CardContent>
							<div className="text-2xl font-semibold tabular-nums">
								{monitor.interval ? `${monitor.interval}s` : "—"}
							</div>
						</CardContent>
					</Card>
				</div>

				<Card>
					<CardHeader>
						<CardTitle>
							<Trans>Response time</Trans>
						</CardTitle>
						<CardDescription>
							<Trans>Latency of each check over time</Trans>
						</CardDescription>
					</CardHeader>
					<CardContent>
						<UptimeChart checks={checks} />
					</CardContent>
				</Card>

				<Card>
					<CardHeader>
						<CardTitle>
							<Trans>Recent checks</Trans>
						</CardTitle>
						<CardDescription>
							<Trans>Most recent first</Trans>
						</CardDescription>
					</CardHeader>
					<CardContent>
						<ChecksTable checks={checks} />
					</CardContent>
				</Card>
			</div>
		</>
	)
})
