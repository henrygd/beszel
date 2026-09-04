import { useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { memo, useEffect, useState } from "react"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import Spinner from "@/components/spinner"
import { toast } from "@/components/ui/use-toast"
import { cn } from "@/lib/utils"
import { Trans } from "@lingui/react/macro"
import { BellIcon, BellOffIcon, PauseIcon, PencilIcon, PlayIcon, PlusIcon, RotateCwIcon, Trash2Icon } from "lucide-react"
import { getPagePath } from "@nanostores/router"
import { $router, Link } from "@/components/router"
import { isReadOnlyUser, pb } from "@/lib/api"
import { $monitors, cleanup, init } from "@/lib/monitors"
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
	const { t } = useLingui()
	const readonly = isReadOnlyUser()
	const [testing, setTesting] = useState(false)
	const [deleteOpen, setDeleteOpen] = useState(false)

	const update = async (body: object, label: string) => {
		try {
			await pb.collection("monitors").update(monitor.id, body)
		} catch (e) {
			toast({ title: label, description: e instanceof Error ? e.message : String(e), variant: "destructive" })
		}
	}

	const runTest = async () => {
		setTesting(true)
		try {
			await pb.send(`/api/beszel/monitors/${monitor.id}/test`, { method: "POST" })
			toast({ title: t`Check completed` })
		} catch (e) {
			toast({ title: t`Check failed`, description: e instanceof Error ? e.message : String(e), variant: "destructive" })
		} finally {
			setTesting(false)
		}
	}

	const title = readonly ? (
		<span>{monitor.name}</span>
	) : (
		<Link href={getPagePath($router, "monitor", { id: monitor.id })} className="hover:underline">
			{monitor.name}
		</Link>
	)

	return (
		<Card>
			<CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle className="text-base font-medium">{title}</CardTitle>
				<div className="flex items-center gap-2">
					<Badge className={STATUS_STYLES[monitor.status]}>{monitor.status}</Badge>
					<Badge variant="outline">{TYPE_LABELS[monitor.type] ?? monitor.type}</Badge>
				</div>
			</CardHeader>
			<CardContent className="text-sm text-muted-foreground">
				<div className="truncate font-mono text-xs">{monitor.target}</div>
				<div className="mt-2 flex flex-wrap gap-x-4 gap-y-1">
					<span>{t`Uptime 24h: ${monitor.uptime_24h > 0 ? monitor.uptime_24h.toFixed(1) : "—"}%`}</span>
					{monitor.last_latency_ms > 0 && <span>{t`${monitor.last_latency_ms.toFixed(0)} ms`}</span>}
					{monitor.cert_days >= 0 && monitor.cert_days < 60 && (
						<span>{t`Cert expires in ${monitor.cert_days.toFixed(0)} days`}</span>
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
							onClick={() => update({ paused: !monitor.paused }, t`Pause toggled`)}
							title={monitor.paused ? t`Resume` : t`Pause`}
						>
							{monitor.paused ? <PlayIcon /> : <PauseIcon />}
						</Button>
						<Button variant="outline" size="sm" onClick={() => onEdit(monitor)} title={t`Edit`}>
							<PencilIcon />
						</Button>
						<Button variant="outline" size="sm" onClick={() => setDeleteOpen(true)} title={t`Delete`}>
							<Trash2Icon />
						</Button>
						<Button
							variant="outline"
							size="sm"
							onClick={() => update({ notify: !monitor.notify }, t`Notifications toggled`)}
							title={monitor.notify ? t`Mute notifications` : t`Unmute notifications`}
						>
							{monitor.notify ? <BellIcon /> : <BellOffIcon />}
						</Button>
					</div>
				)}
			</CardContent>
			<AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Are you sure you want to delete {monitor.name}?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>This action cannot be undone. This will permanently delete the monitor and its check history.</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn(buttonVariants({ variant: "destructive" }))}
							onClick={() =>
								pb
									.collection("monitors")
									.delete(monitor.id)
									.catch((e: unknown) =>
										toast({
											title: t`Delete failed`,
											description: e instanceof Error ? e.message : String(e),
											variant: "destructive",
										})
									)
							}
						>
							<Trans>Continue</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</Card>
	)
}

export default memo(() => {
	const { t } = useLingui()
	const monitors = useStore($monitors)
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editing, setEditing] = useState<MonitorRecord | undefined>(undefined)
	const [loaded, setLoaded] = useState(false)
	const [loadError, setLoadError] = useState("")
	const readonly = isReadOnlyUser()

	useEffect(() => {
		document.title = `${t`Monitors`} / Beszel`
	}, [])

	useEffect(() => {
		init()
		let cancelled = false
		pb.collection<MonitorRecord>("monitors")
			.getFullList()
			.then(() => {
				if (!cancelled) {
					setLoaded(true)
				}
			})
			.catch((e: unknown) => {
				if (!cancelled) {
					setLoadError(e instanceof Error ? e.message : String(e))
					setLoaded(true)
				}
			})
		return () => {
			cancelled = true
			cleanup()
		}
	}, [])

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
				) : loadError ? (
					<Card>
						<CardContent className="py-10 text-center text-destructive">{loadError}</CardContent>
					</Card>
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
			{!readonly && (
				<MonitorDialog
					open={dialogOpen}
					setOpen={setDialogOpen}
					monitor={editing}
					onSaved={() => setDialogOpen(false)}
				/>
			)}
		</>
	)
})
