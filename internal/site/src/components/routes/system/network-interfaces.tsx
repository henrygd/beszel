import { t } from "@lingui/core/macro"
import {
	CableIcon,
	CheckCircle2Icon,
	CircleOffIcon,
	GaugeIcon,
	Globe2Icon,
	NetworkIcon,
	RouterIcon,
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import type { NetworkInterfaceRecord } from "@/types"

const speedUnits = ["bps", "Kbps", "Mbps", "Gbps", "Tbps"]

function formatLinkSpeed(speed?: number) {
	if (!speed) {
		return t`Not reported`
	}

	let value = speed
	let unitIndex = 0
	while (value >= 1000 && unitIndex < speedUnits.length - 1) {
		value /= 1000
		unitIndex += 1
	}
	const precision = value >= 100 ? 0 : value >= 10 ? 1 : 2
	return `${value.toFixed(precision)} ${speedUnits[unitIndex]}`
}

function isInterfaceUp(iface: NetworkInterfaceRecord) {
	return iface.flags?.some((flag) => flag.toLowerCase() === "up") ?? false
}

function isLoopback(iface: NetworkInterfaceRecord) {
	return iface.flags?.some((flag) => flag.toLowerCase() === "loopback") ?? false
}

function InterfaceMeta({ label, value }: { label: string; value: string }) {
	return (
		<div className="min-w-0">
			<div className="text-[0.65rem] font-medium uppercase tracking-[0.14em] text-muted-foreground">{label}</div>
			<div className="mt-1 truncate font-mono text-xs text-foreground/85" title={value}>
				{value}
			</div>
		</div>
	)
}

function SummaryStat({ label, value, icon: Icon }: { label: string; value: string | number; icon: React.ElementType }) {
	return (
		<div className="flex items-center gap-3 px-4 py-3.5 sm:px-5">
			<Icon className="size-4 shrink-0 text-primary/80" strokeWidth={1.7} />
			<div className="min-w-0">
				<div className="font-mono text-lg font-semibold leading-none tracking-tight tabular-nums">{value}</div>
				<div className="mt-1 truncate text-[0.68rem] font-medium uppercase tracking-[0.13em] text-muted-foreground">{label}</div>
			</div>
		</div>
	)
}

export default function NetworkInterfaces({ interfaces }: { interfaces?: NetworkInterfaceRecord[] }) {
	if (!interfaces) {
		return null
	}

	const onlineCount = interfaces.filter(isInterfaceUp).length
	const addressCount = interfaces.reduce((count, iface) => count + (iface.addresses?.length ?? 0), 0)
	const fastest = interfaces.reduce<number | undefined>((current, iface) => {
		if (!iface.speed) return current
		return current === undefined ? iface.speed : Math.max(current, iface.speed)
	}, undefined)
	const sortedInterfaces = [...interfaces].sort((a, b) => (b.speed ?? 0) - (a.speed ?? 0) || a.name.localeCompare(b.name))

	return (
		<Card className="network-inventory overflow-hidden border-primary/15 bg-card/80 shadow-sm">
			<CardHeader className="relative z-10 border-b border-border/60 px-4 py-5 sm:px-6">
				<div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
					<div className="flex min-w-0 items-start gap-3.5">
						<div className="network-inventory-icon grid size-10 shrink-0 place-items-center rounded-xl border border-primary/20 bg-primary/10 text-primary shadow-inner">
							<NetworkIcon className="size-5" strokeWidth={1.7} />
						</div>
						<div className="min-w-0">
							<div className="mb-1 font-mono text-[0.65rem] font-semibold uppercase tracking-[0.18em] text-primary/80">
								{t`Host inventory`}
							</div>
							<CardTitle className="text-xl sm:text-2xl">{t`Network interfaces`}</CardTitle>
							<CardDescription className="mt-1 max-w-2xl leading-relaxed">
								{t`Detected links, addresses, and negotiated speeds reported by the agent`}
							</CardDescription>
						</div>
					</div>
					<Badge variant="outline" className="w-fit gap-1.5 border-primary/25 bg-primary/5 px-2.5 py-1 font-mono text-[0.68rem] uppercase tracking-[0.1em]">
						<span className="size-1.5 rounded-full bg-primary" />
						{t`Agent reported`}
					</Badge>
				</div>
			</CardHeader>

			<div className="relative z-10 grid grid-cols-2 divide-x divide-y border-b border-border/60 sm:grid-cols-4 sm:divide-y-0 rtl:divide-x-reverse">
				<SummaryStat label={t`Detected`} value={interfaces.length} icon={RouterIcon} />
				<SummaryStat label={t`Online`} value={onlineCount} icon={onlineCount ? CheckCircle2Icon : CircleOffIcon} />
				<SummaryStat label={t`Addresses`} value={addressCount} icon={Globe2Icon} />
				<SummaryStat label={t`Fastest link`} value={formatLinkSpeed(fastest)} icon={GaugeIcon} />
			</div>

			<CardContent className="relative z-10 p-4 sm:p-6">
				{sortedInterfaces.length ? (
					<div className="grid gap-3 lg:grid-cols-2">
						{sortedInterfaces.map((iface) => {
							const online = isInterfaceUp(iface)
							const loopback = isLoopback(iface)
							const addresses = iface.addresses ?? []
							return (
								<div
									key={iface.name}
									className={cn(
										"group rounded-xl border border-border/70 bg-background/45 p-4 transition-colors hover:border-primary/35 hover:bg-primary/[0.03]",
										!online && "opacity-75"
									)}
								>
									<div className="flex items-start justify-between gap-3">
										<div className="flex min-w-0 items-center gap-3">
											<div className={cn("grid size-9 shrink-0 place-items-center rounded-lg border", online ? "border-emerald-500/25 bg-emerald-500/10 text-emerald-500" : "border-border bg-muted text-muted-foreground")}>
												<CableIcon className="size-4" strokeWidth={1.8} />
											</div>
											<div className="min-w-0">
												<div className="truncate font-mono text-sm font-semibold tracking-tight">{iface.name}</div>
												<div className="mt-1 flex flex-wrap items-center gap-2 text-[0.68rem] font-medium uppercase tracking-[0.1em] text-muted-foreground">
													<span className="inline-flex items-center gap-1.5">
														<span className={cn("size-1.5 rounded-full", online ? "bg-emerald-500" : "bg-muted-foreground/50")} />
														{online ? t`Online` : t`Offline`}
													</span>
													{loopback && <Badge variant="secondary" className="px-1.5 py-0 text-[0.6rem]">{t`Loopback`}</Badge>}
												</div>
											</div>
										</div>
										<div className="shrink-0 text-end">
											<div className="font-mono text-sm font-semibold tabular-nums text-primary">{formatLinkSpeed(iface.speed)}</div>
											<div className="mt-1 text-[0.62rem] font-medium uppercase tracking-[0.12em] text-muted-foreground">{t`Link speed`}</div>
										</div>
									</div>

									<div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 border-t border-border/60 pt-3">
										<InterfaceMeta label="MAC" value={iface.mac || t`Not available`} />
										<InterfaceMeta label="MTU" value={iface.mtu ? `${iface.mtu} bytes` : t`Not available`} />
									</div>

									{addresses.length > 0 && (
										<div className="mt-3 flex flex-wrap gap-1.5">
											{addresses.map((address) => (
												<span key={address} className="rounded-md border border-border/60 bg-muted/45 px-2 py-1 font-mono text-[0.68rem] text-muted-foreground">
													{address}
												</span>
											))}
										</div>
									)}
								</div>
							)
						})}
					</div>
				) : (
					<div className="rounded-xl border border-dashed border-border/80 bg-background/35 px-4 py-8 text-center">
						<NetworkIcon className="mx-auto size-6 text-muted-foreground/60" strokeWidth={1.5} />
						<p className="mt-3 text-sm font-medium">{t`No network interfaces reported`}</p>
						<p className="mt-1 text-sm text-muted-foreground">{t`The agent did not return an interface inventory for this host.`}</p>
					</div>
				)}
			</CardContent>
		</Card>
	)
}
