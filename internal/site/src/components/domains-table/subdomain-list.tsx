"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useToast } from "@/components/ui/use-toast"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table"
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "@/components/ui/tooltip"
import { Skeleton } from "@/components/ui/skeleton"
import {
	Globe,
	RefreshCw,
	Trash2,
	Search,
	Server,
	ExternalLink,
	CheckCircle2,
	XCircle,
	Shield,
} from "lucide-react"
import {
	getDomainSubdomains,
	refreshSubdomainDiscovery,
	deleteSubdomain,
} from "@/lib/domains"
import { useState } from "react"

interface SubdomainListProps {
	domainId: string
}

function SubdomainStatusBadge({ status }: { status: string }) {
	const configs = {
		active: { color: "bg-green-500", icon: CheckCircle2, text: "Active" },
		inactive: { color: "bg-gray-500", icon: XCircle, text: "Inactive" },
		error: { color: "bg-red-500", icon: XCircle, text: "Error" },
	}

	const config = configs[status as keyof typeof configs] || configs.inactive
	const Icon = config.icon

	return (
		<div className="flex items-center gap-1.5">
			<div className={`h-2 w-2 rounded-full ${config.color}`} />
			<Icon className="h-3.5 w-3.5 text-muted-foreground" />
			<span className="text-xs capitalize">{config.text}</span>
		</div>
	)
}

function SourceBadge({ source }: { source: string }) {
	const sourceConfig: Record<string, { label: string; variant: "default" | "secondary" | "outline" }> = {
		dns: { label: "DNS", variant: "default" },
		http: { label: "HTTP", variant: "secondary" },
		pattern: { label: "Pattern", variant: "outline" },
		certificate: { label: "Cert", variant: "secondary" },
	}

	const config = sourceConfig[source] || { label: source, variant: "outline" }

	return (
		<Badge variant={config.variant} className="text-xs">
			{config.label}
		</Badge>
	)
}

