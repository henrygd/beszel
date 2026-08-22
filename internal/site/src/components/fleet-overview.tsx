import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import {
	ActivityIcon,
	AlertTriangleIcon,
	Clock3Icon,
	PauseIcon,
	PlusIcon,
	RadioTowerIcon,
	RefreshCwIcon,
	ServerIcon,
} from "lucide-react"
import { useMemo, useState, type ElementType, type ReactNode } from "react"
import { isReadOnlyUser } from "@/lib/api"
import { alertManager } from "@/lib/alerts"
import { $alerts, $downSystems, $pausedSystems, $systems, $upSystems } from "@/lib/stores"
import * as systemsManager from "@/lib/systemsManager"
import { AddSystemDialog } from "./add-system"
import { Button } from "./ui/button"
import { Card, CardContent } from "./ui/card"

interface PulseSegment {
	label: string
	count: number
	className: string
}

export function FleetOverview() {
	const { t } = useLingui()
	const systems = useStore($systems)
	const upSystems = useStore($upSystems)
	const downSystems = useStore($downSystems)
	const pausedSystems = useStore($pausedSystems)
	const alerts = useStore($alerts)
	const [addSystemOpen, setAddSystemOpen] = useState(false)
	const [isRefreshing, setIsRefreshing] = useState(false)

	const activeAlertCount = useMemo(
		() =>
			Object.values(alerts).reduce(
				(total, systemAlerts) => total + [...systemAlerts.values()].filter((alert) => alert.triggered).length,
				0
			),
		[alerts]
	)

	const statusCounts = useMemo(() => {
		const up = Object.keys(upSystems).length
		const down = Object.keys(downSystems).length
		const paused = Object.keys(pausedSystems).length
		return { up, down, paused, pending: Math.max(0, systems.length - up - down - paused) }
	}, [systems, upSystems, downSystems, pausedSystems])

	async function refreshFleet() {
		if (isRefreshing) return
		setIsRefreshing(true)
		try {
			await systemsManager.refresh()
			await alertManager.refresh()
		} finally {
			setIsRefreshing(false)
		}
	}

	if (systems.length === 0) {
		return (
			<>
				<Card className="relative overflow-hidden border-primary/30">
					<div className="signal-scan absolute inset-0 opacity-35" aria-hidden="true" />
					<CardContent className="relative grid min-h-64 place-items-center p-6 text-center sm:p-10">
						<div className="max-w-xl">
							<div className="signal-glow mx-auto mb-5 grid size-14 place-items-center rounded-2xl border border-primary/30 bg-primary/10 text-primary">
								<RadioTowerIcon className="size-7" aria-hidden="true" />
							</div>
							<p className="mb-2 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.22em] text-primary">
								<Trans>Fleet awaiting signal</Trans>
							</p>
							<h2 className="text-balance text-2xl font-semibold tracking-tight sm:text-3xl">
								<Trans>Connect your first system</Trans>
							</h2>
							<p className="mx-auto mt-3 max-w-lg text-pretty text-sm leading-6 text-muted-foreground sm:text-base">
								<Trans>
									Install a lightweight Beszel agent to start monitoring health, resources, containers, and alerts.
								</Trans>
							</p>
							{!isReadOnlyUser() && (
								<Button className="mt-6" onClick={() => setAddSystemOpen(true)}>
									<PlusIcon className="size-4" />
									<Trans>Add first system</Trans>
								</Button>
							)}
						</div>
					</CardContent>
				</Card>
				<AddSystemDialog open={addSystemOpen} setOpen={setAddSystemOpen} />
			</>
		)
	}

	const segments: PulseSegment[] = [
		{ label: t`Online`, count: statusCounts.up, className: "bg-primary" },
		{ label: t`Pending`, count: statusCounts.pending, className: "bg-amber-400" },
		{ label: t`Down`, count: statusCounts.down, className: "bg-destructive" },
		{ label: t`Paused`, count: statusCounts.paused, className: "bg-muted-foreground/45" },
	]

	return (
		<Card className="overflow-hidden">
			<CardContent className="p-4 sm:p-6">
				<div className="mb-5 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
					<div>
						<p className="font-mono text-[0.68rem] font-semibold uppercase tracking-[0.22em] text-primary">
							<Trans>Live fleet pulse</Trans>
						</p>
						<h2 className="mt-1 text-xl font-semibold tracking-tight">
							<Plural value={systems.length} one="# monitored system" other="# monitored systems" />
						</h2>
					</div>
					<div className="flex items-center gap-2">
						<p className="font-mono text-xs text-muted-foreground">
							<Trans>Current connection state</Trans>
						</p>
						<Button
							variant="ghost"
							size="sm"
							className="h-8 gap-1.5 px-2 text-muted-foreground"
							onClick={refreshFleet}
							disabled={isRefreshing}
							aria-label={t`Refresh fleet`}
						>
							<RefreshCwIcon className={isRefreshing ? "size-3.5 animate-spin" : "size-3.5"} />
							<span className="hidden sm:inline">
								<Trans>Refresh</Trans>
							</span>
						</Button>
					</div>
				</div>

				<div
					className="fleet-pulse-track flex h-2.5 w-full overflow-hidden rounded-full bg-muted"
					role="img"
					aria-label={`${t`Fleet status`}: ${statusCounts.up} ${t`online`}, ${statusCounts.pending} ${t`pending`}, ${statusCounts.down} ${t`down`}, ${statusCounts.paused} ${t`paused`}`}
				>
					{segments.map(
						(segment) =>
							segment.count > 0 && (
								<div
									key={segment.label}
									className={segment.className}
									style={{ width: `${(segment.count / systems.length) * 100}%` }}
									title={`${segment.label}: ${segment.count}`}
								/>
							)
					)}
				</div>

				<div className="mt-5 grid grid-cols-2 gap-px overflow-hidden rounded-xl border bg-border sm:grid-cols-5">
					<PulseStat icon={ActivityIcon} label={<Trans>Online</Trans>} value={statusCounts.up} accent="text-primary" />
					<PulseStat
						icon={Clock3Icon}
						label={<Trans>Pending</Trans>}
						value={statusCounts.pending}
						accent="text-amber-500"
					/>
					<PulseStat
						icon={AlertTriangleIcon}
						label={<Trans>Down</Trans>}
						value={statusCounts.down}
						accent="text-destructive"
					/>
					<PulseStat
						icon={PauseIcon}
						label={<Trans>Paused</Trans>}
						value={statusCounts.paused}
						accent="text-muted-foreground"
					/>
					<PulseStat
						icon={ServerIcon}
						label={<Trans>Active alerts</Trans>}
						value={activeAlertCount}
						accent={activeAlertCount ? "text-amber-500" : "text-primary"}
					/>
				</div>
			</CardContent>
		</Card>
	)
}

function PulseStat({
	icon: Icon,
	label,
	value,
	accent,
}: {
	icon: ElementType
	label: ReactNode
	value: number
	accent: string
}) {
	return (
		<div className="flex min-h-20 items-center gap-3 bg-card px-4 py-3">
			<Icon className={`size-4 shrink-0 ${accent}`} aria-hidden="true" />
			<div>
				<p className="font-mono text-2xl font-semibold tabular-nums leading-none">{value}</p>
				<p className="mt-1 text-xs text-muted-foreground">{label}</p>
			</div>
		</div>
	)
}
