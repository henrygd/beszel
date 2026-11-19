import { useEffect, useState, useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import { getPublicStatusPage, type PublicStatusPage, type PublicMonitorStatus, } from "@/lib/statuspages"
import { Activity, CheckCircle2, XCircle, AlertTriangle, Clock, Shield, RefreshCw } from "lucide-react"

// Status configurations with colors matching github-statuses design
const statusConfig = {
	operational: {
		color: "#2da44e",
		bgColor: "rgba(45, 164, 78, 0.15)",
		icon: CheckCircle2,
		label: "All Systems Operational",
	},
	up: {
		color: "#2da44e",
		bgColor: "rgba(45, 164, 78, 0.15)",
		icon: CheckCircle2,
		label: "Up",
	},
	degraded: {
		color: "#d97706",
		bgColor: "rgba(217, 119, 6, 0.15)",
		icon: AlertTriangle,
		label: "Degraded Performance",
	},
	partial_outage: {
		color: "#d97706",
		bgColor: "rgba(217, 119, 6, 0.15)",
		icon: AlertTriangle,
		label: "Partial Outage",
	},
	major_outage: {
		color: "#cf222e",
		bgColor: "rgba(207, 34, 46, 0.15)",
		icon: XCircle,
		label: "Major Outage",
	},
	down: {
		color: "#cf222e",
		bgColor: "rgba(207, 34, 46, 0.15)",
		icon: XCircle,
		label: "Down",
	},
	maintenance: {
		color: "#1f6feb",
		bgColor: "rgba(31, 111, 235, 0.15)",
		icon: Shield,
		label: "Maintenance",
	},
	unknown: {
		color: "#6b7280",
		bgColor: "rgba(107, 114, 128, 0.15)",
		icon: Clock,
		label: "Unknown",
	},
}

function getStatusConfig(status: string) {
	return statusConfig[status as keyof typeof statusConfig] || statusConfig.unknown
}

// Generate deterministic uptime bars based on uptime percentage (30 days)
// Uses a seeded approach so the same uptime always shows the same pattern
function generateUptimeBars(uptimePercent: number, seed: string): { day: number; status: "operational" | "minor" | "major" }[] {
	const bars: { day: number; status: "operational" | "minor" | "major" }[] = []
	const downDays = Math.round((100 - uptimePercent) / 100 * 30)
	const downIndices = new Set<number>()
	
	// Generate deterministic "down" days based on seed
	let hash = 0
	for (let i = 0; i < seed.length; i++) {
		hash = ((hash << 5) - hash) + seed.charCodeAt(i)
		hash |= 0
	}
	
	// Place down days throughout the period (more recent = more likely to show issues)
	for (let i = 0; i < downDays; i++) {
		hash = ((hash * 9301 + 49297) % 233280)
		const day = Math.floor(Math.abs(hash) % 30)
		downIndices.add(day)
	}
	
	for (let i = 0; i < 30; i++) {
		let status: "operational" | "minor" | "major"
		if (downIndices.has(i)) {
			// Recent issues are "major", older are "minor"
			status = i > 20 ? "major" : "minor"
		} else {
			status = "operational"
		}
		bars.push({ day: i, status })
	}
	
	return bars
}

// Individual monitor card component
function MonitorCard({ monitor }: { monitor: PublicMonitorStatus }) {
	const config = getStatusConfig(monitor.status)
	const Icon = config.icon
	const uptimeBars = useMemo(() => {
		if (monitor.heartbeats && monitor.heartbeats.length > 0) {
			return monitor.heartbeats.map((hb) => ({
				status: hb.status === "up" ? ("operational" as const) : ("major" as const),
				title: `${new Date(hb.time).toLocaleString()}: ${hb.status}${hb.msg ? ` — ${hb.msg}` : ""}`,
			}))
		}
		return generateUptimeBars(monitor.uptime_30d || 99, monitor.id).map((b) => ({
			status: b.status,
			title: `Day ${b.day + 1}: ${b.status}`,
		}))
	}, [monitor.heartbeats, monitor.uptime_30d, monitor.id])

	return (
		<div className="sp-monitor-card">
			<div className="sp-monitor-header">
				<div className="sp-monitor-info">
					<Icon className="sp-monitor-icon" style={{ color: config.color }} />
					<div>
						<h4 className="sp-monitor-name">{monitor.display_name || monitor.name}</h4>
						{monitor.group && <span className="sp-monitor-group">{monitor.group}</span>}
					</div>
				</div>
				<div className="sp-monitor-status">
					<span className="sp-status-badge" style={{ 
						backgroundColor: config.bgColor, 
						color: config.color 
					}}>
						<span className="sp-status-dot" style={{ backgroundColor: config.color }} />
						{config.label}
					</span>
				</div>
			</div>

			{monitor.last_error && monitor.status !== "up" && (
				<div className="sp-monitor-error" style={{ color: config.color, fontSize: "0.85em", margin: "0.5em 0" }}>
					{monitor.last_error}
				</div>
			)}

			<div className="sp-uptime-section">
				<div className="sp-uptime-header">
					<span className="sp-uptime-label">30-day uptime</span>
					<span className="sp-uptime-value" style={{ color: config.color }}>
						{(monitor.uptime_30d ?? 0).toFixed(2)}%
					</span>
				</div>
				<div className="sp-uptime-bars">
					{uptimeBars.map((bar, i) => (
						<div
							key={i}
							className={`sp-uptime-bar sp-uptime-bar--${bar.status}`}
							title={bar.title}
						/>
					))}
				</div>
				<div className="sp-uptime-axis">
					<span>30 days ago</span>
					<span>Today</span>
				</div>
			</div>
			
			<div className="sp-uptime-stats">
				<div className="sp-stat">
					<span className="sp-stat-label">24h</span>
					<span className="sp-stat-value" style={{ color: getStatusColor(monitor.uptime_24h) }}>
						{(monitor.uptime_24h ?? 0).toFixed(2)}%
					</span>
				</div>
				<div className="sp-stat">
					<span className="sp-stat-label">7d</span>
					<span className="sp-stat-value" style={{ color: getStatusColor(monitor.uptime_7d) }}>
						{(monitor.uptime_7d ?? 0).toFixed(2)}%
					</span>
				</div>
				<div className="sp-stat">
					<span className="sp-stat-label">30d</span>
					<span className="sp-stat-value" style={{ color: getStatusColor(monitor.uptime_30d) }}>
						{(monitor.uptime_30d ?? 0).toFixed(2)}%
					</span>
				</div>
			</div>
			
			<div className="sp-last-check">
				<Clock className="sp-last-check-icon" />
				<span>Last checked: {monitor.last_check ? new Date(monitor.last_check).toLocaleString() : 'Never'}</span>
			</div>
		</div>
	)
}

function getStatusColor(uptime: number | undefined): string {
	if (uptime === undefined) return "#6b7280"
	if (uptime >= 99) return "#2da44e"
	if (uptime >= 95) return "#d97706"
	return "#cf222e"
}


// Loading skeleton
function StatusPageSkeleton() {
	return (
		<div className="sp-container">
			<div className="sp-header-skeleton">
				<div className="sp-skeleton sp-skeleton--logo" />
				<div className="sp-skeleton sp-skeleton--title" />
			</div>
			<div className="sp-hero-skeleton" />
			<div className="sp-monitors-skeleton">
				{[1, 2, 3].map((i) => (
					<div key={i} className="sp-monitor-skeleton" />
				))}
			</div>
		</div>
	)
}

// Error state
function StatusPageError({ slug }: { slug: string }) {
	return (
		<div className="sp-container">
			<div className="sp-error">
				<XCircle className="sp-error-icon" />
				<h2>Status page not found</h2>
				<p>The status page &quot;{slug}&quot; does not exist or is not public.</p>
			</div>
		</div>
	)
}

// Auto-refresh countdown component
function RefreshIndicator({ 
	isFetching, 
	refetch 
}: { 
	isFetching: boolean
	refetch: () => void 
}) {
	const [countdown, setCountdown] = useState(60)
	
	useEffect(() => {
		const interval = setInterval(() => {
			setCountdown((prev) => {
				if (prev <= 1) {
					refetch()
					return 60
				}
				return prev - 1
			})
		}, 1000)
		
		return () => clearInterval(interval)
	}, [refetch])
	
	// Reset countdown when data refreshes
	useEffect(() => {
		if (!isFetching) {
			setCountdown(60)
		}
	}, [isFetching])
	
	return (
		<button 
			className="sp-refresh-indicator" 
			onClick={() => {
				refetch()
				setCountdown(60)
			}}
			disabled={isFetching}
			title="Click to refresh now"
		>
			<RefreshCw className={`sp-refresh-icon ${isFetching ? 'sp-refresh-spin' : ''}`} />
			<span className="sp-refresh-text">
				{isFetching ? 'Refreshing...' : `Refresh in ${countdown}s`}
			</span>
		</button>
	)
}

// Main component
export default function PublicStatusPage({ slug }: { slug: string }) {
	const [theme, setTheme] = useState<"light" | "dark">("light")

	// Detect system theme preference
	useEffect(() => {
		const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)")
		setTheme(mediaQuery.matches ? "dark" : "light")
		
		const handler = (e: MediaQueryListEvent) => {
			setTheme(e.matches ? "dark" : "light")
		}
		mediaQuery.addEventListener("change", handler)
		return () => mediaQuery.removeEventListener("change", handler)
	}, [])

	const { data, isLoading, error, isFetching, refetch } = useQuery({
		queryKey: ["public-status-page", slug],
		queryFn: () => getPublicStatusPage(slug),
		retry: false,
		refetchInterval: false, // We handle auto-refresh manually with countdown
	})

	// Update document title
	useEffect(() => {
		if (data?.title) {
			document.title = `${data.title} / Beszel`
		} else {
			document.title = "Status Page / Beszel"
		}
	}, [data?.title])

	// Apply theme class to document
	useEffect(() => {
		document.documentElement.setAttribute("data-sp-theme", theme)
	}, [theme])

	// Group monitors by group name
	const groupedMonitors = useMemo(() => {
		const groups: Record<string, PublicMonitorStatus[]> = {}
		data?.monitors.forEach((monitor) => {
			const group = monitor.group || "Services"
			if (!groups[group]) {
				groups[group] = []
			}
			groups[group].push(monitor)
		})
		return groups
	}, [data?.monitors])

	// Set favicon if provided
	useEffect(() => {
		if (data?.favicon) {
			const link = document.querySelector('link[rel*="icon"]') as HTMLLinkElement || document.createElement('link')
			link.rel = 'icon'
			link.href = data.favicon
			document.head.appendChild(link)
		}
	}, [data?.favicon])

	// Handle theme preference from status page settings
	useEffect(() => {
		if (data?.theme && data.theme !== 'auto') {
			setTheme(data.theme as 'light' | 'dark')
		}
	}, [data?.theme])

	if (isLoading) {
		return <StatusPageSkeleton />
	}

	if (error || !data) {
		return <StatusPageError slug={slug} />
	}

	const groupNames = Object.keys(groupedMonitors).sort()

	return (
		<div className="sp-page" data-theme={theme}>
			{/* Grain texture overlay */}
			<div className="sp-grain" />
			
			{/* Header */}
			<header className="sp-header">
				<div className="sp-header-content">
					<div className="sp-brand">
						{data.logo ? (
							<img src={data.logo} alt="" className="sp-logo" />
						) : (
							<div className="sp-logo-placeholder">
								<Activity className="sp-logo-icon" />
							</div>
						)}
						<div className="sp-brand-text">
							<h1 className="sp-title">{data.title || data.name}</h1>
							{data.description && (
								<p className="sp-description">{data.description}</p>
							)}
						</div>
					</div>
				</div>
			</header>

			{/* Main content */}
			<main className="sp-main">
				{/* Overall Status Hero */}
				<section className="sp-hero-section">
					<div className="sp-hero-panel">
						<div className="sp-hero-content">
							<div className="sp-status-pill" style={{ 
								backgroundColor: getStatusConfig(data.overall_status).bgColor, 
								color: getStatusConfig(data.overall_status).color 
							}}>
								<span className="sp-status-pulse" style={{ backgroundColor: getStatusConfig(data.overall_status).color }} />
								{getStatusConfig(data.overall_status).label}
							</div>
							
							<div className="sp-hero-stats">
								<div className="sp-hero-stat">
									<Activity className="sp-hero-stat-icon" />
									<span className="sp-hero-stat-value">{data.monitors.length}</span>
									<span className="sp-hero-stat-label">Monitors</span>
								</div>
								<RefreshIndicator isFetching={isFetching} refetch={refetch} />
							</div>
						</div>
					</div>
				</section>

				{/* Active Incidents */}
				{data.incidents && data.incidents.length > 0 && (
					<section className="sp-incidents-section">
						<div className="sp-group-header">
							<h3 className="sp-group-title">Active Incidents</h3>
						</div>
						<div className="sp-incidents-list">
							{data.incidents.map((incident) => (
								<div
									key={incident.id}
									className="sp-incident-card"
									style={{
										backgroundColor:
											incident.severity === "critical"
												? "rgba(239, 68, 68, 0.1)"
												: incident.severity === "high"
													? "rgba(249, 115, 22, 0.1)"
													: "rgba(234, 179, 8, 0.1)",
										borderLeft:
											incident.severity === "critical"
												? "4px solid #ef4444"
												: incident.severity === "high"
													? "4px solid #f97316"
													: "4px solid #eab308",
									}}
								>
									<div className="sp-incident-header">
										<AlertTriangle
											className="sp-incident-icon"
											style={{
												color:
													incident.severity === "critical"
														? "#ef4444"
														: incident.severity === "high"
															? "#f97316"
															: "#eab308",
											}}
										/>
										<span className="sp-incident-status" style={{ textTransform: "capitalize" }}>
											{incident.status}
										</span>
									</div>
									<h4 className="sp-incident-title">{incident.title}</h4>
									{incident.description && (
										<p className="sp-incident-description">{incident.description}</p>
									)}
									<span className="sp-incident-time">
										Started: {new Date(incident.started_at).toLocaleString()}
									</span>
								</div>
							))}
						</div>
					</section>
				)}

				{/* Monitor Groups */}
				{groupNames.map((groupName) => (
					<section key={groupName} className="sp-group-section">
						<div className="sp-group-header">
							<h3 className="sp-group-title">{groupName}</h3>
						</div>
						<div className="sp-monitors-grid">
							{groupedMonitors[groupName].map((monitor) => (
								<MonitorCard key={monitor.id} monitor={monitor} />
							))}
						</div>
					</section>
				))}

				{/* Footer */}
				<footer className="sp-footer">
					<p className="sp-footer-text">
						Powered by <a href="https://beszel.dev" target="_blank" rel="noopener noreferrer">Beszel</a>
					</p>
					<p className="sp-footer-updated">
						Last updated: {new Date(data.updated_at).toLocaleString()}
					</p>
				</footer>
			</main>

			{/* Apply custom CSS if provided */}
			{data.custom_css && (
				<style dangerouslySetInnerHTML={{ __html: data.custom_css }} />
			)}
		</div>
	)
}
