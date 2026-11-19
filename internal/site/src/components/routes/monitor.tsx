import { memo, useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import { useToast } from "@/components/ui/use-toast"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
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
import {
	Globe,
	Clock,
	Activity,
	RefreshCw,
	ExternalLink,
	Edit3,
	Trash2,
	CheckCircle2,
	XCircle,
	PauseIcon,
	PlayIcon,
	TrendingUp,
	TrendingDown,
	Plus,
	Zap,
	Gauge,
	Smartphone,
	Lock,
	Eye,
	LayoutDashboard,
	type LucideIcon,
} from "lucide-react"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import {
	type Heartbeat,
	type Monitor,
	getMonitor,
	getMonitorStats,
	getMonitorHeartbeats,
	manualCheck,
	pauseMonitor,
	resumeMonitor,
	deleteMonitor,
	getMonitorTypeLabel,
	getMonitorFaviconUrl,
	formatUptime,
	formatPing,
	runPageSpeedCheck,
} from "@/lib/monitors"
import { formatDate } from "@/lib/domains"
import {
	addMonitorToStatusPage,
	createStatusPage,
	getStatusPageMonitors,
	getStatusPages,
	getStatusPageUrl,
	removeMonitorFromStatusPage,
} from "@/lib/statuspages"
import { XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Line, LineChart } from "recharts"
import { Link, navigate } from "@/components/router"
import { AddMonitorDialog } from "@/components/monitors-table/add-monitor-dialog"
import { cn } from "@/lib/utils"
import { MonitorInfoSections } from "./monitor-info-sections"

type HeartbeatRow = Heartbeat

// Uptime Bar Component - Visual timeline of recent checks
function UptimeBarVisualization({ heartbeats }: { heartbeats?: HeartbeatRow[] }) {
	const recent = useMemo(() => {
		if (!heartbeats?.length) return []
		return heartbeats.slice(0, 30).reverse()
	}, [heartbeats])

	if (!recent.length) {
		return (
			<div className="flex gap-1 h-10 items-center">
				{Array.from({ length: 12 }).map((_, i) => (
					<div key={i} className="flex-1 h-8 rounded-full bg-muted/50" />
				))}
			</div>
		)
	}

	const getBarStyle = (status: string) => {
		switch (status) {
			case "up":
				return "bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.5)] dark:shadow-[0_0_12px_rgba(34,197,94,0.7)]"
			case "down":
				return "bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.5)] dark:shadow-[0_0_12px_rgba(239,68,68,0.7)]"
			case "paused":
				return "bg-gray-400"
			default:
				return "bg-yellow-500 shadow-[0_0_8px_rgba(234,179,8,0.5)] dark:shadow-[0_0_12px_rgba(234,179,8,0.7)]"
		}
	}

	return (
		<TooltipProvider>
			<div className="space-y-3">
				<div className="flex gap-1 h-10">
					{recent.map((hb, i) => (
						<Tooltip key={i}>
							<TooltipTrigger asChild>
								<div
									className={cn(
										"flex-1 rounded-full transition-all hover:scale-110 cursor-pointer min-w-[4px]",
										getBarStyle(hb.status)
									)}
								/>
							</TooltipTrigger>
							<TooltipContent side="bottom" className="text-xs">
								<p className="capitalize font-medium">{hb.status}</p>
								<p className="text-muted-foreground">{formatPing(hb.ping)}</p>
								<p className="text-muted-foreground">{formatDate(hb.time || "")}</p>
							</TooltipContent>
						</Tooltip>
					))}
				</div>
				<div className="flex justify-between text-xs text-muted-foreground">
					<span className="font-medium">{recent.length} recent checks</span>
					<div className="flex items-center gap-3">
						<span className="inline-flex items-center gap-1.5">
							<span className="w-2.5 h-2.5 rounded-full bg-green-500 shadow-[0_0_6px_rgba(34,197,94,0.4)]" />
							{recent.filter((h) => h.status === "up").length} up
						</span>
						<span className="inline-flex items-center gap-1.5">
							<span className="w-2.5 h-2.5 rounded-full bg-red-500 shadow-[0_0_6px_rgba(239,68,68,0.4)]" />
							{recent.filter((h) => h.status === "down").length} down
						</span>
					</div>
				</div>
			</div>
		</TooltipProvider>
	)
}

// Response time statistics component
function ResponseTimeStats({ heartbeats }: { heartbeats?: HeartbeatRow[] }) {
	const stats = useMemo(() => {
		if (!heartbeats?.length) return null
		const pings = heartbeats.filter((h) => h.ping && h.ping > 0).map((h) => h.ping)
		if (!pings.length) return null

		const sorted = [...pings].sort((a, b) => a - b)
		const avg = Math.round(pings.reduce((a, b) => a + b, 0) / pings.length)
		const min = sorted[0]
		const max = sorted[sorted.length - 1]
		const p95 = sorted[Math.floor(sorted.length * 0.95)]
		const p99 = sorted[Math.floor(sorted.length * 0.99)]

		return { avg, min, max, p95, p99, count: pings.length }
	}, [heartbeats])

	if (!stats) return null

	return (
		<div className="grid grid-cols-5 gap-2 text-center">
			<div className="p-2 bg-muted/50 rounded-lg">
				<div className="text-xs text-muted-foreground">Avg</div>
				<div className="text-lg font-semibold">{formatPing(stats.avg)}</div>
			</div>
			<div className="p-2 bg-muted/50 rounded-lg">
				<div className="text-xs text-muted-foreground">Min</div>
				<div className="text-lg font-semibold text-green-600">{formatPing(stats.min)}</div>
			</div>
			<div className="p-2 bg-muted/50 rounded-lg">
				<div className="text-xs text-muted-foreground">Max</div>
				<div className="text-lg font-semibold text-red-600">{formatPing(stats.max)}</div>
			</div>
			<div className="p-2 bg-muted/50 rounded-lg">
				<div className="text-xs text-muted-foreground">P95</div>
				<div className="text-lg font-semibold">{formatPing(stats.p95)}</div>
			</div>
			<div className="p-2 bg-muted/50 rounded-lg">
				<div className="text-xs text-muted-foreground">P99</div>
				<div className="text-lg font-semibold">{formatPing(stats.p99)}</div>
			</div>
		</div>
	)
}

function getVitalColor(status: string): string {
	switch (status) {
		case "good":
			return "text-green-500"
		case "needs-improvement":
			return "text-yellow-500"
		default:
			return "text-red-500"
	}
}

function getVitalBg(status: string): string {
	switch (status) {
		case "good":
			return "bg-green-500/10 border-green-500/20"
		case "needs-improvement":
			return "bg-yellow-500/10 border-yellow-500/20"
		default:
			return "bg-red-500/10 border-red-500/20"
	}
}

function ScoreRing({ score, label }: { score: number; label: string }) {
	const color = score >= 90 ? "text-green-500" : score >= 70 ? "text-yellow-500" : "text-red-500"
	const bg = score >= 90 ? "stroke-green-500" : score >= 70 ? "stroke-yellow-500" : "stroke-red-500"
	const circumference = 2 * Math.PI * 18
	const offset = circumference - (score / 100) * circumference
	return (
		<div className="flex flex-col items-center gap-1">
			<div className="relative w-12 h-12">
				<svg
					className="w-12 h-12 -rotate-90"
					viewBox="0 0 44 44"
					role="img"
					aria-label={`Score ${Math.round(score)} for ${label}`}
				>
					<title>
						Score {Math.round(score)} for {label}
					</title>
					<circle cx="22" cy="22" r="18" fill="none" stroke="currentColor" strokeWidth="4" className="text-muted/30" />
					<circle
						cx="22"
						cy="22"
						r="18"
						fill="none"
						strokeWidth="4"
						strokeLinecap="round"
						className={bg}
						strokeDasharray={circumference}
						strokeDashoffset={offset}
					/>
				</svg>
				<span className={cn("absolute inset-0 flex items-center justify-center text-xs font-bold", color)}>
					{Math.round(score)}
				</span>
			</div>
			<span className="text-[10px] text-muted-foreground uppercase tracking-wider">{label}</span>
		</div>
	)
}

function VitalCard({ label, value, status, detail }: { label: string; value: string; status: string; detail: string }) {
	return (
		<div className={cn("p-3 rounded-lg border", getVitalBg(status))}>
			<div className="flex items-center justify-between mb-1">
				<span className="text-xs text-muted-foreground">{label}</span>
				<div
					className={cn(
						"w-2 h-2 rounded-full",
						status === "good" ? "bg-green-500" : status === "needs-improvement" ? "bg-yellow-500" : "bg-red-500"
					)}
				/>
			</div>
			<div className={cn("text-lg font-bold", getVitalColor(status))}>{value}</div>
			<div className="text-[10px] text-muted-foreground">{detail}</div>
		</div>
	)
}

function formatMs(ms: number): string {
	if (ms < 1000) return `${Math.round(ms)}ms`
	return `${(ms / 1000).toFixed(1)}s`
}

function CoreWebVitalsCard({ monitorId, url }: { monitorId: string; url?: string }) {
	const [strategy, setStrategy] = useState<"mobile" | "desktop">("mobile")
	const [lastError, setLastError] = useState<string | null>(null)
	const { toast } = useToast()

	const {
		data,
		isPending: isPageSpeedLoading,
		mutate,
		error: mutationError,
	} = useMutation({
		mutationFn: () => runPageSpeedCheck(monitorId, strategy),
		onSuccess: () => {
			setLastError(null)
			toast({ title: "Lighthouse check complete" })
		},
		onError: (err: Error) => {
			const msg = err.message || "Unknown error"
			setLastError(msg)
			toast({ title: "Check failed", description: msg, variant: "destructive" })
		},
	})

	if (!url) return null

	const errorMessage = lastError || (mutationError ? (mutationError as Error).message : null)

	return (
		<Card>
			<CardHeader className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
				<div>
					<CardTitle className="flex items-center gap-2">
						<Zap className="h-5 w-5 text-yellow-500" />
						Core Web Vitals
					</CardTitle>
					<CardDescription>
						{data
							? `Checked ${new Date(data.checkedAt).toLocaleString()}`
							: "Run a Lighthouse check to get performance metrics"}
					</CardDescription>
				</div>
				<div className="flex items-center gap-2">
					<div className="flex rounded-lg border overflow-hidden">
						<button
							onClick={() => setStrategy("mobile")}
							className={cn(
								"px-3 py-1.5 text-xs font-medium transition-colors",
								strategy === "mobile" ? "bg-primary text-primary-foreground" : "bg-muted hover:bg-muted/80"
							)}
						>
							<Smartphone className="h-3 w-3 inline mr-1" />
							Mobile
						</button>
						<button
							onClick={() => setStrategy("desktop")}
							className={cn(
								"px-3 py-1.5 text-xs font-medium transition-colors",
								strategy === "desktop" ? "bg-primary text-primary-foreground" : "bg-muted hover:bg-muted/80"
							)}
						>
							<Gauge className="h-3 w-3 inline mr-1" />
							Desktop
						</button>
					</div>
					<Button size="sm" onClick={() => mutate()} disabled={isPageSpeedLoading}>
						<RefreshCw className={cn("mr-2 h-4 w-4", isPageSpeedLoading && "animate-spin")} />
						{isPageSpeedLoading ? "Running..." : "Run Check"}
					</Button>
				</div>
			</CardHeader>
			<CardContent>
				{errorMessage ? (
					<div className="flex flex-col items-center justify-center py-8 gap-3">
						<div className="p-3 bg-red-500/10 rounded-full">
							<XCircle className="h-6 w-6 text-red-500" />
						</div>
						<div className="text-center max-w-md">
							<p className="text-sm font-medium text-red-600 dark:text-red-400">Check failed</p>
							<p className="text-sm text-muted-foreground mt-1">
								{errorMessage.includes("429") || errorMessage.includes("quota")
									? "PageSpeed API quota exceeded. Please try again later."
									: errorMessage.includes("network")
										? "Network error. Please check your connection and try again."
										: "An error occurred while running the check."}
							</p>
						</div>
						<Button variant="outline" size="sm" onClick={() => mutate()} disabled={isPageSpeedLoading}>
							<RefreshCw className={cn("mr-2 h-4 w-4", isPageSpeedLoading && "animate-spin")} />
							Retry
						</Button>
					</div>
				) : data ? (
					<div className="space-y-4">
						{/* Lighthouse Scores */}
						<div className="flex items-center gap-4 justify-center sm:justify-start">
							<ScoreRing score={data.performance} label="Perf" />
							<ScoreRing score={data.accessibility} label="A11y" />
							<ScoreRing score={data.bestPractices} label="BP" />
							<ScoreRing score={data.seo} label="SEO" />
						</div>

						{/* Core Web Vitals */}
						<div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
							<VitalCard
								label="LCP"
								value={formatMs(data.lcp)}
								status={data.vitals.lcp || "poor"}
								detail="Largest Contentful Paint"
							/>
							<VitalCard
								label="FID"
								value={formatMs(data.tbt)}
								status={data.vitals.fid || "poor"}
								detail="Total Blocking Time (proxy)"
							/>
							<VitalCard
								label="CLS"
								value={data.cls.toFixed(3)}
								status={data.vitals.cls || "poor"}
								detail="Cumulative Layout Shift"
							/>
							<VitalCard
								label="FCP"
								value={formatMs(data.fcp)}
								status={data.vitals.fcp || "poor"}
								detail="First Contentful Paint"
							/>
							<VitalCard
								label="TTFB"
								value={formatMs(data.ttfb)}
								status={data.vitals.ttfb || "poor"}
								detail="Time to First Byte"
							/>
							<VitalCard
								label="TTI"
								value={formatMs(data.tti)}
								status={data.vitals.tti || "poor"}
								detail="Time to Interactive"
							/>
						</div>
					</div>
				) : (
					<div className="flex flex-col items-center justify-center py-8 gap-3 text-muted-foreground">
						<div className="p-3 bg-muted/50 rounded-full">
							<Gauge className="h-6 w-6 opacity-50" />
						</div>
						<p className="text-sm">No Lighthouse data yet. Click "Run Check" to analyze performance.</p>
						<p className="text-xs text-muted-foreground">Powered by Google PageSpeed Insights</p>
					</div>
				)}
			</CardContent>
		</Card>
	)
}

// Status badge component
function StatusBadge({ status }: { status: string }) {
	const configs = {
		up: { color: "bg-green-500", icon: CheckCircle2, text: "Up" },
		down: { color: "bg-red-500", icon: XCircle, text: "Down" },
		pending: { color: "bg-yellow-500", icon: Clock, text: "Pending" },
		paused: { color: "bg-gray-500", icon: PauseIcon, text: "Paused" },
		maintenance: { color: "bg-blue-500", icon: Activity, text: "Maintenance" },
	}

	const config = configs[status as keyof typeof configs] || configs.pending
	const Icon = config.icon

	return (
		<div className="flex items-center gap-2">
			<div className={`h-2.5 w-2.5 rounded-full ${config.color}`} />
			<Icon className="h-4 w-4 text-muted-foreground" />
			<span className="capitalize text-sm">{config.text}</span>
		</div>
	)
}

// Stat card component
function StatCard({
	title,
	value,
	icon: Icon,
	subtitle,
	trend,
	className,
}: {
	title: string
	value: string
	icon: LucideIcon
	subtitle?: string
	trend?: "up" | "down" | "neutral"
	className?: string
}) {
	const TrendIcon = trend === "up" ? TrendingUp : trend === "down" ? TrendingDown : null

	return (
		<Card className={className}>
			<CardContent className="p-4">
				<div className="flex items-start gap-3">
					<div className="p-2 bg-muted rounded-lg">
						<Icon className="h-4 w-4 text-muted-foreground" />
					</div>
					<div className="flex-1 min-w-0">
						<p className="text-sm text-muted-foreground">{title}</p>
						<div className="flex items-center gap-2">
							<p className="font-semibold truncate">{value}</p>
							{TrendIcon && <TrendIcon className="h-4 w-4 text-muted-foreground" />}
						</div>
						{subtitle && <p className="text-xs text-muted-foreground">{subtitle}</p>}
					</div>
				</div>
			</CardContent>
		</Card>
	)
}

function MonitorFaviconImage({ monitor, iconColor }: { monitor: Monitor; iconColor: string }) {
	const [error, setError] = useState(false)
	const faviconUrl = getMonitorFaviconUrl(monitor)
	if (!faviconUrl || error) {
		return <Globe className={cn("h-6 w-6", iconColor)} />
	}
	return <img src={faviconUrl} alt="" className="h-6 w-6 object-contain" onError={() => setError(true)} />
}

export default memo(function MonitorDetail({ id }: { id: string }) {
	const { toast } = useToast()
	const queryClient = useQueryClient()
	const [isEditDialogOpen, setIsEditDialogOpen] = useState(false)
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
	const [timeRange, setTimeRange] = useState<"24h" | "7d" | "30d">("24h")

	const { data: monitor, isLoading: isMonitorLoading } = useQuery({
		queryKey: ["monitor", id],
		queryFn: () => getMonitor(id),
		staleTime: Infinity,
		refetchInterval: 30000,
	})

	const { data: stats } = useQuery({
		queryKey: ["monitor-stats", id],
		queryFn: () => getMonitorStats(id),
		refetchInterval: 30000,
	})

	const { data: heartbeatsData } = useQuery({
		queryKey: ["monitor-heartbeats", id],
		queryFn: () => getMonitorHeartbeats(id),
		refetchInterval: 30000,
	})
	const heartbeats = heartbeatsData?.heartbeats

	const checkMutation = useMutation({
		mutationFn: () => manualCheck(id),
		onSuccess: (result) => {
			toast({
				title: `Check complete`,
				description: `${monitor?.name} is ${result.status}`,
			})
			queryClient.invalidateQueries({ queryKey: ["monitor", id] })
			queryClient.invalidateQueries({ queryKey: ["monitor-heartbeats", id] })
			queryClient.invalidateQueries({ queryKey: ["monitor-stats", id] })
		},
	})

	const pauseMutation = useMutation({
		mutationFn: () => (monitor?.status === "paused" ? resumeMonitor(id) : pauseMonitor(id)),
		onSuccess: () => {
			toast({
				title: monitor?.status === "paused" ? "Monitor resumed" : "Monitor paused",
			})
			queryClient.invalidateQueries({ queryKey: ["monitor", id] })
		},
	})

	const deleteMutation = useMutation({
		mutationFn: () => deleteMonitor(id),
		onSuccess: () => {
			toast({ title: "Monitor deleted" })
			navigate("/")
		},
	})

	const [isCreateStatusPageOpen, setIsCreateStatusPageOpen] = useState(false)
	const [statusPageName, setStatusPageName] = useState("")
	const [statusPageSlug, setStatusPageSlug] = useState("")
	const [statusPagePublic, setStatusPagePublic] = useState(true)

	const { data: statusPages } = useQuery({
		queryKey: ["status-pages"],
		queryFn: () => getStatusPages(),
	})

	const { data: linkedStatusPageMonitors } = useQuery({
		queryKey: ["monitor-status-page-links", id, statusPages?.map((page) => page.id).join(",")],
		queryFn: async () => {
			if (!statusPages?.length) return []
			const links = await Promise.all(
				statusPages.map(async (page) =>
					(await getStatusPageMonitors(page.id)).map((link) => ({ ...link, status_page_id: page.id }))
				)
			)
			return links.flat().filter((link) => link.monitor_id === id)
		},
		enabled: Boolean(statusPages?.length),
	})

	const updateStatusPagesMutation = useMutation({
		mutationFn: async ({ pageId, linked }: { pageId: string; linked: boolean }) => {
			if (linked) {
				await removeMonitorFromStatusPage(pageId, id)
			} else {
				await addMonitorToStatusPage(pageId, { monitor: id })
			}
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["monitor-status-page-links", id] })
			queryClient.invalidateQueries({ queryKey: ["status-pages"] })
			toast({ title: "Status pages updated" })
		},
	})

	const createStatusPageMutation = useMutation({
		mutationFn: async () => {
			const page = await createStatusPage({
				name: statusPageName || `${monitor?.name} Status`,
				slug: statusPageSlug || monitor?.name?.toLowerCase().replace(/\s+/g, "-") || "status",
				title: statusPageName || `${monitor?.name} Status Page`,
				public: statusPagePublic,
			})
			// Auto-link this monitor to the newly created status page
			await addMonitorToStatusPage(page.id, { monitor: id })
			return page
		},
		onSuccess: (page) => {
			queryClient.invalidateQueries({ queryKey: ["status-pages"] })
			queryClient.invalidateQueries({ queryKey: ["monitor-status-page-links", id] })
			toast({ title: "Status page created and monitor linked" })
			setIsCreateStatusPageOpen(false)
			setStatusPageName("")
			setStatusPageSlug("")
			setStatusPagePublic(true)
			// Open private pages in new tab since user is authenticated
			if (!page.public) {
				window.open(getStatusPageUrl(page.slug), "_blank", "noopener,noreferrer")
			}
		},
	})

	const handleDelete = () => {
		setIsDeleteDialogOpen(true)
	}

	// Filter heartbeats by time range
	const filteredHeartbeats = useMemo(() => {
		if (!heartbeats) return []
		const now = Date.now()
		const ranges: Record<string, number> = {
			"24h": 24 * 60 * 60 * 1000,
			"7d": 7 * 24 * 60 * 60 * 1000,
			"30d": 30 * 24 * 60 * 60 * 1000,
		}
		const cutoff = now - (ranges[timeRange] || ranges["24h"])
		const filtered = heartbeats.filter((h: HeartbeatRow) => {
			const t = new Date(h.time || "").getTime()
			return t >= cutoff
		})
		return filtered.length > 0 ? filtered : heartbeats
	}, [heartbeats, timeRange])

	// Prepare chart data from heartbeats
	const chartData = useMemo(() => {
		if (!filteredHeartbeats.length) return []
		return filteredHeartbeats
			.slice()
			.reverse()
			.map((h: HeartbeatRow) => ({
				time: new Date(h.time || "").toLocaleTimeString(),
				responseTime: h.ping || 0,
				status: h.status === "up" ? 1 : 0,
			}))
	}, [filteredHeartbeats])

	// Calculate stats
	const uptimeStats = useMemo(() => {
		if (!heartbeats || !Array.isArray(heartbeats) || heartbeats.length === 0) return null
		const total = heartbeats.length
		const up = heartbeats.filter((h: HeartbeatRow) => h.status === "up").length
		const avgResponse = heartbeats.reduce((sum: number, h: HeartbeatRow) => sum + (h.ping || 0), 0) / total
		return {
			uptime: ((up / total) * 100).toFixed(2),
			avgResponse: avgResponse.toFixed(0),
			totalChecks: total,
		}
	}, [heartbeats])

	if (isMonitorLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<RefreshCw className="h-8 w-8 animate-spin text-muted-foreground" />
			</div>
		)
	}

	if (!monitor) {
		return (
			<div className="text-center py-12">
				<Globe className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
				<h2 className="text-xl font-semibold mb-2">Monitor not found</h2>
				<p className="text-muted-foreground">The monitor you are looking for does not exist.</p>
				<Button asChild className="mt-4">
					<Link href="/">Go back home</Link>
				</Button>
			</div>
		)
	}

	const isUp = monitor.status === "up"
	const isPaused = monitor.status === "paused"
	const isPending = monitor.status === "pending"

	const headerIconColor = isUp
		? "text-green-500"
		: isPaused
			? "text-gray-500"
			: isPending
				? "text-yellow-500"
				: "text-red-500"
	const headerBgColor = isUp
		? "bg-green-500/10"
		: isPaused
			? "bg-gray-500/10"
			: isPending
				? "bg-yellow-500/10"
				: "bg-red-500/10"

	return (
		<div className="grid gap-4 mb-14">
			{/* Header */}
			<Card>
				<CardContent className="p-6">
					<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
						<div className="flex items-center gap-4">
							<div className={cn("h-12 w-12 rounded-full flex items-center justify-center", headerBgColor)}>
								<MonitorFaviconImage monitor={monitor} iconColor={headerIconColor} />
							</div>
							<div>
								<h1 className="text-2xl font-bold">{monitor.name}</h1>
								<div className="flex items-center gap-2 mt-1 flex-wrap">
									<StatusBadge status={monitor.status} />
									<Badge variant="secondary">{getMonitorTypeLabel(monitor.type)}</Badge>
									{monitor.interval && <Badge variant="outline">{monitor.interval}s interval</Badge>}
									{isPending && (
										<Badge variant="outline" className="text-yellow-600 border-yellow-500/30">
											Waiting for first check
										</Badge>
									)}
								</div>
								{monitor.url && <p className="text-sm text-muted-foreground mt-1">{monitor.url}</p>}
							</div>
						</div>
						<div className="flex items-center gap-2 flex-wrap">
							<Button
								variant={isPending ? "default" : "outline"}
								size="sm"
								onClick={() => checkMutation.mutate()}
								disabled={checkMutation.isPending || isPaused}
							>
								<RefreshCw className={cn("mr-2 h-4 w-4", checkMutation.isPending && "animate-spin")} />
								{isPending ? "Run First Check" : "Check Now"}
							</Button>
							{monitor.url && (
								<Button variant="outline" size="sm" asChild>
									<a href={monitor.url} target="_blank" rel="noopener noreferrer">
										<ExternalLink className="mr-2 h-4 w-4" />
										<Trans>Visit</Trans>
									</a>
								</Button>
							)}
							<Button
								variant="outline"
								size="sm"
								onClick={() => pauseMutation.mutate()}
								disabled={pauseMutation.isPending}
							>
								{monitor.status === "paused" ? (
									<>
										<PlayIcon className="mr-2 h-4 w-4" />
										<Trans>Resume</Trans>
									</>
								) : (
									<>
										<PauseIcon className="mr-2 h-4 w-4" />
										<Trans>Pause</Trans>
									</>
								)}
							</Button>
							<Button variant="outline" size="sm" onClick={() => setIsEditDialogOpen(true)}>
								<Edit3 className="mr-2 h-4 w-4" />
								<Trans>Edit</Trans>
							</Button>
							<Button variant="destructive" size="sm" onClick={handleDelete}>
								<Trash2 className="mr-2 h-4 w-4" />
								<Trans>Delete</Trans>
							</Button>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Summary Bar */}
			<div className="grid sm:grid-cols-4 gap-4">
				<StatCard title="Uptime (24h)" value={formatUptime(stats?.uptime_percent_24h ?? 0)} icon={Activity} />
				<StatCard title="Uptime (7d)" value={formatUptime(stats?.uptime_percent_7d ?? 0)} icon={Activity} />
				<StatCard title="Uptime (30d)" value={formatUptime(stats?.uptime_percent_30d ?? 0)} icon={Activity} />
				<StatCard
					title="Avg Response"
					value={
						stats?.avg_ping_24h
							? `${Math.round(stats.avg_ping_24h)}ms`
							: uptimeStats
								? `${uptimeStats.avgResponse}ms`
								: "-"
					}
					subtitle={`${stats?.uptime_24h?.total ?? uptimeStats?.totalChecks ?? 0} checks`}
					icon={Clock}
				/>
			</div>

			{/* Uptime Bar & Response Stats */}
			<div className="grid sm:grid-cols-2 gap-4">
				<Card>
					<CardHeader>
						<CardTitle>Recent Uptime</CardTitle>
						<CardDescription>Visual timeline of the last 30 checks</CardDescription>
					</CardHeader>
					<CardContent>
						<UptimeBarVisualization heartbeats={heartbeats} />
					</CardContent>
				</Card>
				<Card>
					<CardHeader>
						<CardTitle>Response Time Statistics</CardTitle>
						<CardDescription>Distribution of response times</CardDescription>
					</CardHeader>
					<CardContent>
						<ResponseTimeStats heartbeats={heartbeats} />
					</CardContent>
				</Card>
			</div>

			{/* Core Web Vitals */}
			<CoreWebVitalsCard monitorId={id} url={monitor.url} />

			{/* Domain Info Sections */}
			<MonitorInfoSections monitor={monitor} heartbeats={heartbeats} />

			{/* Response Time Chart */}
			<Card>
				<CardHeader className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
					<div>
						<CardTitle>Response Time</CardTitle>
						<CardDescription>
							<Trans>Response time over the selected period</Trans>
						</CardDescription>
					</div>
					<div className="flex items-center gap-2">
						{(["24h", "7d", "30d"] as const).map((range) => (
							<Button
								key={range}
								variant={timeRange === range ? "default" : "outline"}
								size="sm"
								onClick={() => setTimeRange(range)}
							>
								{range === "24h" ? "24h" : range === "7d" ? "7d" : "30d"}
							</Button>
						))}
					</div>
				</CardHeader>
				<CardContent>
					<div className="h-[300px]">
							{chartData.length > 0 ? (
								<ResponsiveContainer width="100%" height="100%">
									<LineChart data={chartData}>
										<defs>
											<linearGradient id="colorResponse" x1="0" y1="0" x2="0" y2="1">
												<stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3} />
												<stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
											</linearGradient>
										</defs>
										<CartesianGrid vertical={false} strokeDasharray="3 3" opacity={0.3} />
										<XAxis dataKey="time" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
										<YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} unit="ms" />
										<Tooltip
											contentStyle={{
												backgroundColor: "hsl(var(--card))",
												border: "1px solid hsl(var(--border))",
												borderRadius: "8px",
											}}
											formatter={(value: number) => [`${value}ms`, "Response Time"]}
										/>
										<Line
											type="monotone"
											dataKey="responseTime"
											stroke="#3b82f6"
											strokeWidth={2}
											fillOpacity={1}
											fill="url(#colorResponse)"
											name="Response Time"
											isAnimationActive={false}
											dot={false}
											activeDot={{ r: 4, fill: "#3b82f6", stroke: "#fff", strokeWidth: 2 }}
										/>
									</LineChart>
								</ResponsiveContainer>
							) : (
								<div className="h-full flex flex-col items-center justify-center gap-3 text-muted-foreground">
									<div className="p-3 bg-muted/50 rounded-full">
										<Activity className="h-6 w-6 opacity-50" />
									</div>
									<p className="text-sm font-medium">
										{isPending
											? "No check data yet. Run a check to see the chart."
											: "No data available for selected time range"}
									</p>
									{isPending && (
										<Button
											variant="outline"
											size="sm"
											onClick={() => checkMutation.mutate()}
											disabled={checkMutation.isPending}
										>
												<RefreshCw className={cn("mr-2 h-4 w-4", checkMutation.isPending && "animate-spin")} />
												Run First Check
											</Button>
										)}
									</div>
							)}
					</div>
				</CardContent>
			</Card>

			<div className="grid sm:grid-cols-2 gap-4">
				<Card>
					<CardHeader>
						<CardTitle>Monitor Details</CardTitle>
					</CardHeader>
					<CardContent className="space-y-2">
						<div className="flex justify-between">
							<span className="text-muted-foreground">Type</span>
							<span className="font-medium">{getMonitorTypeLabel(monitor.type)}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Interval</span>
							<span className="font-medium">{monitor.interval}s</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Retries</span>
							<span className="font-medium">{monitor.retries}</span>
						</div>
						<div className="flex justify-between">
							<span className="text-muted-foreground">Created</span>
							<span className="font-medium">{formatDate(monitor.created)}</span>
						</div>
						{monitor.last_check && (
							<div className="flex justify-between">
								<span className="text-muted-foreground">Last Check</span>
								<span className="font-medium">{formatDate(monitor.last_check)}</span>
							</div>
						)}
					</CardContent>
				</Card>

				<Card>
					<CardHeader>
						<div className="flex items-center justify-between">
							<div>
								<CardTitle>Status Page</CardTitle>
								<CardDescription>Create or link to status pages</CardDescription>
							</div>
							<Button variant="outline" size="sm" onClick={() => setIsCreateStatusPageOpen(true)}>
								<Plus className="mr-2 h-4 w-4" />
								Create New
							</Button>
						</div>
					</CardHeader>
					<CardContent className="space-y-3">
						{statusPages && statusPages.length > 0 ? (
							<div className="space-y-2">
								{statusPages.map((page) => {
									const isLinked = linkedStatusPageMonitors?.some((link) => link.status_page_id === page.id) || false
									const linkInfo = linkedStatusPageMonitors?.find((link) => link.status_page_id === page.id)

									return (
										<div
											key={page.id}
											className={cn(
												"flex items-center justify-between p-3 rounded-lg border transition-colors",
												isLinked ? "bg-primary/5 border-primary/20" : "bg-muted/30 border-border"
											)}
										>
											<div className="min-w-0 flex-1">
												<div className="flex items-center gap-2">
													<LayoutDashboard className="h-3.5 w-3.5 text-muted-foreground" />
													<span className="font-medium text-sm truncate">{page.name}</span>
													{page.public ? (
														<Badge variant="outline" className="text-[10px] gap-1">
															<Eye className="h-2.5 w-2.5" />
															Public
														</Badge>
													) : (
														<Badge variant="outline" className="text-[10px] gap-1">
															<Lock className="h-2.5 w-2.5" />
															Private
														</Badge>
													)}
												</div>
												{isLinked && linkInfo && (
													<p className="text-xs text-muted-foreground mt-1">
														Display: {linkInfo.display_name || monitor?.name}
														{linkInfo.group && ` • Group: ${linkInfo.group}`}
													</p>
												)}
												{!isLinked && (
													<p className="text-xs text-muted-foreground mt-1">
														{page.monitor_count} monitor{page.monitor_count !== 1 ? "s" : ""} linked
													</p>
												)}
											</div>
											<div className="flex items-center gap-2 ml-2">
												<Button
													variant="ghost"
													size="icon"
													className="h-8 w-8"
													asChild
													title={page.public ? "View public status page" : "View private status page"}
												>
													<a href={getStatusPageUrl(page.slug)} target="_blank" rel="noopener noreferrer">
														<ExternalLink className="h-4 w-4" />
													</a>
												</Button>
												<Button
													variant={isLinked ? "default" : "outline"}
													size="sm"
													onClick={() => {
														updateStatusPagesMutation.mutate({
															pageId: page.id,
															linked: isLinked,
														})
													}}
													disabled={updateStatusPagesMutation.isPending}
												>
													{isLinked ? (
														<>
															<CheckCircle2 className="mr-1 h-3 w-3" />
															Linked
														</>
													) : (
														"Link"
													)}
												</Button>
											</div>
										</div>
									)
								})}
							</div>
						) : (
							<div className="text-center py-4">
								<p className="text-sm text-muted-foreground">No status pages yet.</p>
								<p className="text-xs text-muted-foreground mt-1">
									Create one to share your service status or keep it private for internal use.
								</p>
							</div>
						)}
					</CardContent>
				</Card>
			</div>

			{/* Create Status Page Dialog */}
			{isCreateStatusPageOpen && (
				<AlertDialog open={isCreateStatusPageOpen} onOpenChange={setIsCreateStatusPageOpen}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>Create Status Page</AlertDialogTitle>
							<AlertDialogDescription>Create a status page and optionally link this monitor.</AlertDialogDescription>
						</AlertDialogHeader>
						<div className="grid gap-4 py-4">
							<div className="grid gap-2">
								<Label htmlFor="sp-name">Name</Label>
								<Input
									id="sp-name"
									value={statusPageName}
									onChange={(e) => setStatusPageName(e.target.value)}
									placeholder={`${monitor.name} Status`}
								/>
							</div>
							<div className="grid gap-2">
								<Label htmlFor="sp-slug">Slug</Label>
								<Input
									id="sp-slug"
									value={statusPageSlug}
									onChange={(e) => setStatusPageSlug(e.target.value)}
									placeholder={monitor.name?.toLowerCase().replace(/\s+/g, "-")}
								/>
							</div>
							<div className="flex items-center justify-between rounded-lg border p-3">
								<div className="space-y-0.5">
									<Label htmlFor="sp-public" className="text-sm font-medium">
										Public Status Page
									</Label>
									<p className="text-xs text-muted-foreground">Anyone can view this page without authentication.</p>
								</div>
								<Switch id="sp-public" checked={statusPagePublic} onCheckedChange={setStatusPagePublic} />
							</div>
						</div>
						<AlertDialogFooter>
							<AlertDialogCancel onClick={() => setIsCreateStatusPageOpen(false)}>Cancel</AlertDialogCancel>
							<AlertDialogAction
								onClick={() => createStatusPageMutation.mutate()}
								disabled={createStatusPageMutation.isPending}
							>
								Create
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			)}
			<AddMonitorDialog open={isEditDialogOpen} onOpenChange={setIsEditDialogOpen} monitor={monitor} isEdit />

			<AlertDialog open={isDeleteDialogOpen} onOpenChange={setIsDeleteDialogOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Delete Monitor</AlertDialogTitle>
						<AlertDialogDescription>
							Are you sure you want to delete this monitor? This action cannot be undone.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => {
								deleteMutation.mutate()
								setIsDeleteDialogOpen(false)
							}}
							className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
						>
							Delete
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	)
})
