import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { PlusIcon, RadioTowerIcon, RefreshCwIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { $router, Link } from "@/components/router"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { isReadOnlyUser } from "@/lib/api"
import { alertManager } from "@/lib/alerts"
import { $alerts, $downSystems, $pausedSystems, $systems, $upSystems } from "@/lib/stores"
import * as systemsManager from "@/lib/systemsManager"
import { AddSystemDialog } from "./add-system"
import { cn } from "@/lib/utils"

/** Maximum number of rack ticks rendered before collapsing the rest into a count. */
const MAX_TICKS = 144

type TickStatus = "up" | "down" | "paused" | "pending"

const TICK_ORDER: Record<TickStatus, number> = { down: 0, pending: 1, paused: 2, up: 3 }

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

	/** Per-system ticks — problems first so they can't hide in a long row. */
	const ticks = useMemo(() => {
		return systems
			.map((system) => {
				const status: TickStatus =
					system.id in upSystems
						? "up"
						: system.id in downSystems
							? "down"
							: system.id in pausedSystems
								? "paused"
								: "pending"
				return { id: system.id, name: system.name, status }
			})
			.sort((a, b) => TICK_ORDER[a.status] - TICK_ORDER[b.status] || a.name.localeCompare(b.name))
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
				<Card className="bg-blueprint relative overflow-hidden">
					<CardContent className="grid min-h-72 place-items-center p-6 text-center sm:p-10">
						<div className="max-w-xl">
							<div className="mx-auto mb-6 grid size-14 place-items-center rounded-xl border border-primary/25 bg-primary/8 text-primary">
								<RadioTowerIcon className="size-6" aria-hidden="true" />
							</div>
							<h1 className="text-balance text-2xl font-semibold tracking-tight sm:text-3xl">
								<Trans>Connect your first system</Trans>
							</h1>
							<p className="mx-auto mt-3 max-w-lg text-pretty text-sm leading-6 text-muted-foreground sm:text-base">
								<Trans>
									Install a lightweight Beszel agent to start monitoring health, resources, containers, and alerts.
								</Trans>
							</p>
							{!isReadOnlyUser() && (
								<Button className="mt-7" onClick={() => setAddSystemOpen(true)}>
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

	const stats: { label: string; value: number; status: TickStatus; alert?: boolean }[] = [
		{ label: t`Online`, value: statusCounts.up, status: "up" },
		{ label: t`Pending`, value: statusCounts.pending, status: "pending" },
		{ label: t`Down`, value: statusCounts.down, status: "down" },
		{ label: t`Paused`, value: statusCounts.paused, status: "paused" },
		{ label: t`Active alerts`, value: activeAlertCount, status: activeAlertCount ? "pending" : "up", alert: true },
	]

	const shownTicks = ticks.slice(0, MAX_TICKS)
	const hiddenTickCount = ticks.length - shownTicks.length

	return (
		<section aria-label={t`Fleet status`}>
			{/* Page header */}
			<div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
				<div>
					<p className="eyebrow">{t`Fleet`}</p>
					<h1 className="mt-1.5 text-2xl font-semibold tracking-tight sm:text-[1.75rem]">
						<Plural value={systems.length} one="# monitored system" other="# monitored systems" />
					</h1>
				</div>
				<Button
					variant="outline"
					size="sm"
					onClick={refreshFleet}
					disabled={isRefreshing}
					aria-label={t`Refresh fleet`}
				>
					<RefreshCwIcon className={cn("size-3.5", isRefreshing && "animate-spin")} />
					<Trans>Refresh</Trans>
				</Button>
			</div>

			{/* Vitals panel */}
			<Card className="mt-5">
				<CardContent className="p-4 sm:p-5">
					<div
						className="grid gap-1"
						style={{ gridTemplateColumns: "repeat(auto-fill, minmax(6px, 1fr))" }}
						role="img"
						aria-label={`${t`Fleet status`}: ${statusCounts.up} ${t`online`}, ${statusCounts.pending} ${t`pending`}, ${statusCounts.down} ${t`down`}, ${statusCounts.paused} ${t`paused`}`}
					>
						{shownTicks.map((tick) => (
							<Link
								key={tick.id}
								href={getPagePath($router, "system", { id: tick.id })}
								title={`${tick.name} — ${tick.status}`}
								aria-label={tick.name}
								className={cn("rack-tick block h-8 rounded-[3px]", {
									"bg-success": tick.status === "up",
									"bg-destructive animate-pulse": tick.status === "down",
									"bg-warning": tick.status === "pending",
									"bg-muted-foreground/40": tick.status === "paused",
								})}
							/>
						))}
					</div>
					{hiddenTickCount > 0 && (
						<p className="mt-2 font-mono text-[0.65rem] uppercase tracking-[0.14em] text-muted-foreground">
							+{hiddenTickCount}
						</p>
					)}

					<div className="mt-5 grid grid-cols-2 gap-x-4 gap-y-3 border-t pt-4 sm:grid-cols-5">
						{stats.map((stat) => (
							<div key={stat.label} className="flex flex-col gap-1">
								<p className="flex items-center gap-2 text-xs text-muted-foreground">
									<span
										className={cn("led size-1.5 shrink-0", {
											"led-up": stat.status === "up",
											"led-down": stat.status === "down",
											"led-pending": stat.status === "pending",
											"led-paused": stat.status === "paused",
										})}
										aria-hidden="true"
									/>
									{stat.label}
								</p>
								<p className="ps-3.5 font-mono text-2xl font-semibold tabular-nums leading-none">{stat.value}</p>
							</div>
						))}
					</div>
				</CardContent>
			</Card>

			<AddSystemDialog open={addSystemOpen} setOpen={setAddSystemOpen} />
		</section>
	)
}
