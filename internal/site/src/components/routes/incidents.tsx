import { memo, useEffect, useState } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Separator } from "@/components/ui/separator"
import { useToast } from "@/components/ui/use-toast"
import { CalendarIcon, AlertTriangle, CheckCircle2, Clock, Plus, Eye, Check, X, MessageSquare, ShieldAlert } from "lucide-react"
import { getIncidents, acknowledgeIncident, resolveIncident, closeIncident, getIncidentUpdates, addIncidentUpdate, createIncident, type Incident, type IncidentUpdate } from "@/lib/incidents"
import { formatDate } from "@/lib/domains"

function StatusBadge({ status }: { status: string }) {
	const configs: Record<string, { color: string; icon: React.ElementType; text: string }> = {
		open: { color: "bg-red-500", icon: AlertTriangle, text: "Open" },
		acknowledged: { color: "bg-yellow-500", icon: Clock, text: "Acknowledged" },
		resolved: { color: "bg-green-500", icon: CheckCircle2, text: "Resolved" },
		closed: { color: "bg-gray-500", icon: CheckCircle2, text: "Closed" },
	}
	const config = configs[status] || configs.open
	const Icon = config.icon
	return (
		<div className="flex items-center gap-2">
			<div className={`h-2.5 w-2.5 rounded-full ${config.color}`} />
			<Icon className="h-4 w-4 text-muted-foreground" />
			<span className="capitalize text-sm">{config.text}</span>
		</div>
	)
}

function SeverityBadge({ severity }: { severity: string }) {
	const colors: Record<string, string> = {
		critical: "bg-red-500",
		high: "bg-orange-500",
		medium: "bg-yellow-500",
		low: "bg-blue-500",
	}
	return <Badge className={colors[severity] || "bg-gray-500"}>{severity}</Badge>
}

function IncidentSkeleton() {
	return (
		<div className="grid gap-4">
			{[1, 2, 3].map((i) => (
				<Card key={i}>
					<CardHeader className="pb-2">
						<div className="flex items-center justify-between">
							<Skeleton className="h-6 w-48" />
							<div className="flex gap-2">
								<Skeleton className="h-5 w-16" />
								<Skeleton className="h-5 w-20" />
							</div>
						</div>
					</CardHeader>
					<CardContent>
						<Skeleton className="h-4 w-full mb-3" />
						<div className="flex gap-4">
							<Skeleton className="h-3 w-24" />
							<Skeleton className="h-3 w-24" />
						</div>
					</CardContent>
				</Card>
			))}
		</div>
	)
}

