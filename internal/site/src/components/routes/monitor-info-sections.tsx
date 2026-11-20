import { useEffect, useMemo, useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Globe, Shield, Server, MapPin, FileText, Info, AlertTriangle } from "lucide-react"
import { cn } from "@/lib/utils"
import { formatDate } from "@/lib/domains"
import type { Monitor, Heartbeat } from "@/lib/monitors"

// --- Styled components inspired by domainstack.io ---

function InfoSection({
	title,
	icon: Icon,
	children,
	accent = "slate",
}: {
	title: string
	icon: React.ElementType
	children: React.ReactNode
	accent?: "blue" | "green" | "orange" | "purple" | "slate"
}) {
	const accentColors = {
		blue: "border-blue-500/10 bg-blue-500/5",
		green: "border-green-500/10 bg-green-500/5",
		orange: "border-orange-500/10 bg-orange-500/5",
		purple: "border-purple-500/10 bg-purple-500/5",
		slate: "border-border bg-background/60",
	}

	return (
		<Card className={cn("relative overflow-hidden rounded-xl border", accentColors[accent])}>
			<CardHeader className="pb-3">
				<div className="flex items-center gap-2">
					<Icon className="h-4 w-4 text-muted-foreground" />
					<CardTitle className="text-base">{title}</CardTitle>
				</div>
			</CardHeader>
			<CardContent>{children}</CardContent>
		</Card>
	)
}

function KV({
	label,
	value,
	suffix,
	leading,
}: {
	label: string
	value: string
	suffix?: React.ReactNode
	leading?: React.ReactNode
}) {
	return (
		<div className="flex h-14 min-w-0 items-center justify-between gap-3 rounded-lg border bg-background/60 px-3 py-2">
			<div className="flex min-w-0 flex-col">
				<div className="text-[10px] leading-none tracking-wider uppercase text-foreground/70">{label}</div>
				<div className="inline-flex min-w-0 items-center gap-1.5 text-[13px] text-foreground/95 mt-1">
					{leading ? <span className="shrink-0">{leading}</span> : null}
					<span className="truncate">{value}</span>
					{suffix ? <span className="shrink-0">{suffix}</span> : null}
				</div>
			</div>
		</div>
	)
}

function KVGrid({ children, cols = 2 }: { children: React.ReactNode; cols?: 1 | 2 | 3 }) {
	const colClass = cols === 3 ? "sm:grid-cols-3" : cols === 1 ? "grid-cols-1" : "sm:grid-cols-2"
	return <div className={cn("grid grid-cols-1 gap-2", colClass)}>{children}</div>
}

// --- Data fetching hooks ---

interface DnsRecord {
	type: string
	value: string
	ttl?: number
}

interface DomainInfo {
	hostname: string | null
	rootDomain: string | null
	dnsRecords: DnsRecord[]
	geo: { city?: string; region?: string; country?: string; lat?: number; lon?: number } | null
	ssl: { valid: boolean; expiry?: string; daysLeft?: number } | null
	seo: {
		title?: string
		description?: string
		canonical?: string
		robots?: string
		generator?: string
	} | null
	loading: boolean
}

