import { t } from "@lingui/core/macro"
import { useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import {
	BellIcon,
	BellOffIcon,
	PauseIcon,
	PlayIcon,
	PlusIcon,
	RotateCwIcon,
	Trash2Icon,
} from "lucide-react"
import { memo, useEffect, useState } from "react"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import Spinner from "@/components/spinner"
import { isReadOnlyUser, pb } from "@/lib/api"
import { $monitors } from "@/lib/monitors"
import type { MonitorRecord, MonitorStatus } from "@/types"
import { MonitorDialog } from "./monitor-dialog"

const STATUS_STYLES: Record<MonitorStatus, string> = {
	up: "bg-green-500",
	down: "bg-red-500",
	warn: "bg-yellow-500",
	paused: "bg-primary/40",
	pending: "bg-yellow-500",
}

const TYPE_LABELS: Record<string, string> = {
	http: "HTTP",
	keyword: "Keyword",
	ping: "Ping",
	dns: "DNS",
	tls: "TLS",
}

function MonitorCard({ monitor, onEdit }: { monitor: MonitorRecord; onEdit: (m: MonitorRecord) => void }) {
	const readonly = isReadOnlyUser()
	const [testing, setTesting] = useState(false)

	const update = async (body: object) => {
		await pb.collection("monitors").update(monitor.id, body)
	}

	const runTest = async () => {
		setTesting(true)
		try {
			await pb.send(`/api/beszel/monitors/${monitor.id}/test`, { method: "POST" })
		} finally {
			setTesting(false)
		}
	}

	return (
		<Card>
			<CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle className="text-base font-medium">
					<button type="button" className="hover:underline" onClick={() => onEdit(monitor)}>
						{monitor.name}
					</button>
				</CardTitle>
				<div className="flex items-center gap-2">
					<Badge className={STATUS_STYLES[monitor.status]}>{monitor.status}</Badge>
					<Badge variant="outline">{TYPE_LABELS[monitor.type] ?? monitor.type}</Badge>
				</div>
			</CardHeader>
			<CardContent className="text-sm text-muted-foreground">
				<div className="truncate font-mono text-xs">{monitor.target}</div>
				<div className="mt-2 flex flex-wrap gap-x-4 gap-y-1">
					{monitor.uptime_24h > 0 && <span>{t`Uptime 24h`} {monitor.uptime_24h.toFixed(1)}%</span>}
					{monitor.last_latency_ms > 0 && <span>{monitor.last_latency_ms.toFixed(0)} ms</span>}
					{monitor.cert_days > 0 && (
						<span>
							{t`Cert`} {monitor.cert_days.toFixed(0)} {t`days`}
						</span>
					)}
				</div>
				{!readonly && (
					<div className="mt-3 flex flex-wrap gap-2">
						<Button variant="outline" size="sm" disabled={testing} onClick={runTest} title={t`Run check now`}>
							<RotateCwIcon className={testing ? "animate-spin" : ""} />
						</Button>
						<Button
							variant="outline"
							size="sm"
							onClick={() => update({ paused: !monitor.paused })}
							title={monitor.paused ? t`Resume` : t`Pause`}
						>
							{monitor.paused ? <PlayIcon /> : <PauseIcon />}
						</Button>
						<Button
							variant="outline"
							size="sm"
							onClick={() => pb.collection("monitors").delete(monitor.id)}
							title={t`Delete`}
						>
							<Trash2Icon />
						</Button>
						<Button
							variant="outline"
							size="sm"
							onClick={() => update({ notify: !monitor.notify })}
							title={monitor.notify ? t`Mute notifications` : t`Unmute notifications`}
						>
							{monitor.notify ? <BellIcon /> : <BellOffIcon />}
						</Button>
					</div>
				)}
			</CardContent>
		</Card>
	)
}

export default memo(() => {
	const { t } = useLingui()
	const monitors = useStore($monitors)
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editing, setEditing] = useState<MonitorRecord | undefined>(undefined)
	const [loaded, setLoaded] = useState(false)
	const readonly = isReadOnlyUser()

	useEffect(() => {
		document.title = `${t`Monitors`} / Beszel`
		pb.collection<MonitorRecord>("monitors")
			.getFullList()
			.then((records) => {
				$monitors.set(Object.fromEntries(records.map((m) => [m.id, m])))
				setLoaded(true)
			})
			.catch(() => setLoaded(true))
		const unsub = pb.collection("monitors").subscribe("*", (e) => {
			const record = e.record as unknown as MonitorRecord
			const current = $monitors.get()
			if (e.action === "delete") {
				const next = { ...current }
				delete next[record.id]
				$monitors.set(next)
			} else {
				$monitors.set({ ...current, [record.id]: record })
			}
		})
		return () => {
			unsub.then((fn) => (typeof fn === "function" ? fn() : undefined))
		}
	}, [t])

	const list = Object.values(monitors).sort((a, b) => a.name.localeCompare(b.name))

	return (
		<>
			<div className="flex flex-col gap-4">
				<ActiveAlerts />
				<div className="flex items-center justify-between">
					<h1 className="text-xl font-semibold">{t`Monitors`}</h1>
					{!readonly && (
						<Button
							size="sm"
							onClick={() => {
								setEditing(undefined)
								setDialogOpen(true)
							}}
						>
							<PlusIcon /> {t`Add monitor`}
						</Button>
					)}
				</div>
				{!loaded ? (
					<div className="relative h-40">
						<Spinner />
					</div>
				) : list.length === 0 ? (
					<Card>
						<CardContent className="py-10 text-center text-muted-foreground">
							{t`No monitors yet. Add your first HTTP, DNS, ping or TLS check.`}
						</CardContent>
					</Card>
				) : (
					<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
						{list.map((m) => (
							<MonitorCard
								key={m.id}
								monitor={m}
								onEdit={(mon) => {
									setEditing(mon)
									setDialogOpen(true)
								}}
							/>
						))}
					</div>
				)}
			</div>
			<FooterRepoLink />
			<MonitorDialog
				open={dialogOpen}
				setOpen={setDialogOpen}
				monitor={editing}
				onSaved={() => setDialogOpen(false)}
			/>
		</>
	)
})