export default memo(() => {
	const { t } = useLingui()
	const { toast } = useToast()
	const queryClient = useQueryClient()
	const [filter, setFilter] = useState("all")
	const [selectedIncident, setSelectedIncident] = useState<Incident | null>(null)
	const [isDetailOpen, setIsDetailOpen] = useState(false)
	const [isCreateOpen, setIsCreateOpen] = useState(false)

	useEffect(() => {
		document.title = `${t`Incidents`} / Beszel`
	}, [t])

	const { data: incidents = [], isLoading } = useQuery({
		queryKey: ["incidents", filter],
		queryFn: () => getIncidents(filter === "all" ? undefined : { status: filter }),
	})

	const { data: incidentUpdates = [] } = useQuery({
		queryKey: ["incident-updates", selectedIncident?.id],
		queryFn: () => getIncidentUpdates(selectedIncident!.id),
		enabled: Boolean(selectedIncident) && isDetailOpen,
	})

	const acknowledgeMutation = useMutation({
		mutationFn: acknowledgeIncident,
		onSuccess: () => {
			toast({ title: "Incident acknowledged" })
			queryClient.invalidateQueries({ queryKey: ["incidents"] })
			if (selectedIncident) {
				queryClient.invalidateQueries({ queryKey: ["incident", selectedIncident.id] })
			}
		},
	})

	const resolveMutation = useMutation({
		mutationFn: (id: string) => resolveIncident(id),
		onSuccess: () => {
			toast({ title: "Incident resolved" })
			queryClient.invalidateQueries({ queryKey: ["incidents"] })
			if (selectedIncident) {
				queryClient.invalidateQueries({ queryKey: ["incident", selectedIncident.id] })
			}
		},
	})

	const closeMutation = useMutation({
		mutationFn: closeIncident,
		onSuccess: () => {
			toast({ title: "Incident closed" })
			queryClient.invalidateQueries({ queryKey: ["incidents"] })
			if (selectedIncident) {
				queryClient.invalidateQueries({ queryKey: ["incident", selectedIncident.id] })
				setIsDetailOpen(false)
			}
		},
	})

	const addUpdateMutation = useMutation({
		mutationFn: ({ id, message }: { id: string; message: string }) => addIncidentUpdate(id, message),
		onSuccess: () => {
			toast({ title: "Update added" })
			queryClient.invalidateQueries({ queryKey: ["incident-updates", selectedIncident?.id] })
		},
	})

	const createMutation = useMutation({
		mutationFn: createIncident,
		onSuccess: () => {
			toast({ title: "Incident created" })
			queryClient.invalidateQueries({ queryKey: ["incidents"] })
			setIsCreateOpen(false)
		},
	})

	const openDetail = (incident: Incident) => {
		setSelectedIncident(incident)
		setIsDetailOpen(true)
	}

	const getIncidentStats = () => {
		const total = incidents.length
		const open = incidents.filter((i) => i.status === "open").length
		const acknowledged = incidents.filter((i) => i.status === "acknowledged").length
		const resolved = incidents.filter((i) => i.status === "resolved").length
		return { total, open, acknowledged, resolved }
	}

	const stats = getIncidentStats()

	return (
		<div className="container flex flex-col gap-6">
			<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
				<div className="flex items-center gap-3">
					<div className="p-2 bg-primary/10 rounded-lg">
						<ShieldAlert className="h-5 w-5 text-primary" />
					</div>
					<div>
						<h1 className="text-2xl font-semibold">{t`Incidents`}</h1>
						<p className="text-sm text-muted-foreground">
							{stats.open > 0 && <span className="text-red-500 font-medium">{stats.open} open</span>}
							{stats.open > 0 && stats.acknowledged > 0 && <span className="mx-1">•</span>}
							{stats.acknowledged > 0 && <span className="text-yellow-500 font-medium">{stats.acknowledged} acknowledged</span>}
							{(stats.open > 0 || stats.acknowledged > 0) && <span className="mx-1">•</span>}
							{stats.total} total
						</p>
					</div>
				</div>
				<div className="flex flex-wrap items-center gap-2">
					{["all", "open", "acknowledged", "resolved", "closed"].map((s) => (
						<Button
							key={s}
							variant={filter === s ? "default" : "outline"}
							size="sm"
							onClick={() => setFilter(s)}
						>
							{s.charAt(0).toUpperCase() + s.slice(1)}
						</Button>
					))}
					<Dialog open={isCreateOpen} onOpenChange={setIsCreateOpen}>
						<DialogTrigger asChild>
							<Button size="sm" className="ml-2">
								<Plus className="h-4 w-4 mr-1" />
								<Trans>Create</Trans>
							</Button>
						</DialogTrigger>
						<CreateIncidentDialog onSubmit={(data) => createMutation.mutate(data)} isLoading={createMutation.isPending} />
					</Dialog>
				</div>
			</div>

			{isLoading ? (
				<IncidentSkeleton />
			) : incidents.length === 0 ? (
				<Card>
					<CardContent className="p-8 text-center">
						<div className="flex justify-center mb-4">
							<ShieldAlert className="h-12 w-12 text-muted-foreground opacity-50" />
						</div>
						<p className="text-muted-foreground">{t`No incidents found.`}</p>
						<p className="text-sm text-muted-foreground mt-1">Monitor down events and manual incidents will appear here</p>
					</CardContent>
				</Card>
			) : (
				<div className="grid gap-4">
					{incidents.map((incident: Incident) => (
						<Card key={incident.id} className="hover:border-primary/30 transition-all">
							<CardHeader className="pb-2">
								<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
									<div className="flex items-center gap-2">
										<CardTitle className="text-lg">{incident.title}</CardTitle>
									</div>
									<div className="flex items-center gap-2">
										<SeverityBadge severity={incident.severity} />
										<StatusBadge status={incident.status} />
									</div>
								</div>
							</CardHeader>
							<CardContent>
								{incident.description && (
									<p className="text-sm text-muted-foreground mb-3">
										{incident.description}
									</p>
								)}
								<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
									<div className="flex items-center gap-4 text-xs text-muted-foreground">
										<span className="flex items-center gap-1">
											<CalendarIcon className="h-3 w-3" />
											Started: {formatDate(incident.started_at)}
										</span>
										{incident.resolved_at && (
											<span className="flex items-center gap-1">
												<CheckCircle2 className="h-3 w-3" />
												Resolved: {formatDate(incident.resolved_at)}
											</span>
										)}
									</div>
									<div className="flex items-center gap-2">
										{incident.status === "open" && (
											<Button
												variant="outline"
												size="sm"
												onClick={() => acknowledgeMutation.mutate(incident.id)}
												disabled={acknowledgeMutation.isPending}
											>
												<Clock className="h-3 w-3 mr-1" />
												<Trans>Acknowledge</Trans>
											</Button>
										)}
										{(incident.status === "open" || incident.status === "acknowledged") && (
											<Button
												variant="outline"
												size="sm"
												onClick={() => resolveMutation.mutate(incident.id)}
												disabled={resolveMutation.isPending}
											>
												<Check className="h-3 w-3 mr-1" />
												<Trans>Resolve</Trans>
											</Button>
										)}
										<Button variant="ghost" size="sm" onClick={() => openDetail(incident)}>
											<Eye className="h-3 w-3 mr-1" />
											<Trans>Details</Trans>
										</Button>
									</div>
								</div>
							</CardContent>
						</Card>
					))}
				</div>
			)}

			{/* Incident Detail Dialog */}
			<Dialog open={isDetailOpen} onOpenChange={setIsDetailOpen}>
				<DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
					{selectedIncident && (
						<>
							<DialogHeader>
								<DialogTitle className="flex items-center gap-2">
									{selectedIncident.title}
									<SeverityBadge severity={selectedIncident.severity} />
									<StatusBadge status={selectedIncident.status} />
								</DialogTitle>
								<DialogDescription>
									{selectedIncident.description || "No description provided"}
								</DialogDescription>
							</DialogHeader>

							<div className="space-y-4 my-4">
								<div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
									<span className="flex items-center gap-1">
										<CalendarIcon className="h-4 w-4" />
										Started: {formatDate(selectedIncident.started_at)}
									</span>
									{selectedIncident.resolved_at && (
										<span className="flex items-center gap-1">
											<CheckCircle2 className="h-4 w-4" />
											Resolved: {formatDate(selectedIncident.resolved_at)}
										</span>
									)}
								</div>

								<Separator />

								{/* Updates Section */}
								<div>
									<h3 className="font-semibold mb-3 flex items-center gap-2">
										<MessageSquare className="h-4 w-4" />
										Updates ({incidentUpdates.length})
									</h3>

									{incidentUpdates.length > 0 ? (
										<div className="space-y-3">
											{incidentUpdates.map((update: IncidentUpdate) => (
												<Card key={update.id}>
													<CardContent className="p-3">
														<div className="flex items-start gap-2">
															<div className="flex-1">
																<p className="text-sm">{update.message}</p>
																<p className="text-xs text-muted-foreground mt-1">
																	{formatDate(update.created_at)}
																</p>
															</div>
														</div>
													</CardContent>
												</Card>
											))}
										</div>
									) : (
										<p className="text-sm text-muted-foreground italic">No updates yet</p>
									)}

									{/* Add Update Form */}
									{(selectedIncident.status === "open" || selectedIncident.status === "acknowledged") && (
										<form
											onSubmit={(e) => {
												e.preventDefault()
												const form = e.target as HTMLFormElement
												const message = (form.elements.namedItem("message") as HTMLInputElement).value
												if (message.trim()) {
													addUpdateMutation.mutate({ id: selectedIncident.id, message })
													form.reset()
												}
											}}
											className="mt-4 flex gap-2"
										>
											<Input
												name="message"
												placeholder="Add an update..."
												className="flex-1"
												disabled={addUpdateMutation.isPending}
											/>
											<Button type="submit" size="sm" disabled={addUpdateMutation.isPending}>
												<Trans>Add</Trans>
											</Button>
										</form>
									)}
								</div>
							</div>

							<DialogFooter className="gap-2">
								{selectedIncident.status !== "closed" && (
									<Button
										variant="outline"
										onClick={() => closeMutation.mutate(selectedIncident.id)}
										disabled={closeMutation.isPending}
									>
										<X className="h-4 w-4 mr-1" />
										<Trans>Close</Trans>
									</Button>
								)}
								<Button onClick={() => setIsDetailOpen(false)}>
									<Trans>Done</Trans>
								</Button>
							</DialogFooter>
						</>
					)}
				</DialogContent>
			</Dialog>
		</div>
	)
})

