import { $tailscaleNodes, pb } from "@/lib/stores"
import { TailscaleNode, ChartTimes } from "@/types"
import { useEffect, useState } from "react"
import { Card, CardHeader, CardTitle, CardContent, CardDescription } from "../ui/card"
import { useStore } from "@nanostores/react"
import Spinner from "../spinner"
import { 
	ServerIcon, 
	WifiIcon, 
	TagIcon, 
	GlobeIcon,
	MonitorIcon,
	SmartphoneIcon,
	AppleIcon,
	UserIcon,
	CalendarIcon,

} from "lucide-react"
import { WindowsIcon, TuxIcon } from "../ui/icons"
import { cn } from "@/lib/utils"
import { useLingui } from "@lingui/react/macro"
import { navigate } from "../router"
import { Button } from "../ui/button"
import { Badge } from "../ui/badge"
import { Separator } from "../ui/separator"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { $chartTime, $direction } from "@/lib/stores"
import { chartTimeData, getPbTimestamp } from "@/lib/utils"
import { timeTicks } from "d3-time"
import { ClockArrowUp } from "lucide-react"

import { Label } from "../ui/label"
import LatencyChart from "../charts/tailscale-latency-chart"
import ChartTimeSelect from "../charts/chart-time-select"

// Cache for time data
const cache = new Map<string, any>()

// create ticks and domain for charts (copied from system.tsx)
function getTimeData(chartTime: ChartTimes, lastCreated: number) {
	const cached = cache.get("td")
	if (cached && cached.chartTime === chartTime) {
		if (!lastCreated || cached.time >= lastCreated) {
			return cached.data
		}
	}

	const now = new Date()
	const startTime = chartTimeData[chartTime].getOffset(now)
	const ticks = timeTicks(startTime, now, chartTimeData[chartTime].ticks ?? 12).map((date) => date.getTime())
	const data = {
		ticks,
		domain: [chartTimeData[chartTime].getOffset(now).getTime(), now.getTime()],
	}
	cache.set("td", { time: now.getTime(), data, chartTime })
	return data
}

// Import ChartCard from the system component
const ChartCard = ({ title, description, children, grid, empty, cornerEl }: {
	title: string
	description: string
	children: React.ReactNode
	grid?: boolean
	empty?: boolean
	cornerEl?: JSX.Element | null
}) => {
	const { isIntersecting, ref } = useIntersectionObserver()

	return (
		<Card className={cn("pb-2 sm:pb-4 odd:last-of-type:col-span-full", { "col-span-full": !grid })} ref={ref}>
			<CardHeader className="pb-5 pt-4 relative space-y-1 max-sm:py-3 max-sm:px-4">
				<CardTitle className="text-xl sm:text-2xl">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
				{cornerEl && <div className="relative py-1 block sm:w-44 sm:absolute sm:top-2.5 sm:end-3.5">{cornerEl}</div>}
			</CardHeader>
			<div className="ps-0 w-[calc(100%-1.5em)] h-48 md:h-52 relative group">
				{empty && (
					<div className="flex items-center justify-center h-full">
						<p className="text-sm text-muted-foreground">No data available</p>
					</div>
				)}
				{isIntersecting && children}
			</div>
		</Card>
	)
}

function getOSIcon(os: string) {
	const lowerOS = os.toLowerCase()
	if (lowerOS.includes("linux")) return <TuxIcon className="h-4 w-4" />
	if (lowerOS.includes("macos") || lowerOS.includes("darwin")) return <AppleIcon className="h-4 w-4" />
	if (lowerOS.includes("windows")) return <WindowsIcon className="h-4 w-4" />
	if (lowerOS.includes("ios")) return <SmartphoneIcon className="h-4 w-4" />
	if (lowerOS.includes("android")) return <SmartphoneIcon className="h-4 w-4" />
	if (lowerOS.includes("tvos")) return <AppleIcon className="h-4 w-4" />
	return <ServerIcon className="h-4 w-4" />
}

function getStatusText(online: boolean) {
	return online ? "Online" : "Offline"
}

function truncateTailnetName(name: string) {
	// If the name contains a tailnet domain (e.g., "apprise.tail43c135.ts.net")
	// truncate it to just the hostname part (e.g., "apprise")
	if (name.includes(".")) {
		return name.split(".")[0]
	}
	return name
}

function truncateVersion(version: string) {
	// If the version contains a dash, truncate it to just the part before the dash
	// e.g., "1.54.0-1234567890abcdef" becomes "1.54.0"
	if (version.includes("-")) {
		return version.split("-")[0]
	}
	return version
}

