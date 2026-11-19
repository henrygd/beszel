"use client"

import { useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import { CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Collapsible } from "@/components/ui/collapsible"
import {
	Globe,
	Server,
	Activity,
	ArrowUpRight,
	ArrowDownRight,
	Minus,
	ExternalLink,
} from "lucide-react"
import {
	listMonitors,
	groupMonitorsByDomain,
	getMonitorStatusColor,
	formatUptime,
	type Monitor,
	type GroupedMonitors,
} from "@/lib/monitors"
import { Link } from "@/components/router"
import { cn } from "@/lib/utils"

function DomainGroupHeader({
	domain,
	group,
	monitorCount,
}: {
	domain: string
	group: GroupedMonitors
	monitorCount: number
}) {
	// Calculate aggregate status for the domain
	const allMonitors = [...group.monitors, ...Array.from(group.subdomains.values()).flat()]
	const upCount = allMonitors.filter((m) => m.status === "up").length
	const downCount = allMonitors.filter((m) => m.status === "down").length
	const pausedCount = allMonitors.filter((m) => m.status === "paused").length

	const statusColor =
		downCount > 0 ? "bg-red-500" : upCount > 0 ? "bg-green-500" : pausedCount > 0 ? "bg-yellow-500" : "bg-gray-400"

	return (
		<div className="flex items-center gap-3 py-2">
			<div className={cn("h-3 w-3 rounded-full", statusColor)} />
			<div className="flex-1">
				<div className="flex items-center gap-2">
					<Globe className="h-4 w-4 text-muted-foreground" />
					<span className="font-semibold">{domain}</span>
					<Badge variant="secondary" className="text-xs">
						{monitorCount} monitor{monitorCount !== 1 ? "s" : ""}
					</Badge>
				</div>
			</div>
			<div className="flex items-center gap-2 text-sm text-muted-foreground">
				{upCount > 0 && (
					<span className="flex items-center gap-1 text-green-600">
						<ArrowUpRight className="h-3.5 w-3.5" />
						{upCount}
					</span>
				)}
				{downCount > 0 && (
					<span className="flex items-center gap-1 text-red-600">
						<ArrowDownRight className="h-3.5 w-3.5" />
						{downCount}
					</span>
				)}
				{pausedCount > 0 && (
					<span className="flex items-center gap-1 text-yellow-600">
						<Minus className="h-3.5 w-3.5" />
						{pausedCount}
					</span>
				)}
			</div>
		</div>
	)
}

function MonitorCard({ monitor }: { monitor: Monitor }) {
	const uptime24h = monitor.uptime_stats?.["24h"] ?? 100
	const statusColor = getMonitorStatusColor(monitor.status)

	return (
		<Link href={`/monitor/${monitor.id}`}>
			<div className="group relative rounded-lg border p-3 hover:bg-muted/50 transition-colors cursor-pointer">
				<div className="flex items-start gap-3">
					<div className={cn("mt-1 h-2.5 w-2.5 rounded-full", statusColor)} />
					<div className="flex-1 min-w-0">
						<div className="flex items-center gap-2">
							<span className="font-medium text-sm truncate">{monitor.name}</span>
							{monitor.active === false && (
								<Badge variant="secondary" className="text-[10px]">
									Paused
								</Badge>
							)}
						</div>
						<div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
							<Activity className="h-3 w-3" />
							<span>24h: {formatUptime(uptime24h)}</span>
						</div>
					</div>
					<ExternalLink className="h-3.5 w-3.5 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
				</div>
			</div>
		</Link>
	)
}

function SubdomainSection({
	subdomain,
	monitors,
}: {
	subdomain: string
	monitors: Monitor[]
}) {
	return (
		<Collapsible
			title={`${subdomain} (${monitors.length})`}
			icon={<Server className="h-4 w-4" />}
			defaultOpen={true}
			className="ml-4 border-l-2 border-muted rounded-none border-t-0 border-r-0 border-b-0"
		>
			<div className="pl-6 py-1 space-y-1">
				{monitors.map((monitor) => (
					<MonitorCard key={monitor.id} monitor={monitor} />
				))}
			</div>
		</Collapsible>
	)
}

function DomainGroup({ domain, group }: { domain: string; group: GroupedMonitors }) {
	const monitorCount = group.monitors.length + Array.from(group.subdomains.values()).flat().length

	const content = (
		<CardContent className="pt-0 pb-4 px-4">
			{/* Root domain monitors */}
			{group.monitors.length > 0 && (
				<div className="mb-3">
					<div className="flex items-center gap-2 py-1.5 text-sm text-muted-foreground">
						<Globe className="h-3.5 w-3.5" />
						<span>Root Domain</span>
					</div>
					<div className="pl-6 space-y-1">
						{group.monitors.map((monitor) => (
							<MonitorCard key={monitor.id} monitor={monitor} />
						))}
					</div>
				</div>
			)}

			{/* Subdomain sections */}
			{Array.from(group.subdomains.entries()).map(([subdomain, monitors]) => (
				<SubdomainSection key={subdomain} subdomain={subdomain} monitors={monitors} />
			))}
		</CardContent>
	)

	return (
		<Collapsible
			title={domain}
			icon={<Globe className="h-4 w-4" />}
			defaultOpen={true}
			description={<DomainGroupHeader domain={domain} group={group} monitorCount={monitorCount} />}
		>
			{content}
		</Collapsible>
	)
}

export function GroupedMonitorsTable() {
	const { data: monitors, isLoading } = useQuery({
		queryKey: ["monitors"],
		queryFn: listMonitors,
	})

	const groupedMonitors = useMemo(() => {
		if (!monitors) return new Map()
		return groupMonitorsByDomain(monitors)
	}, [monitors])

	const ungroupedMonitors = useMemo(() => {
		if (!monitors) return []
		return monitors.filter((m) => !m.url && !m.hostname)
	}, [monitors])

	if (isLoading) {
		return (
			<div className="space-y-4">
				{[1, 2, 3].map((i) => (
					<div key={i} className="h-32 bg-muted rounded-lg animate-pulse" />
				))}
			</div>
		)
	}

	const domainGroups = Array.from(groupedMonitors.entries()).sort((a, b) => a[0].localeCompare(b[0]))

	return (
		<div className="space-y-4">
			{/* Domain groups */}
			{domainGroups.map(([domain, group]) => (
				<DomainGroup key={domain} domain={domain} group={group} />
			))}

			{/* Ungrouped monitors (no URL/hostname) */}
			{ungroupedMonitors.length > 0 && (
				<Collapsible
					title="Other Monitors"
					icon={<Server className="h-4 w-4" />}
					defaultOpen={true}
				>
					<div className="space-y-1">
						{ungroupedMonitors.map((monitor) => (
							<MonitorCard key={monitor.id} monitor={monitor} />
						))}
					</div>
				</Collapsible>
			)}

			{/* Empty state */}
			{domainGroups.length === 0 && ungroupedMonitors.length === 0 && (
				<div className="text-center py-12">
					<Activity className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
					<p className="text-lg font-medium">No monitors yet</p>
					<p className="text-muted-foreground">Create your first monitor to get started</p>
				</div>
			)}
		</div>
	)
}
