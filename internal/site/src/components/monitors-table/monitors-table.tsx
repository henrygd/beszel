import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import {
	AlertTriangle,
	ArrowDownIcon,
	ArrowUpIcon,
	CheckCircleIcon,
	Edit3Icon,
	FilterIcon,
	GlobeIcon,
	LayoutGridIcon,
	LayoutListIcon,
	PauseIcon,
	PlayIcon,
	PlusIcon,
	RefreshCwIcon,
	Settings2Icon,
	TagIcon,
	Trash2Icon,
	XCircle,
	XCircleIcon,
} from "lucide-react"
import { memo, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import { useToast } from "@/components/ui/use-toast"
import {
	deleteMonitor,
	getMonitorTypeLabel,
	getMonitorFaviconUrl,
	listMonitors,
	manualCheck,
	pauseMonitor,
	resumeMonitor,
	type Monitor,
	type MonitorStatus,
	type MonitorType,
	formatUptime,
	formatPing,
} from "@/lib/monitors"
import { cn, useBrowserStorage } from "@/lib/utils"
import { AddMonitorDialog } from "./add-monitor-dialog"
import { Link } from "@/components/router"

// Status indicator component
function StatusIndicator({ status }: { status: MonitorStatus }) {
	const colors = {
		up: "bg-green-500",
		down: "bg-red-500",
		pending: "bg-yellow-400",
		paused: "bg-gray-400",
		maintenance: "bg-blue-500",
	}

	const icons = {
		up: CheckCircleIcon,
		down: XCircleIcon,
		pending: RefreshCwIcon,
		paused: PauseIcon,
		maintenance: RefreshCwIcon,
	}

	const Icon = icons[status] || RefreshCwIcon

	return (
		<div className="flex items-center gap-2">
			<div className={cn("h-2.5 w-2.5 rounded-full", colors[status])} />
			<Icon className="h-4 w-4 text-muted-foreground" />
			<span className="capitalize text-sm">{status}</span>
		</div>
	)
}

// Favicon component
function MonitorFavicon({ monitor, className }: { monitor: Monitor; className?: string }) {
	const [error, setError] = useState(false)
	const faviconUrl = getMonitorFaviconUrl(monitor)

	if (!faviconUrl || error) {
		return <GlobeIcon className={cn("h-4 w-4 text-muted-foreground", className)} />
	}

	return (
		<img
			src={faviconUrl}
			alt=""
			className={cn("h-4 w-4 object-contain", className)}
			onError={() => setError(true)}
			loading="lazy"
		/>
	)
}

// Monitor Card component for grid view
function MonitorCard({
	monitor,
	onEdit,
	displayOptions,
}: {
	monitor: Monitor
	onEdit: (m: Monitor) => void
	displayOptions: DisplayOptions
}) {
	const { toast } = useToast()
	const queryClient = useQueryClient()

	const checkMutation = useMutation({
		mutationFn: manualCheck,
		onSuccess: (result) => {
			toast({
				title: `Check complete`,
				description: `${monitor.name} is ${result.status} (${formatPing(result.ping)})`,
			})
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
		},
	})

	const pauseMutation = useMutation({
		mutationFn: monitor.status === "paused" ? resumeMonitor : pauseMonitor,
		onSuccess: () => {
			toast({
				title: monitor.status === "paused" ? "Monitor resumed" : "Monitor paused",
			})
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
		},
	})

	const deleteMutation = useMutation({
		mutationFn: deleteMonitor,
		onSuccess: () => {
			toast({ title: "Monitor deleted" })
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
		},
	})

	return (
		<div className="rounded-lg border bg-card p-4 space-y-4 hover:shadow-md transition-shadow">
			<div className="flex items-start justify-between">
				<Link href={`/monitor/${monitor.id}`} className="flex items-center gap-3 cursor-pointer min-w-0">
					<div className="shrink-0">
						<div className="flex items-center gap-2">
							<MonitorFavicon monitor={monitor} className="h-5 w-5" />
							<StatusIndicator status={monitor.status} />
						</div>
					</div>
					<div className="min-w-0">
						<div className="font-medium truncate hover:underline">{monitor.name}</div>
						<div className="text-xs text-muted-foreground truncate">
							{monitor.url || monitor.hostname}
							{monitor.port ? `:${monitor.port}` : ""}
						</div>
					</div>
				</Link>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="ghost" size="icon" className="h-8 w-8 shrink-0">
							<Edit3Icon className="h-4 w-4" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem onClick={() => onEdit(monitor)}>
							<Edit3Icon className="mr-2 h-4 w-4" />
							Edit
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => deleteMutation.mutate(monitor.id)} className="text-destructive">
							<Trash2Icon className="mr-2 h-4 w-4" />
							Delete
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</div>

			<div className="space-y-3">
				<div className="flex items-center justify-between">
					<div className="text-xs text-muted-foreground">Type</div>
					<div className="inline-flex items-center rounded-md bg-muted px-2 py-1 text-xs font-medium">
						{getMonitorTypeLabel(monitor.type)}
					</div>
				</div>

				{/* Uptime - Prominent pill display */}
				{displayOptions.showUptimePills && (
					<div className="flex flex-col gap-2">
						<div className="flex items-center gap-2 flex-wrap">
							{displayOptions.showUptimePercentage && (
								<UptimePill uptime={monitor.uptime_stats?.uptime_24h ?? 100} label="24h" />
							)}
							{displayOptions.showUptimePercentage &&
								monitor.uptime_stats?.uptime_7d !== undefined &&
								monitor.uptime_stats.uptime_7d !== monitor.uptime_stats?.uptime_24h && (
									<UptimePill uptime={monitor.uptime_stats.uptime_7d} label="7d" />
								)}
						</div>
						{displayOptions.showHeartbeatDots && <UptimeDots heartbeats={monitor.recent_heartbeats} />}
					</div>
				)}

				<div className="flex items-center justify-between text-sm">
					<div className="text-xs text-muted-foreground">Response</div>
					<div>
						{monitor.last_check ? (
							formatPing(monitor.uptime_stats?.last_ping || 0)
						) : (
							<span className="text-muted-foreground">-</span>
						)}
					</div>
				</div>
			</div>

			{monitor.tags && monitor.tags.length > 0 && (
				<div className="flex flex-wrap gap-1">
					{monitor.tags.map((tag) => (
						<span
							key={tag}
							className="inline-flex items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium"
						>
							<TagIcon className="h-3 w-3" />
							{tag}
						</span>
					))}
				</div>
			)}

			<div className="flex items-center gap-2 pt-2 border-t">
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="sm"
								className="flex-1"
								onClick={() => checkMutation.mutate(monitor.id)}
								disabled={checkMutation.isPending}
							>
								<RefreshCwIcon className={cn("h-4 w-4 mr-1", checkMutation.isPending && "animate-spin")} />
								Check
							</Button>
						</TooltipTrigger>
						<TooltipContent>
							<p>Check now</p>
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="sm"
								className="flex-1"
								onClick={() => pauseMutation.mutate(monitor.id)}
								disabled={pauseMutation.isPending}
							>
								{monitor.status === "paused" ? (
									<>
										<PlayIcon className="h-4 w-4 mr-1" /> Resume
									</>
								) : (
									<>
										<PauseIcon className="h-4 w-4 mr-1" /> Pause
									</>
								)}
							</Button>
						</TooltipTrigger>
						<TooltipContent>
							<p>{monitor.status === "paused" ? "Resume" : "Pause"}</p>
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</div>
		</div>
	)
}