function useDomainInfo(monitor: Monitor | undefined, heartbeats: Heartbeat[] | undefined) {
	const [info, setInfo] = useState<DomainInfo>({
		hostname: null,
		rootDomain: null,
		dnsRecords: [],
		geo: null,
		ssl: null,
		seo: null,
		loading: true,
	})

	const hostname = useMemo(() => {
		if (!monitor) return null
		if (monitor.hostname) return monitor.hostname.toLowerCase()
		if (monitor.url) {
			try {
				const url = new URL(monitor.url.startsWith("http") ? monitor.url : `https://${monitor.url}`)
				return url.hostname.toLowerCase()
			} catch {
				return monitor.url.toLowerCase()
			}
		}
		return null
	}, [monitor])

	const rootDomain = useMemo(() => {
		if (!hostname) return null
		const clean = hostname.replace(/^www\./, "")
		const parts = clean.split(".")
		if (parts.length <= 2) return clean
		const specialTLDs = ["co.uk", "com.au", "co.jp", "com.br", "co.nz", "co.za", "co.in", "com.cn"]
		const lastTwo = parts.slice(-2).join(".")
		const lastThree = parts.slice(-3).join(".")
		if (specialTLDs.includes(lastThree)) return lastThree
		return lastTwo
	}, [hostname])

	// SSL from latest heartbeat
	useEffect(() => {
		if (!heartbeats?.length) {
			setInfo((prev) => ({ ...prev, ssl: null }))
			return
		}
		const latest = heartbeats[0]
		if (latest.cert_expiry) {
			const expiry = new Date(latest.cert_expiry * 1000)
			const daysLeft = Math.ceil((expiry.getTime() - Date.now()) / (1000 * 60 * 60 * 24))
			setInfo((prev) => ({
				...prev,
				ssl: { valid: latest.cert_valid ?? true, expiry: expiry.toISOString(), daysLeft },
			}))
		}
	}, [heartbeats])

	// Fetch DNS, geo, SEO
	useEffect(() => {
		if (!hostname) {
			setInfo((prev) => ({ ...prev, loading: false }))
			return
		}
		const host = hostname

		let cancelled = false

		async function fetchData() {
			// DNS via Cloudflare DoH
			const dnsPromise = fetch(`https://cloudflare-dns.com/dns-query?name=${encodeURIComponent(host)}&type=A`, {
				headers: { Accept: "application/dns-json" },
			})
				.then((r) => r.json())
				.then((data) => {
					const records: DnsRecord[] = []
					if (data.Answer) {
						for (const ans of data.Answer) {
							records.push({ type: "A", value: ans.data, ttl: ans.TTL })
						}
					}
					return records
				})
				.catch(() => [] as DnsRecord[])

			// Geolocation via ipapi.co (free, no key needed for basic)
			const geoPromise = fetch(`https://ipapi.co/${encodeURIComponent(host)}/json/`, {
				headers: { Accept: "application/json" },
			})
				.then((r) => {
					if (!r.ok) throw new Error("geo failed")
					return r.json()
				})
				.then((data) => ({
					city: data.city,
					region: data.region,
					country: data.country_name,
					lat: data.latitude,
					lon: data.longitude,
				}))
				.catch(() => null)

			// SEO meta tags
			const seoPromise = monitor?.url
				? fetch(monitor.url, { method: "GET", mode: "cors" })
						.then((r) => r.text())
						.then((html) => {
							const parser = new DOMParser()
							const doc = parser.parseFromString(html, "text/html")
							const getMeta = (name: string) =>
								doc.querySelector(`meta[name="${name}"]`)?.getAttribute("content") ||
								doc.querySelector(`meta[property="og:${name}"]`)?.getAttribute("content") ||
								undefined
							return {
								title: doc.querySelector("title")?.textContent || undefined,
								description: getMeta("description"),
								canonical: doc.querySelector('link[rel="canonical"]')?.getAttribute("href") || undefined,
								robots: getMeta("robots"),
								generator: getMeta("generator"),
							}
						})
						.catch(() => null)
				: Promise.resolve(null)

			const [dnsRecords, geo, seo] = await Promise.all([dnsPromise, geoPromise, seoPromise])

			if (!cancelled) {
				setInfo((prev) => ({
					...prev,
					hostname,
					rootDomain,
					dnsRecords,
					geo,
					seo,
					loading: false,
				}))
			}
		}

		fetchData()
		return () => {
			cancelled = true
		}
	}, [hostname, monitor?.url, rootDomain])

	return { ...info, hostname, rootDomain }
}

// --- Map component ---

function MapEmbed({ lat, lon, hostname }: { lat: number; lon: number; hostname?: string | null }) {
	const mapUrl = `https://www.openstreetmap.org/export/embed.html?bbox=${lon - 0.5}%2C${lat - 0.5}%2C${lon + 0.5}%2C${lat + 0.5}&layer=mapnik&marker=${lat}%2C${lon}`
	return (
		<div className="relative h-[220px] w-full overflow-hidden rounded-lg border">
			<iframe
				title={`Map for ${hostname || "location"}`}
				src={mapUrl}
				className="h-full w-full border-0"
				loading="lazy"
			/>
			<a
				href={`https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=12/${lat}/${lon}`}
				target="_blank"
				rel="noopener noreferrer"
				className="absolute bottom-2 right-2 rounded bg-white/90 px-2 py-1 text-[10px] font-medium text-foreground shadow hover:bg-white"
			>
				View Larger Map
			</a>
		</div>
	)
}

// --- Main composite component ---