export function SubdomainList({ domainId }: SubdomainListProps) {
	const { toast } = useToast()
	const queryClient = useQueryClient()
	const [isDiscovering, setIsDiscovering] = useState(false)

	const { data: subdomains, isLoading } = useQuery({
		queryKey: ["domain-subdomains", domainId],
		queryFn: () => getDomainSubdomains(domainId),
	})

	const refreshMutation = useMutation({
		mutationFn: async () => {
			setIsDiscovering(true)
			await refreshSubdomainDiscovery(domainId)
			// Wait a bit for discovery to start
			await new Promise((resolve) => setTimeout(resolve, 2000))
			setIsDiscovering(false)
		},
		onSuccess: () => {
			toast({ title: "Subdomain discovery started" })
			queryClient.invalidateQueries({ queryKey: ["domain-subdomains", domainId] })
		},
		onError: (error: Error) => {
			setIsDiscovering(false)
			toast({
				title: "Discovery failed",
				description: error.message,
				variant: "destructive",
			})
		},
	})

	const deleteMutation = useMutation({
		mutationFn: deleteSubdomain,
		onSuccess: () => {
			toast({ title: "Subdomain deleted" })
			queryClient.invalidateQueries({ queryKey: ["domain-subdomains", domainId] })
		},
		onError: (error: Error) => {
			toast({
				title: "Failed to delete",
				description: error.message,
				variant: "destructive",
			})
		},
	})

	if (isLoading) {
		return (
			<Card>
				<CardHeader>
					<Skeleton className="h-6 w-32" />
					<Skeleton className="h-4 w-48 mt-2" />
				</CardHeader>
				<CardContent>
					<div className="space-y-2">
						<Skeleton className="h-10 w-full" />
						<Skeleton className="h-10 w-full" />
						<Skeleton className="h-10 w-full" />
					</div>
				</CardContent>
			</Card>
		)
	}

	const activeCount = subdomains?.filter((s) => s.status === "active").length || 0

	return (
		<Card>
			<CardHeader>
				<div className="flex items-center justify-between">
					<div>
						<CardTitle className="text-lg flex items-center gap-2">
							<Search className="h-5 w-5" />
							Discovered Subdomains
							{subdomains && subdomains.length > 0 && (
								<Badge variant="secondary" className="ml-2">
									{activeCount}/{subdomains.length} active
								</Badge>
							)}
						</CardTitle>
						<CardDescription>
							Subdomains discovered through DNS, HTTP, and pattern enumeration
						</CardDescription>
					</div>
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="outline"
									size="sm"
									onClick={() => refreshMutation.mutate()}
									disabled={isDiscovering || refreshMutation.isPending}
								>
									<RefreshCw className={`mr-2 h-4 w-4 ${isDiscovering ? "animate-spin" : ""}`} />
									{isDiscovering ? "Discovering..." : "Discover"}
								</Button>
							</TooltipTrigger>
							<TooltipContent>
								<p>Run enhanced subdomain discovery</p>
							</TooltipContent>
							</Tooltip>
					</TooltipProvider>
				</div>
			</CardHeader>
			<CardContent>
				{!subdomains || subdomains.length === 0 ? (
					<div className="text-center py-8">
						<Search className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
						<p className="text-lg font-medium">No subdomains discovered yet</p>
						<p className="text-muted-foreground mb-4">
							Run discovery to find subdomains via DNS, HTTP, and certificate transparency logs
						</p>
						<Button
							onClick={() => refreshMutation.mutate()}
							disabled={isDiscovering || refreshMutation.isPending}
						>
							<RefreshCw className={`mr-2 h-4 w-4 ${isDiscovering ? "animate-spin" : ""}`} />
							{isDiscovering ? "Discovering..." : "Start Discovery"}
						</Button>
					</div>
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Subdomain</TableHead>
								<TableHead>Status</TableHead>
								<TableHead>Source</TableHead>
								<TableHead>IP Addresses</TableHead>
								<TableHead>HTTP</TableHead>
								<TableHead className="w-[100px]">Actions</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{subdomains.map((subdomain) => (
								<TableRow key={subdomain.id}>
									<TableCell className="font-medium">
										<div className="flex items-center gap-2">
											<Globe className="h-4 w-4 text-muted-foreground" />
											<span className="font-mono text-sm">
												{subdomain.subdomain_name}
											</span>
										</div>
									</TableCell>
									<TableCell>
										<SubdomainStatusBadge status={subdomain.status} />
									</TableCell>
									<TableCell>
										<SourceBadge source={subdomain.discovery_source} />
									</TableCell>
									<TableCell>
										{subdomain.ip_addresses ? (
											<div className="flex items-center gap-1.5">
												<Server className="h-3.5 w-3.5 text-muted-foreground" />
												<span className="text-xs text-muted-foreground truncate max-w-[150px]">
													{subdomain.ip_addresses.split(",").length} IP(s)
												</span>
											</div>
										) : (
											<span className="text-xs text-muted-foreground">-</span>
										)}
									</TableCell>
									<TableCell>
										{subdomain.http_status ? (
											<div className="flex items-center gap-2">
												<Badge
													variant={subdomain.http_status < 400 ? "default" : "destructive"}
													className="text-xs"
												>
													{subdomain.http_status}
												</Badge>
												{subdomain.server_header && (
													<TooltipProvider>
														<Tooltip>
															<TooltipTrigger>
																<Shield className="h-3.5 w-3.5 text-muted-foreground" />
															</TooltipTrigger>
															<TooltipContent>
																<p className="text-xs">{subdomain.server_header}</p>
															</TooltipContent>
														</Tooltip>
													</TooltipProvider>
												)}
												<a
													href={`https://${subdomain.full_domain}`}
													target="_blank"
													rel="noopener noreferrer"
													className="text-muted-foreground hover:text-foreground"
												>
													<ExternalLink className="h-3.5 w-3.5" />
												</a>
											</div>
										) : (
											<span className="text-xs text-muted-foreground">-</span>
										)}
									</TableCell>
									<TableCell>
										<Button
											variant="ghost"
											size="icon"
											className="h-8 w-8 text-destructive"
											onClick={() => deleteMutation.mutate(subdomain.id)}
											disabled={deleteMutation.isPending}
										>
											<Trash2 className="h-4 w-4" />
										</Button>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				)}
			</CardContent>
		</Card>
	)
}