// Uptime pill badge component - styled like domainstack.io status badges
function UptimePill({ uptime, label = "24h" }: { uptime: number; label?: string }) {
	let colorClass = "bg-green-500 text-white border-green-600"
	let icon = <CheckCircleIcon className="h-3.5 w-3.5 text-white" />
	let ringClass = "ring-green-500/30"

	if (uptime < 99.9) {
		colorClass = "bg-green-500 text-white border-green-600"
		ringClass = "ring-green-500/30"
	}
	if (uptime < 95) {
		colorClass = "bg-yellow-500 text-white border-yellow-600"
		icon = <AlertTriangle className="h-3.5 w-3.5 text-white" />
		ringClass = "ring-yellow-500/30"
	}
	if (uptime < 90) {
		colorClass = "bg-red-500 text-white border-red-600"
		icon = <XCircle className="h-3.5 w-3.5 text-white" />
		ringClass = "ring-red-500/30"
	}

	return (
		<div
			className={cn(
				"inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border text-xs font-semibold shadow-sm",
				"transition-all hover:scale-105",
				colorClass,
				ringClass,
				"ring-1"
			)}
		>
			{icon}
			<span>{formatUptime(uptime)}</span>
			<span className="text-[10px] font-medium uppercase opacity-80 border-l border-white/30 pl-1.5 ml-0.5">
				{label}
			</span>
		</div>
	)
}