export function MonitorInfoSections({
	monitor,
	heartbeats,
}: {
	monitor: Monitor | undefined
	heartbeats: Heartbeat[] | undefined
}) {
	const { hostname, rootDomain, dnsRecords, geo, ssl, seo, loading } = useDomainInfo(monitor, heartbeats)

	if (!monitor) return null

	const showSeo = seo && (seo.title || seo.description)

	return (
		<div className="grid gap-4">
			{/* Registration / Domain Overview */}
			<InfoSection title="Domain Overview" icon={Globe} accent="blue">
				<KVGrid>
					<KV
						label="Hostname"
						value={hostname || "N/A"}
						leading={<Globe className="h-3.5 w-3.5 text-muted-foreground" />}
					/>
					<KV
						label="Root Domain"
						value={rootDomain || "N/A"}
						leading={<Server className="h-3.5 w-3.5 text-muted-foreground" />}
					/>
					<KV label="Type" value={monitor.type.toUpperCase()} />
					<KV label="Created" value={formatDate(monitor.created)} />
				</KVGrid>
			</InfoSection>

			<div className="grid sm:grid-cols-2 gap-4">
				{/* Hosting & Geolocation */}
				<InfoSection title="Hosting & Location" icon={MapPin} accent="green">
					{geo ? (
						<div className="space-y-3">
							<KVGrid cols={1}>
								<KV
									label="Location"
									value={[geo.city, geo.region, geo.country].filter(Boolean).join(", ") || "Unknown"}
									leading={<MapPin className="h-3.5 w-3.5 text-muted-foreground" />}
								/>
							</KVGrid>
							{geo.lat && geo.lon ? <MapEmbed lat={geo.lat} lon={geo.lon} hostname={hostname} /> : null}
						</div>
					) : loading ? (
						<div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
							<span className="h-4 w-4 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-foreground" />
							Looking up location...
						</div>
					) : (
						<div className="flex items-start gap-3 rounded-lg border border-dashed p-4">
							<Info className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
							<div className="text-sm text-muted-foreground">No geolocation data available.</div>
						</div>
					)}
				</InfoSection>

				{/* SSL Certificate */}
				<InfoSection title="SSL Certificate" icon={Shield} accent="purple">
					{ssl ? (
						<div className="space-y-2">
							<KVGrid cols={1}>
								<KV
									label="Status"
									value={ssl.valid ? "Valid" : "Invalid"}
									leading={
										<div className={cn("h-2.5 w-2.5 rounded-full", ssl.valid ? "bg-green-500" : "bg-red-500")} />
									}
								/>
								<KV label="Expires" value={ssl.expiry ? formatDate(ssl.expiry) : "Unknown"} />
								{ssl.daysLeft !== undefined && (
									<KV
										label="Days Left"
										value={`${ssl.daysLeft} days`}
										suffix={
											ssl.daysLeft <= 7 ? (
												<Badge variant="destructive" className="text-[10px]">
													Expiring Soon
												</Badge>
											) : ssl.daysLeft <= 30 ? (
												<Badge variant="outline" className="text-[10px] border-yellow-500/50 text-yellow-600">
													Warning
												</Badge>
											) : null
										}
									/>
								)}
							</KVGrid>
						</div>
					) : monitor.type === "https" || monitor.url?.startsWith("https") ? (
						<div className="flex items-start gap-3 rounded-lg border border-dashed p-4">
							<AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
							<div className="text-sm text-muted-foreground">No SSL data yet. It will appear after the next check.</div>
						</div>
					) : (
						<div className="flex items-start gap-3 rounded-lg border border-dashed p-4">
							<Info className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
							<div className="text-sm text-muted-foreground">SSL not applicable for this monitor type.</div>
						</div>
					)}
				</InfoSection>
			</div>

			{/* DNS Records */}
			{dnsRecords.length > 0 && (
				<InfoSection title="DNS Records" icon={Server} accent="orange">
					<div className="space-y-2">
						{dnsRecords.map((rec, i) => (
							<div key={i} className="flex items-center gap-3 rounded-lg border bg-background/60 px-3 py-2">
								<Badge variant="outline" className="shrink-0 font-mono text-[10px]">
									{rec.type}
								</Badge>
								<span className="text-[13px] text-foreground/90 truncate">{rec.value}</span>
								{rec.ttl && <span className="ml-auto text-[10px] text-muted-foreground">TTL {rec.ttl}</span>}
							</div>
						))}
					</div>
				</InfoSection>
			)}

			{/* SEO & Meta */}
			{showSeo && (
				<InfoSection title="SEO & Social" icon={FileText} accent="slate">
					<KVGrid>
						{seo?.title && <KV label="Title" value={seo.title} />}
						{seo?.description && <KV label="Description" value={seo.description} />}
						{seo?.canonical && <KV label="Canonical" value={seo.canonical} />}
						{seo?.robots && <KV label="Robots" value={seo.robots} />}
						{seo?.generator && <KV label="Generator" value={seo.generator} />}
					</KVGrid>
				</InfoSection>
			)}
		</div>
	)
}