function CreateIncidentDialog({
	onSubmit,
	isLoading,
}: {
	onSubmit: (data: { title: string; description: string; severity: string; type: string }) => void
	isLoading: boolean
}) {
	const [title, setTitle] = useState("")
	const [description, setDescription] = useState("")
	const [severity, setSeverity] = useState("medium")
	const [type] = useState("manual")

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault()
		onSubmit({ title, description, severity, type })
	}

	return (
		<DialogContent>
			<DialogHeader>
				<DialogTitle>
					<Trans>Create New Incident</Trans>
				</DialogTitle>
				<DialogDescription>
					<Trans>Manually create an incident for tracking</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="space-y-4">
				<div className="space-y-2">
					<Label htmlFor="title">Title</Label>
					<Input
						id="title"
						value={title}
						onChange={(e) => setTitle(e.target.value)}
						placeholder="Incident title"
						required
					/>
				</div>
				<div className="space-y-2">
					<Label htmlFor="description">Description</Label>
					<Textarea
						id="description"
						value={description}
						onChange={(e) => setDescription(e.target.value)}
						placeholder="Describe the incident"
						rows={3}
					/>
				</div>
				<div className="space-y-2">
					<Label htmlFor="severity">Severity</Label>
					<Select value={severity} onValueChange={setSeverity}>
						<SelectTrigger>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="critical">Critical</SelectItem>
							<SelectItem value="high">High</SelectItem>
							<SelectItem value="medium">Medium</SelectItem>
							<SelectItem value="low">Low</SelectItem>
						</SelectContent>
					</Select>
				</div>
				<DialogFooter>
					<Button type="submit" disabled={isLoading || !title.trim()}>
						{isLoading ? <Trans>Creating...</Trans> : <Trans>Create Incident</Trans>}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