// Uptime bar component with pill style
function UptimeBar({ stats }: { stats?: Record<string, number> }) {
	const uptime24h = stats?.uptime_24h ?? 100
	const uptime7d = stats?.uptime_7d ?? 100
	const uptime30d = stats?.uptime_30d ?? 100

	return (
		<div className="flex flex-col gap-1.5">
			<div className="flex items-center gap-2">
				<UptimePill uptime={uptime24h} label="24h" />
				{uptime7d !== 100 && uptime7d !== uptime24h && <UptimePill uptime={uptime7d} label="7d" />}
				{uptime30d !== 100 && uptime30d !== uptime24h && uptime30d !== uptime7d && (
					<UptimePill uptime={uptime30d} label="30d" />
				)}
			</div>
		</div>
	)
}

// Mini uptime dots visualization - styled as a clean status bar
function UptimeDots({ heartbeats }: { heartbeats?: Array<{ status: string; time: string }> }) {
	const totalSlots = 16
	const recent = heartbeats?.slice(-totalSlots) || []
	const emptySlots = totalSlots - recent.length

	const getStatusColor = (status: string) => {
		switch (status) {
			case "up":
				return "bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)] dark:shadow-[0_0_10px_rgba(34,197,94,0.8)]"
			case "down":
				return "bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.6)] dark:shadow-[0_0_10px_rgba(239,68,68,0.8)]"
			case "paused":
				return "bg-gray-400"
			default:
				return "bg-yellow-500 shadow-[0_0_8px_rgba(234,179,8,0.6)] dark:shadow-[0_0_10px_rgba(234,179,8,0.8)]"
		}
	}

	return (
		<TooltipProvider>
			<div className="flex items-center gap-1">
				<div className="flex gap-[3px] p-1.5 rounded-lg bg-muted/50 border border-border/50">
					{recent.map((hb, i) => (
						<Tooltip key={i}>
							<TooltipTrigger asChild>
								<div
									className={cn(
										"h-5 w-[7px] rounded-full transition-all cursor-pointer hover:scale-125",
										getStatusColor(hb.status)
									)}
								/>
							</TooltipTrigger>
							<TooltipContent side="bottom" className="text-xs">
								<p className="capitalize font-medium">{hb.status}</p>
								<p className="text-muted-foreground">{new Date(hb.time).toLocaleString()}</p>
							</TooltipContent>
						</Tooltip>
					))}
					{emptySlots > 0 &&
						Array.from({ length: emptySlots }).map((_, i) => (
							<div key={`empty-${i}`} className="h-5 w-[7px] rounded-full bg-muted/80" />
						))}
				</div>
			</div>
		</TooltipProvider>
	)
}