function getPreferredDerpServer(latencyData: Array<{created: number, latency: Record<string, { latencyMs: number; preferred?: boolean }>}>) {
	if (latencyData.length === 0) return null
	
	// Get the most recent data point
	const latestData = latencyData[latencyData.length - 1]
	
	// Find the preferred DERP server
	for (const [serverName, serverData] of Object.entries(latestData.latency)) {
		if (serverData.preferred) {
			return { name: serverName, latency: serverData.latencyMs }
		}
	}
	
	// If no preferred server found, return the one with lowest latency
	let lowestLatency = Infinity
	let fastestServer = null
	
	for (const [serverName, serverData] of Object.entries(latestData.latency)) {
		if (serverData.latencyMs < lowestLatency) {
			lowestLatency = serverData.latencyMs
			fastestServer = { name: serverName, latency: serverData.latencyMs }
		}
	}
	
	return fastestServer
}

export default function TailscaleNodeDetail({ nodeId }: { nodeId: string }) {
	const tailscaleNodes = useStore($tailscaleNodes)
	const chartTime = useStore($chartTime)
	const [node, setNode] = useState<TailscaleNode | null>(null)
	const [loading, setLoading] = useState(true)
	const [latencyData, setLatencyData] = useState<Array<{created: number, latency: Record<string, { latencyMs: number; preferred?: boolean }>}>>([])
	const [setExpandedSections] = useState<Record<string, boolean>>({
		network: true,
		info: false,
		latency: false,
	})

	useEffect(() => {
		const fetchNodeData = async () => {
			try {
				// Always fetch from database first
				const record = await pb.collection('tailscale_stats').getFirstListItem(`node_id = "${nodeId}"`)
				if (record) {
					// Parse the info field to get node details

					let nodeInfo: any = {}
					try {
						if (record.info && typeof record.info === 'string') {
							// Check if the info field is actually JSON and not just "object"
							if (record.info.trim() !== 'object' && record.info.trim() !== '[object Object]') {
								nodeInfo = JSON.parse(record.info)
							} else {
								console.warn("Node info contains invalid data:", record.info)
								// If the info field is corrupted, try to use the record itself
								nodeInfo = record
							}
						} else if (record.info && typeof record.info === 'object') {
							// If info is already an object, use it directly
							nodeInfo = record.info
						}
					} catch (e) {
						console.warn("Failed to parse node info", e)
						// Fallback to using the record itself
						nodeInfo = record
					}

					const nodeData: TailscaleNode = {
						id: record.node_id,
						node_id: record.node_id,
						nodeId: nodeInfo.nodeId || "",
						name: nodeInfo.name || record.node_id,
						hostname: nodeInfo.hostname || record.node_id,
						ip: nodeInfo.addresses?.[0] || "",
						addresses: nodeInfo.addresses || [],
						user: nodeInfo.user || "",
						os: nodeInfo.os || "",
						version: nodeInfo.version || "",
						created: nodeInfo.created || record.created,
						lastSeen: nodeInfo.lastSeen || record.lastSeen || "",
						online: nodeInfo.online || false,
						keyExpiry: nodeInfo.keyExpiry || "",
						keyExpiryDisabled: nodeInfo.keyExpiryDisabled || false,
						authorized: nodeInfo.authorized || false,
						isExternal: nodeInfo.isExternal || false,
						updateAvailable: nodeInfo.updateAvailable || false,
						blocksIncomingConnections: nodeInfo.blocksIncomingConnections || false,
						machineKey: nodeInfo.machineKey || "",
						nodeKey: nodeInfo.nodeKey || "",
						tailnetLockKey: nodeInfo.tailnetLockKey || "",
						tailnetLockError: nodeInfo.tailnetLockError || "",
						tags: nodeInfo.tags || [],
						advertisedRoutes: nodeInfo.advertisedRoutes || [],
						enabledRoutes: nodeInfo.enabledRoutes || [],
						endpoints: nodeInfo.endpoints || [],
						mappingVariesByDestIP: nodeInfo.mappingVariesByDestIP || false,
						derpLatency: nodeInfo.derpLatency || {},
						clientSupports: nodeInfo.clientSupports || null,
						tailnet: record.tailnet,
						network: record.network,
						info: record.info,
						updated: record.updated,
					}
					setNode(nodeData)

					// Fetch latency data for this node
					try {
						// Use the same pattern as system page but without type field (tailscale_stats doesn't have it)
						const latencyRecords = await pb.collection('tailscale_stats').getFullList({
							filter: pb.filter("node_id={:nodeId} && created > {:created}", {
								nodeId: nodeId,
								created: getPbTimestamp(chartTime),
							}),
							fields: "created,network",
							sort: "created",
						})
						
						const latencyDataPoints = latencyRecords.map(record => {
							let networkData: any = {}
							try {
								if (record.network && typeof record.network === 'string') {
									networkData = JSON.parse(record.network)
								} else if (record.network && typeof record.network === 'object') {
									networkData = record.network
								}
							} catch (e) {
								console.warn("Failed to parse network data", e)
							}
							
							return {
								created: new Date(record.created).getTime(),
								latency: networkData.latency || {}
							}
						}).filter(point => Object.keys(point.latency).length > 0) // Only include points with valid latency data
						

						setLatencyData(latencyDataPoints.reverse()) // Reverse to show oldest to newest
					} catch (error) {
						console.error("Failed to fetch latency data:", error)
						setLatencyData([])
					}
				} else {
					// If not found in database, try to find in store as fallback
					const existingNode = tailscaleNodes.find(n => n.id === nodeId)
					if (existingNode) {
						console.log('Using store data as fallback:', existingNode)
						setNode(existingNode)
					} else {
						console.error('Node not found in database or store')
					}
				}
			} catch (error) {
				console.error("Failed to fetch node data:", error)
			} finally {
				setLoading(false)
			}
		}

		fetchNodeData()
	}, [nodeId, tailscaleNodes, chartTime])

	if (loading) {
		return (
			<div className="flex items-center justify-center h-64">
				<Spinner />
			</div>
		)
	}

	if (!node) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="text-center">
					<h2 className="text-xl font-semibold mb-2">Node not found</h2>
					<p className="text-gray-500 mb-4">The requested Tailscale node could not be found.</p>
					<Button onClick={() => navigate("/")}>
						Back to Home
					</Button>
				</div>
			</div>
		)
	}

	const nodeInfo = node.info || {}
	const networkInfo = node.network || {}
	const isOnline = nodeInfo?.online || node.online || false
	


	return (
		<>
			<div id="chartwrap" className="grid gap-4 mb-10 overflow-x-clip">
				{/* node info */}
				<Card>
					<div className="grid xl:flex gap-4 px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
						<div>
							<h1 className="text-[1.6rem] font-semibold mb-1.5 flex items-center space-x-2">
								<span>{truncateTailnetName(nodeInfo?.name || node.name || node.node_id)}</span>
							</h1>
							<div className="flex flex-wrap items-center gap-3 gap-y-2 text-sm opacity-90">
								<div className="capitalize flex gap-2 items-center">
									<span className={cn("relative flex h-3 w-3")}>
										{isOnline && (
											<span
												className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"
												style={{ animationDuration: "1.5s" }}
											></span>
										)}
										<span
											className={cn("relative inline-flex rounded-full h-3 w-3", {
												"bg-green-500": isOnline,
												"bg-red-500": !isOnline,
											})}
										></span>
									</span>
									{getStatusText(isOnline)}
								</div>
								{node.addresses && node.addresses.length > 0 && (
									<>
										<Separator orientation="vertical" className="h-4 bg-primary/30" />
										<div className="flex gap-1.5 items-center">
											<GlobeIcon className="h-4 w-4" /> {node.addresses[0]}
										</div>
									</>
								)}
								{nodeInfo?.name && (
									<>
										<Separator orientation="vertical" className="h-4 bg-primary/30" />
										<div className="flex gap-1.5 items-center">
											<MonitorIcon className="h-4 w-4" /> {nodeInfo.name}
										</div>
									</>
								)}
								{nodeInfo?.os && (
									<>
										<Separator orientation="vertical" className="h-4 bg-primary/30" />
										<div className="flex gap-1.5 items-center">
											{getOSIcon(nodeInfo.os)} {nodeInfo.os}
										</div>
									</>
								)}
								{nodeInfo?.version && (
									<>
										<Separator orientation="vertical" className="h-4 bg-primary/30" />
										<div className="flex gap-1.5 items-center">
											<WifiIcon className="h-4 w-4" /> {truncateVersion(nodeInfo.version)}
										</div>
									</>
								)}
								{node.user && (
									<>
										<Separator orientation="vertical" className="h-4 bg-primary/30" />
										<div className="flex gap-1.5 items-center">
											<UserIcon className="h-4 w-4" /> {node.user}
										</div>
									</>
								)}
								{node.tags && node.tags.length > 0 && (
									<>
										<Separator orientation="vertical" className="h-4 bg-primary/30" />
										<div className="flex gap-1.5 items-center">
											<TagIcon className="h-4 w-4" />
											<div className="flex gap-1">
												{node.tags.slice(0, 2).map((tag: string, index: number) => (
													<span
														key={index}
														className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-1 text-xs"
													>
														{tag}
													</span>
												))}
												{node.tags.length > 2 && (
													<span className="text-xs text-muted-foreground">+{node.tags.length - 2}</span>
												)}
											</div>
										</div>
									</>
								)}
								{(() => {
									// Check if key expiry is disabled - either explicitly or by having a far future date
									const hasKeyExpiry = nodeInfo?.keyExpiry && nodeInfo.keyExpiry !== ""
									const checkExpiryDate = hasKeyExpiry ? new Date(nodeInfo.keyExpiry) : null
									const isDisabled = node.keyExpiryDisabled || 
										(checkExpiryDate && checkExpiryDate.getFullYear() > 2030) // Assume disabled if expiry is very far in future
									

									if (isDisabled) {
										return (
											<>
												<Separator orientation="vertical" className="h-4 bg-primary/30" />
												<div className="flex gap-1.5 items-center">
													<CalendarIcon className="h-4 w-4" />
													<span className="text-sm">Never</span>
												</div>
											</>
										)
									}
									
									const expiryDate = new Date(nodeInfo?.keyExpiry || node.keyExpiry || "")
									const now = new Date()
									const diffTime = expiryDate.getTime() - now.getTime()
									const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24))
									
									if (diffDays < 0) {
										return (
											<>
												<Separator orientation="vertical" className="h-4 bg-primary/30" />
												<div className="flex gap-1.5 items-center">
													<CalendarIcon className="h-4 w-4 text-red-500" />
													<span className="text-sm text-red-500 font-medium">Expired</span>
												</div>
											</>
										)
									}
									
									if (diffDays <= 30) {
										return (
											<>
												<Separator orientation="vertical" className="h-4 bg-primary/30" />
												<div className="flex gap-1.5 items-center">
													<CalendarIcon className="h-4 w-4 text-orange-500" />
													<span className="text-sm text-orange-500 font-medium">{diffDays}d</span>
												</div>
											</>
										)
									}
									
									return (
										<>
											<Separator orientation="vertical" className="h-4 bg-primary/30" />
																							<div className="flex gap-1.5 items-center">
													<CalendarIcon className="h-4 w-4" />
													<span className="text-sm">{diffDays}d</span>
												</div>
										</>
									)
								})()}
							</div>
						</div>
						<div className="xl:ms-auto flex items-center gap-2 max-sm:-mb-1">
							<ChartTimeSelect className="w-full xl:w-40" />
						</div>
					</div>
				</Card>



				{/* Main Content */}
				<div className="grid grid-cols-1 md:grid-cols-2 gap-6">
					{/* Network Information */}
					<Card>
						<CardHeader>
							<div className="flex items-center space-x-2">
								<WifiIcon className="h-5 w-5" />
								<CardTitle>Network Information</CardTitle>
							</div>
						</CardHeader>
						<CardContent className="space-y-4">
							{/* IP Addresses */}
							{node.addresses && node.addresses.length > 0 && (
								<div>
									<Label className="text-sm font-medium">IP Addresses</Label>
									<div className="flex flex-wrap gap-1">
										{node.addresses.map((address: string, index: number) => (
											<Badge key={index} variant="outline" className="text-xs font-mono">
												{address}
											</Badge>
										))}
									</div>
								</div>
							)}

							<div className="grid grid-cols-2 gap-4">
								{latencyData.length > 0 && (() => {
									const preferredServer = getPreferredDerpServer(latencyData)
									if (preferredServer) {
										return (
											<div>
												<Label className="text-sm font-medium">Preferred DERP Server</Label>
												<p className="text-sm text-gray-600">
													{preferredServer.name} ({preferredServer.latency.toFixed(1)}ms)
												</p>
											</div>
										)
									}
									return null
								})()}
								<div>
									<Label className="text-sm font-medium">NAT Mapping Varies</Label>
									<p className="text-sm text-gray-600">{node.mappingVariesByDestIP ? "Yes" : "No"}</p>
								</div>
								<div>
									<Label className="text-sm font-medium">Blocks Incoming</Label>
									<p className="text-sm text-gray-600">{node.blocksIncomingConnections ? "Yes" : "No"}</p>
								</div>
								<div>
									<Label className="text-sm font-medium">External Device</Label>
									<p className="text-sm text-gray-600">{node.isExternal ? "Yes" : "No"}</p>
								</div>
							</div>

							{node.endpoints && node.endpoints.length > 0 && (
								<div>
									<Label className="text-sm font-medium">Endpoints</Label>
									<div className="flex flex-wrap gap-1">
										{node.endpoints.map((endpoint: string, index: number) => (
											<Badge key={index} variant="outline" className="text-xs font-mono">
												{endpoint}
											</Badge>
										))}
									</div>
								</div>
							)}

							{node.advertisedRoutes && node.advertisedRoutes.length > 0 && (
								<div>
									<Label className="text-sm font-medium">Advertised Routes</Label>
									<div className="flex flex-wrap gap-1">
										{node.advertisedRoutes.map((route: string, index: number) => (
											<Badge key={index} variant="outline" className="text-xs">
												{route}
											</Badge>
										))}
									</div>
								</div>
							)}

							{node.enabledRoutes && node.enabledRoutes.length > 0 && (
								<div>
									<Label className="text-sm font-medium">Enabled Routes</Label>
									<div className="flex flex-wrap gap-1">
										{node.enabledRoutes.map((route: string, index: number) => (
											<Badge key={index} variant="outline" className="text-xs">
												{route}
											</Badge>
										))}
									</div>
								</div>
							)}
						</CardContent>
					</Card>

					{/* Node Information */}
					<Card>
						<CardHeader>
							<div className="flex items-center space-x-2">
								<ServerIcon className="h-5 w-5" />
								<CardTitle>Node Information</CardTitle>
							</div>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="grid grid-cols-2 gap-4">
								<div>
									<Label className="text-sm font-medium">Node ID</Label>
									<p className="text-sm text-gray-600 font-mono">{node.nodeId || "N/A"}</p>
								</div>
								<div>
									<Label className="text-sm font-medium">Created</Label>
									<p className="text-sm text-gray-600">
										{node.created ? new Date(node.created).toLocaleDateString() : "N/A"}
									</p>
								</div>
								<div>
									<Label className="text-sm font-medium">Authorized</Label>
									<p className="text-sm text-gray-600">{node.authorized ? "Yes" : "No"}</p>
								</div>
								<div>
									<Label className="text-sm font-medium">Key Expiry Disabled</Label>
									<p className="text-sm text-gray-600">{node.keyExpiryDisabled ? "Yes" : "No"}</p>
								</div>
								<div>
									<Label className="text-sm font-medium">Update Available</Label>
									<p className={cn("text-sm", node.updateAvailable ? "text-orange-600 font-medium" : "text-gray-600")}>
										{node.updateAvailable ? "Yes" : "No"}
									</p>
								</div>
								<div>
									<Label className="text-sm font-medium">Last Seen</Label>
									<p className="text-sm text-gray-600">
										{node.lastSeen ? new Date(node.lastSeen).toLocaleString() : "N/A"}
									</p>
								</div>
							</div>

							{/* Client Capabilities */}
							{node.clientSupports && (
								<div>
									<Label className="text-sm font-medium">Client Capabilities</Label>
									<div className="flex flex-wrap gap-1 mt-1">
										{node.clientSupports.udp && (
											<Badge variant="outline" className="text-xs">UDP</Badge>
										)}
										{node.clientSupports.ipv6 && (
											<Badge variant="outline" className="text-xs">IPv6</Badge>
										)}
										{node.clientSupports.pcp && (
											<Badge variant="outline" className="text-xs">PCP</Badge>
										)}
										{node.clientSupports.pmp && (
											<Badge variant="outline" className="text-xs">PMP</Badge>
										)}
										{node.clientSupports.upnp && (
											<Badge variant="outline" className="text-xs">UPnP</Badge>
										)}
										{node.clientSupports.hairPinning && (
											<Badge variant="outline" className="text-xs">Hair Pinning</Badge>
										)}
									</div>
								</div>
							)}
						</CardContent>
					</Card>
				</div>

				{/* Latency Chart */}
				{latencyData.length > 0 && (
					<ChartCard
						empty={false}
						grid={false}
						title="DERP Latency"
						description="Network latency to top 10 DERP servers over time"
					>
						<LatencyChart 
							data={latencyData}
							chartData={{ 
								orientation: "left",
								chartTime: chartTime,
								...getTimeData(chartTime, latencyData.length > 0 ? latencyData[latencyData.length - 1].created : Date.now()),
								systemStats: [],
								containerData: [],
								agentVersion: { major: 1, minor: 0, patch: 0 }
							}}
						/>
					</ChartCard>
				)}
			</div>
		</>
	)
} 