// Monitor row component
function MonitorRow({
	monitor,
	onEdit,
	displayOptions,
}: {
	monitor: Monitor
	onEdit: (m: Monitor) => void
	displayOptions: DisplayOptions
}) {
	const { toast } = useToast()
	const queryClient = useQueryClient()

	const checkMutation = useMutation({
		mutationFn: manualCheck,
		onSuccess: (result) => {
			toast({
				title: `Check complete`,
				description: `${monitor.name} is ${result.status} (${formatPing(result.ping)})`,
			})
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
		},
		onError: () => {
			toast({
				title: "Check failed",
				variant: "destructive",
			})
		},
	})

	const pauseMutation = useMutation({
		mutationFn: monitor.status === "paused" ? resumeMonitor : pauseMonitor,
		onSuccess: () => {
			toast({
				title: monitor.status === "paused" ? "Monitor resumed" : "Monitor paused",
			})
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
		},
	})

	const deleteMutation = useMutation({
		mutationFn: deleteMonitor,
		onSuccess: () => {
			toast({ title: "Monitor deleted" })
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
		},
	})

	return (
		<TableRow>
			<TableCell>
				<Link href={`/monitor/${monitor.id}`} className="flex items-center gap-3 cursor-pointer">
					<MonitorFavicon monitor={monitor} className="h-5 w-5" />
					<div>
						<div className="font-medium hover:underline">{monitor.name}</div>
						<div className="text-xs text-muted-foreground">
							{monitor.url || monitor.hostname}
							{monitor.port ? `:${monitor.port}` : ""}
						</div>
					</div>
				</Link>
			</TableCell>
			<TableCell>
				<span className="inline-flex items-center rounded-md bg-muted px-2 py-1 text-xs font-medium">
					{getMonitorTypeLabel(monitor.type)}
				</span>
			</TableCell>
			<TableCell>
				<StatusIndicator status={monitor.status} />
			</TableCell>
			<TableCell>
				{monitor.last_check ? (
					<div className="text-sm">{formatPing(monitor.uptime_stats?.last_ping || 0)}</div>
				) : (
					<span className="text-sm text-muted-foreground">-</span>
				)}
			</TableCell>
			<TableCell>
				{displayOptions.showUptimePills ? (
					<UptimeBar stats={monitor.uptime_stats} />
				) : (
					<span className="text-sm text-muted-foreground">-</span>
				)}
			</TableCell>
			<TableCell>
				<div className="flex flex-wrap gap-1">
					{monitor.tags?.map((tag) => (
						<span
							key={tag}
							className="inline-flex items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium"
						>
							<TagIcon className="h-3 w-3" />
							{tag}
						</span>
					))}
				</div>
			</TableCell>
			<TableCell className="text-right">
				<div className="flex items-center justify-end gap-1">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									size="icon"
									className="h-8 w-8"
									onClick={() => checkMutation.mutate(monitor.id)}
									disabled={checkMutation.isPending}
								>
									<RefreshCwIcon className={cn("h-4 w-4", checkMutation.isPending && "animate-spin")} />
								</Button>
							</TooltipTrigger>
							<TooltipContent>
								<p>Check now</p>
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>

					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									size="icon"
									className="h-8 w-8"
									onClick={() => pauseMutation.mutate(monitor.id)}
									disabled={pauseMutation.isPending}
								>
									{monitor.status === "paused" ? <PlayIcon className="h-4 w-4" /> : <PauseIcon className="h-4 w-4" />}
								</Button>
							</TooltipTrigger>
							<TooltipContent>
								<p>{monitor.status === "paused" ? "Resume" : "Pause"}</p>
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>

					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button variant="ghost" size="icon" className="h-8 w-8">
								<Edit3Icon className="h-4 w-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem onClick={() => onEdit(monitor)}>
								<Edit3Icon className="mr-2 h-4 w-4" />
								Edit
							</DropdownMenuItem>
							<DropdownMenuItem className="text-destructive" onClick={() => deleteMutation.mutate(monitor.id)}>
								<Trash2Icon className="mr-2 h-4 w-4" />
								Delete
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			</TableCell>
		</TableRow>
	)
}

type ViewMode = "table" | "grid"
type StatusFilter = "all" | MonitorStatus
type TypeFilter = "all" | MonitorType

interface DisplayOptions {
	showUptimePills: boolean
	showUptimePercentage: boolean
	showHeartbeatDots: boolean
}

// Main component
export default memo(function MonitorsTable() {
	const { t } = useLingui()
	const [filter, setFilter] = useState("")
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
	const [tagFilter, setTagFilter] = useState<string>("all")
	const [typeFilter, setTypeFilter] = useState<TypeFilter>("all")
	const [isAddDialogOpen, setIsAddDialogOpen] = useState(false)
	const [editingMonitor, setEditingMonitor] = useState<Monitor | null>(null)

	const [viewMode, setViewMode] = useBrowserStorage<ViewMode>(
		"monitorsViewMode",
		window.innerWidth < 1024 ? "grid" : "table"
	)

	const [displayOptions, setDisplayOptions] = useBrowserStorage<DisplayOptions>("monitorsDisplayOptions", {
		showUptimePills: true,
		showUptimePercentage: true,
		showHeartbeatDots: true,
	})

	const { data: monitors = [], isLoading } = useQuery({
		queryKey: ["monitors"],
		queryFn: listMonitors,
		refetchInterval: 30000,
	})

	// Extract all unique types
	const allTypes = useMemo(() => {
		const typeSet = new Set<MonitorType>()
		monitors.forEach((m) => typeSet.add(m.type))
		return Array.from(typeSet).sort()
	}, [monitors])

	// Filter by status first
	const statusFilteredMonitors = useMemo(() => {
		if (statusFilter === "all") return monitors
		return monitors.filter((m) => m.status === statusFilter)
	}, [monitors, statusFilter])

	// Then filter by search text and type
	const filteredMonitors = useMemo(() => {
		let result = statusFilteredMonitors
		if (filter) {
			const f = filter.toLowerCase()
			result = result.filter(
				(m) =>
					m.name.toLowerCase().includes(f) ||
					(m.url || "").toLowerCase().includes(f) ||
					(m.hostname || "").toLowerCase().includes(f)
			)
		}
		if (tagFilter !== "all") {
			result = result.filter((m) => m.tags?.includes(tagFilter))
		}
		if (typeFilter !== "all") {
			result = result.filter((m) => m.type === typeFilter)
		}
		return result
	}, [statusFilteredMonitors, filter, tagFilter, typeFilter])

	// Extract all unique tags
	const allTags = useMemo(() => {
		const tagSet = new Set<string>()
		monitors.forEach((m) => m.tags?.forEach((tag) => tagSet.add(tag)))
		return Array.from(tagSet).sort()
	}, [monitors])

	const stats = useMemo(() => {
		const total = monitors.length
		const up = monitors.filter((m) => m.status === "up").length
		const down = monitors.filter((m) => m.status === "down").length
		const paused = monitors.filter((m) => m.status === "paused").length
		const pending = monitors.filter((m) => m.status === "pending").length
		const maintenance = monitors.filter((m) => m.status === "maintenance").length
		return { total, up, down, paused, pending, maintenance }
	}, [monitors])

	return (
		<Card className="w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 pb-5">
				<div className="flex flex-col gap-4">
					{/* Title row */}
					<div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
						<div className="flex-1">
							<CardTitle className="text-xl mb-2 flex items-center gap-2">
								<GlobeIcon className="h-5 w-5 text-primary" />
								<Trans>Status</Trans>
							</CardTitle>
							<CardDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
								<Trans>Monitor websites, APIs, and services</Trans>
								<span className="text-xs text-muted-foreground">
									({stats.up} <ArrowUpIcon className="inline h-3 w-3 text-green-500" />
									{stats.down > 0 && (
										<>
											{" "}
											{stats.down} <ArrowDownIcon className="inline h-3 w-3 text-red-500" />
										</>
									)}
									{stats.paused > 0 && (
										<>
											{" "}
											{stats.paused} <PauseIcon className="inline h-3 w-3 text-gray-400" />
										</>
									)}
									/ {stats.total})
								</span>
							</CardDescription>
						</div>
						<Button onClick={() => setIsAddDialogOpen(true)} className="shrink-0">
							<PlusIcon className="mr-2 h-4 w-4" />
							<Trans>Add Monitor</Trans>
						</Button>
					</div>

					{/* Quick status filters */}
					<div className="flex flex-wrap gap-1.5">
						{[
							{ key: "all", label: `All ${stats.total}`, color: "bg-primary" },
							{ key: "up", label: `Up ${stats.up}`, color: "bg-green-500" },
							{ key: "down", label: `Down ${stats.down}`, color: "bg-red-500" },
							{ key: "paused", label: `Paused ${stats.paused}`, color: "bg-gray-400" },
						].map((s) => (
							<Button
								key={s.key}
								variant={statusFilter === s.key ? "default" : "outline"}
								size="sm"
								className="h-7 text-xs gap-1.5"
								onClick={() => setStatusFilter(s.key as StatusFilter)}
								disabled={s.key !== "all" && parseInt(s.label.split(" ")[1]) === 0}
							>
								<span className={`h-2 w-2 rounded-full ${s.color}`} />
								{s.label.split(" ")[0]}
								<span className="text-[10px] opacity-70">{s.label.split(" ")[1]}</span>
							</Button>
						))}
					</div>

					{/* Filter row */}
					<div className="flex flex-col sm:flex-row gap-2">
						<div className="relative flex-1">
							<Input
								placeholder={t`Filter monitors...`}
								onChange={(e) => setFilter(e.target.value)}
								value={filter}
								className="w-full"
							/>
						</div>
						{allTypes.length > 0 && (
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" size="sm">
										<GlobeIcon className="me-1.5 size-4 opacity-80" />
										{typeFilter === "all" ? t`Type` : getMonitorTypeLabel(typeFilter)}
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuRadioGroup value={typeFilter} onValueChange={(v) => setTypeFilter(v as TypeFilter)}>
										<DropdownMenuRadioItem value="all">
											<Trans>All Types</Trans>
										</DropdownMenuRadioItem>
										{allTypes.map((type) => (
											<DropdownMenuRadioItem key={type} value={type}>
												{getMonitorTypeLabel(type)}
											</DropdownMenuRadioItem>
										))}
									</DropdownMenuRadioGroup>
								</DropdownMenuContent>
							</DropdownMenu>
						)}
						{allTags.length > 0 && (
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" size="sm">
										<TagIcon className="me-1.5 size-4 opacity-80" />
										{tagFilter === "all" ? t`Tags` : tagFilter}
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuRadioGroup value={tagFilter} onValueChange={setTagFilter}>
										<DropdownMenuRadioItem value="all">
											<Trans>All Tags</Trans>
										</DropdownMenuRadioItem>
										{allTags.map((tag) => (
											<DropdownMenuRadioItem key={tag} value={tag}>
												{tag}
											</DropdownMenuRadioItem>
										))}
									</DropdownMenuRadioGroup>
								</DropdownMenuContent>
							</DropdownMenu>
						)}
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline" size="sm">
									<Settings2Icon className="me-1.5 size-4 opacity-80" />
									<Trans>Options</Trans>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" className="min-w-48">
								{/* Layout */}
								<DropdownMenuLabel className="flex items-center gap-2">
									<LayoutGridIcon className="size-4" />
									<Trans>Layout</Trans>
								</DropdownMenuLabel>
								<DropdownMenuRadioGroup value={viewMode} onValueChange={(v) => setViewMode(v as ViewMode)}>
									<DropdownMenuRadioItem value="table" className="gap-2">
										<LayoutListIcon className="size-4" />
										<Trans>Table</Trans>
									</DropdownMenuRadioItem>
									<DropdownMenuRadioItem value="grid" className="gap-2">
										<LayoutGridIcon className="size-4" />
										<Trans>Grid</Trans>
									</DropdownMenuRadioItem>
								</DropdownMenuRadioGroup>
								<DropdownMenuSeparator />

								{/* Display Options */}
								<DropdownMenuLabel className="flex items-center gap-2">
									<Settings2Icon className="size-4" />
									<Trans>Display</Trans>
								</DropdownMenuLabel>
								<div className="px-2 py-1 space-y-1">
									<label className="flex items-center gap-2 text-sm cursor-pointer hover:bg-muted/50 rounded px-1 py-0.5">
										<input
											type="checkbox"
											checked={displayOptions.showUptimePills}
											onChange={(e) => setDisplayOptions({ ...displayOptions, showUptimePills: e.target.checked })}
											className="rounded border-gray-300"
										/>
										<span>Show uptime pills</span>
									</label>
									<label className="flex items-center gap-2 text-sm cursor-pointer hover:bg-muted/50 rounded px-1 py-0.5">
										<input
											type="checkbox"
											checked={displayOptions.showUptimePercentage}
											onChange={(e) => setDisplayOptions({ ...displayOptions, showUptimePercentage: e.target.checked })}
											className="rounded border-gray-300"
										/>
										<span>Show percentage</span>
									</label>
									<label className="flex items-center gap-2 text-sm cursor-pointer hover:bg-muted/50 rounded px-1 py-0.5">
										<input
											type="checkbox"
											checked={displayOptions.showHeartbeatDots}
											onChange={(e) => setDisplayOptions({ ...displayOptions, showHeartbeatDots: e.target.checked })}
											className="rounded border-gray-300"
										/>
										<span>Show heartbeat dots</span>
									</label>
								</div>
								<DropdownMenuSeparator />

								{/* Status Filter */}
								<DropdownMenuLabel className="flex items-center gap-2">
									<FilterIcon className="size-4" />
									<Trans>Status</Trans>
								</DropdownMenuLabel>
								<DropdownMenuRadioGroup value={statusFilter} onValueChange={(v) => setStatusFilter(v as StatusFilter)}>
									<DropdownMenuRadioItem value="all">
										<Trans>All ({stats.total})</Trans>
									</DropdownMenuRadioItem>
									<DropdownMenuRadioItem value="up">
										<Trans>Up ({stats.up})</Trans>
									</DropdownMenuRadioItem>
									<DropdownMenuRadioItem value="down">
										<Trans>Down ({stats.down})</Trans>
									</DropdownMenuRadioItem>
									<DropdownMenuRadioItem value="paused">
										<Trans>Paused ({stats.paused})</Trans>
									</DropdownMenuRadioItem>
									{stats.pending > 0 && (
										<DropdownMenuRadioItem value="pending">
											<Trans>Pending ({stats.pending})</Trans>
										</DropdownMenuRadioItem>
									)}
									{stats.maintenance > 0 && (
										<DropdownMenuRadioItem value="maintenance">
											<Trans>Maintenance ({stats.maintenance})</Trans>
										</DropdownMenuRadioItem>
									)}
								</DropdownMenuRadioGroup>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</CardHeader>
			<CardContent className="p-0">
				{isLoading ? (
					<div className="p-8 text-center text-muted-foreground">
						<Trans>Loading...</Trans>
					</div>
				) : filteredMonitors.length === 0 ? (
					<div className="p-8 text-center text-muted-foreground">
						{filter || statusFilter !== "all" || tagFilter !== "all" || typeFilter !== "all" ? (
							<Trans>No monitors match your filters.</Trans>
						) : (
							<div>
								<p className="mb-4">
									<Trans>No monitors configured yet.</Trans>
								</p>
								<Button onClick={() => setIsAddDialogOpen(true)} variant="outline">
									<PlusIcon className="mr-2 h-4 w-4" />
									<Trans>Add your first monitor</Trans>
								</Button>
							</div>
						)}
					</div>
				) : viewMode === "table" ? (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>
									<Trans>Name</Trans>
								</TableHead>
								<TableHead>
									<Trans>Type</Trans>
								</TableHead>
								<TableHead>
									<Trans>Status</Trans>
								</TableHead>
								<TableHead>
									<Trans>Response</Trans>
								</TableHead>
								<TableHead>
									<Trans>Uptime (24h)</Trans>
								</TableHead>
								<TableHead>
									<Trans>Tags</Trans>
								</TableHead>
								<TableHead className="text-right">
									<Trans>Actions</Trans>
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{filteredMonitors.map((monitor) => (
								<MonitorRow
									key={monitor.id}
									monitor={monitor}
									onEdit={setEditingMonitor}
									displayOptions={displayOptions}
								/>
							))}
						</TableBody>
					</Table>
				) : (
					<div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
						{filteredMonitors.map((monitor) => (
							<MonitorCard
								key={monitor.id}
								monitor={monitor}
								onEdit={setEditingMonitor}
								displayOptions={displayOptions}
							/>
						))}
					</div>
				)}
			</CardContent>

			{/* Add Monitor Dialog */}
			<AddMonitorDialog open={isAddDialogOpen} onOpenChange={setIsAddDialogOpen} />

			{/* Edit Monitor Dialog */}
			{editingMonitor && (
				<AddMonitorDialog
					open={!!editingMonitor}
					onOpenChange={(open) => !open && setEditingMonitor(null)}
					monitor={editingMonitor}
					isEdit
				/>
			)}
		</Card>
	)
})
