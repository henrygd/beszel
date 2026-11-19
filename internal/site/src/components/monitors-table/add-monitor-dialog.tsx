import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState, useEffect } from "react"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { useToast } from "@/components/ui/use-toast"
import {
	createMonitor,
	updateMonitor,
	type Monitor,
	type MonitorType,
	type CreateMonitorRequest,
	type UpdateMonitorRequest,
} from "@/lib/monitors"
import { getNotifications, getMonitorNotifications, setMonitorNotifications } from "@/lib/notifications"

const MONITOR_TYPES: { value: MonitorType; label: string; group: string }[] = [
	{ value: "http", label: "HTTP", group: "General" },
	{ value: "https", label: "HTTPS", group: "General" },
	{ value: "keyword", label: "HTTP Keyword", group: "General" },
	{ value: "json-query", label: "HTTP JSON", group: "General" },
	{ value: "tcp", label: "TCP Port", group: "General" },
	{ value: "ping", label: "Ping", group: "General" },
	{ value: "dns", label: "DNS", group: "General" },
]

const HTTP_METHODS = ["GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH"]

const DNS_RECORD_TYPES = ["A", "AAAA", "CNAME", "MX", "NS", "TXT", "SRV"]

interface AddMonitorDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	monitor?: Monitor | null
	isEdit?: boolean
}

export function AddMonitorDialog({ open, onOpenChange, monitor, isEdit = false }: AddMonitorDialogProps) {
	const { t } = useLingui()
	const { toast } = useToast()
	const queryClient = useQueryClient()
	const [selectedNotificationIds, setSelectedNotificationIds] = useState<string[]>([])
	const { data: notificationProviders } = useQuery({
		queryKey: ["notifications"],
		queryFn: getNotifications,
	})
	useQuery({
		queryKey: ["monitor-notifications", monitor?.id],
		queryFn: () => getMonitorNotifications(monitor!.id),
		enabled: Boolean(monitor?.id),
		select: (ids) => setSelectedNotificationIds(ids),
	})
	const [activeTab, setActiveTab] = useState("basic")

	// Form state
	const [name, setName] = useState("")
	const [type, setType] = useState<MonitorType>("https")
	const [url, setUrl] = useState("")
	const [hostname, setHostname] = useState("")
	const [port, setPort] = useState<number | "">("")
	const [method, setMethod] = useState("GET")
	const [headers, setHeaders] = useState("")
	const [body, setBody] = useState("")
	const [interval, setInterval] = useState(60)
	const [timeout, setTimeout] = useState(30)
	const [retries, setRetries] = useState(1)
	const [keyword, setKeyword] = useState("")
	const [jsonQuery, setJsonQuery] = useState("")
	const [expectedValue, setExpectedValue] = useState("")
	const [invertKeyword, setInvertKeyword] = useState(false)
	const [dnsResolveServer, setDnsResolveServer] = useState("")
	const [dnsResolverMode, setDnsResolverMode] = useState("A")
	const [description, setDescription] = useState("")
	const [ignoreTLSError, setIgnoreTLSError] = useState(false)
	const [certExpiryNotification, setCertExpiryNotification] = useState(false)
	const [certExpiryDays, setCertExpiryDays] = useState(14)

	// Database / network fields
	const [dbConnectionString, setDbConnectionString] = useState("")
	const [dbUsername, setDbUsername] = useState("")
	const [dbPassword, setDbPassword] = useState("")
	const [dbName, setDbName] = useState("")
	const [mqttTopic, setMqttTopic] = useState("")
	const [grpcKeyword, setGrpcKeyword] = useState("")

	// Notification settings
	const [notifyOnDown, setNotifyOnDown] = useState(true)
	const [notifyOnRecover, setNotifyOnRecover] = useState(true)
	const [notifyOnResponseTime, setNotifyOnResponseTime] = useState(false)
	const [responseTimeThreshold, setResponseTimeThreshold] = useState(1000)
	const [notifyOnUptimeDrop, setNotifyOnUptimeDrop] = useState(false)
	const [uptimeThreshold, setUptimeThreshold] = useState(95)
	const [notifyRepeatedFailures, setNotifyRepeatedFailures] = useState(true)
	const [consecutiveFailures, setConsecutiveFailures] = useState(3)
	const [quietHoursStart, setQuietHoursStart] = useState("22:00")
	const [quietHoursEnd, setQuietHoursEnd] = useState("08:00")
	const [quietHoursEnabled, setQuietHoursEnabled] = useState(false)

	// Reset form when dialog opens/closes
	useEffect(() => {
		if (open) {
			if (isEdit && monitor) {
				// Populate form for editing
				setName(monitor.name)
				setType(monitor.type)
				setUrl(monitor.url || "")
				setHostname(monitor.hostname || "")
				setPort(monitor.port || "")
				setMethod(monitor.method || "GET")
				setHeaders("") // Parse from JSON if needed
				setBody("")
				setInterval(monitor.interval || 60)
				setTimeout(monitor.timeout || 30)
				setRetries(monitor.retries || 1)
				setKeyword(monitor.keyword || "")
				setJsonQuery(monitor.json_query || "")
				setExpectedValue(monitor.expected_value || "")
				setInvertKeyword(monitor.invert_keyword || false)
				setDnsResolveServer(monitor.dns_resolve_server || "")
				setDnsResolverMode(monitor.dns_resolver_mode || "A")
				setDescription(monitor.description || "")
				setIgnoreTLSError(monitor.ignore_tls_error || false)
				setCertExpiryNotification(monitor.cert_expiry_notification || false)
				setCertExpiryDays(monitor.cert_expiry_days || 14)

				// Load notification settings
				setNotifyOnDown(monitor.notify_on_down !== false)
				setNotifyOnRecover(monitor.notify_on_recover !== false)
				setNotifyOnResponseTime(monitor.notify_on_response_time || false)
				setResponseTimeThreshold(monitor.response_time_threshold || 1000)
				setNotifyOnUptimeDrop(monitor.notify_on_uptime_drop || false)
				setUptimeThreshold(monitor.uptime_threshold || 95)
				setNotifyRepeatedFailures(monitor.notify_repeated_failures !== false)
				setConsecutiveFailures(monitor.consecutive_failures || 3)
				setQuietHoursStart(monitor.quiet_hours_start || "22:00")
				setQuietHoursEnd(monitor.quiet_hours_end || "08:00")
				setQuietHoursEnabled(monitor.quiet_hours_enabled || false)
			} else {
				// Reset to defaults for new monitor
				setName("")
				setType("https")
				setUrl("")
				setHostname("")
				setPort("")
				setMethod("GET")
				setHeaders("")
				setBody("")
				setInterval(60)
				setTimeout(30)
				setRetries(1)
				setKeyword("")
				setJsonQuery("")
				setExpectedValue("")
				setInvertKeyword(false)
				setDnsResolveServer("")
				setDnsResolverMode("A")
				setDescription("")
				setIgnoreTLSError(false)
				setCertExpiryNotification(false)
				setCertExpiryDays(14)

				// Reset notification settings
				setNotifyOnDown(true)
				setNotifyOnRecover(true)
				setNotifyOnResponseTime(false)
				setResponseTimeThreshold(1000)
				setNotifyOnUptimeDrop(false)
				setUptimeThreshold(95)
				setNotifyRepeatedFailures(true)
				setConsecutiveFailures(3)
				setQuietHoursStart("22:00")
				setQuietHoursEnd("08:00")
				setQuietHoursEnabled(false)
			}
			setActiveTab("basic")
		}
	}, [open, isEdit, monitor])

	const linkNotifications = (monitorId: string) => {
		setMonitorNotifications(monitorId, selectedNotificationIds).catch(() => {})
	}
	const createMutation = useMutation({
		mutationFn: createMonitor,
		onSuccess: (created) => {
			if (created?.id) linkNotifications(created.id)
			toast({ title: t`Monitor created successfully` })
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
			onOpenChange(false)
		},
		onError: (error) => {
			toast({
				title: t`Failed to create monitor`,
				description: error instanceof Error ? error.message : "Unknown error",
				variant: "destructive",
			})
		},
	})

	const updateMutation = useMutation({
		mutationFn: ({ id, data }: { id: string; data: UpdateMonitorRequest }) => updateMonitor(id, data),
		onSuccess: (_res, vars) => {
			linkNotifications(vars.id)
			toast({ title: t`Monitor updated successfully` })
			queryClient.invalidateQueries({ queryKey: ["monitors"] })
			onOpenChange(false)
		},
		onError: (error) => {
			toast({
				title: t`Failed to update monitor`,
				description: error instanceof Error ? error.message : "Unknown error",
				variant: "destructive",
			})
		},
	})

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault()

		if (!name.trim()) {
			toast({ title: t`Name is required`, variant: "destructive" })
			return
		}

		if (isEdit && monitor) {
			const data: UpdateMonitorRequest = {
				name: name.trim(),
				url: needsDbOptions ? dbConnectionString.trim() || undefined : url.trim() || undefined,
				hostname: needsHostname ? hostname.trim() || undefined : undefined,
				port: port ? Number(port) : undefined,
				method: ["http", "https", "keyword", "json-query"].includes(type) ? method : undefined,
				headers: headers.trim() || undefined,
				body: body.trim() || undefined,
				interval,
				timeout,
				retries,
				keyword: type === "keyword" ? keyword.trim() : undefined,
				json_query: type === "json-query" ? jsonQuery.trim() : undefined,
				expected_value: type === "json-query" ? expectedValue.trim() : undefined,
				invert_keyword: type === "keyword" ? invertKeyword : undefined,
				dns_resolve_server: type === "dns" ? dnsResolveServer.trim() : undefined,
				dns_resolver_mode: type === "dns" ? dnsResolverMode : undefined,
				description: description.trim() || undefined,
				ignore_tls_error: type === "https" || type === "keyword" || type === "json-query" ? ignoreTLSError : undefined,
				cert_expiry_notification: type === "https" ? certExpiryNotification : undefined,
				cert_expiry_days: type === "https" ? certExpiryDays : undefined,
				// Notification settings
				notify_on_down: notifyOnDown,
				notify_on_recover: notifyOnRecover,
				notify_on_response_time: notifyOnResponseTime,
				response_time_threshold: notifyOnResponseTime ? responseTimeThreshold : undefined,
				notify_on_uptime_drop: notifyOnUptimeDrop,
				uptime_threshold: notifyOnUptimeDrop ? uptimeThreshold : undefined,
				notify_repeated_failures: notifyRepeatedFailures,
				consecutive_failures: consecutiveFailures,
				quiet_hours_enabled: quietHoursEnabled,
				quiet_hours_start: quietHoursEnabled ? quietHoursStart : undefined,
				quiet_hours_end: quietHoursEnabled ? quietHoursEnd : undefined,
				// Database / network extra fields
				db_username: needsDbOptions ? dbUsername.trim() || undefined : undefined,
				db_password: needsDbOptions ? dbPassword.trim() || undefined : undefined,
				db_name: needsDbOptions ? dbName.trim() || undefined : undefined,
				mqtt_topic: needsMqttOptions ? mqttTopic.trim() || undefined : undefined,
				grpc_keyword: needsGrpcOptions ? grpcKeyword.trim() || undefined : undefined,
			}
			updateMutation.mutate({ id: monitor.id, data })
		} else {
			const data: CreateMonitorRequest = {
				name: name.trim(),
				type,
				url: needsDbOptions ? dbConnectionString.trim() || undefined : url.trim() || undefined,
				hostname: needsHostname ? hostname.trim() || undefined : undefined,
				port: port ? Number(port) : undefined,
				method: ["http", "https", "keyword", "json-query"].includes(type) ? method : undefined,
				headers: headers.trim() || undefined,
				body: body.trim() || undefined,
				interval,
				timeout,
				retries,
				keyword: type === "keyword" ? keyword.trim() : undefined,
				json_query: type === "json-query" ? jsonQuery.trim() : undefined,
				expected_value: type === "json-query" ? expectedValue.trim() : undefined,
				invert_keyword: type === "keyword" ? invertKeyword : undefined,
				dns_resolve_server: type === "dns" ? dnsResolveServer.trim() : undefined,
				dns_resolver_mode: type === "dns" ? dnsResolverMode : undefined,
				description: description.trim() || undefined,
				ignore_tls_error: type === "https" || type === "keyword" || type === "json-query" ? ignoreTLSError : undefined,
				cert_expiry_notification: type === "https" ? certExpiryNotification : undefined,
				cert_expiry_days: type === "https" ? certExpiryDays : undefined,
				// Notification settings
				notify_on_down: notifyOnDown,
				notify_on_recover: notifyOnRecover,
				notify_on_response_time: notifyOnResponseTime,
				response_time_threshold: notifyOnResponseTime ? responseTimeThreshold : undefined,
				notify_on_uptime_drop: notifyOnUptimeDrop,
				uptime_threshold: notifyOnUptimeDrop ? uptimeThreshold : undefined,
				notify_repeated_failures: notifyRepeatedFailures,
				consecutive_failures: consecutiveFailures,
				quiet_hours_enabled: quietHoursEnabled,
				quiet_hours_start: quietHoursEnabled ? quietHoursStart : undefined,
				quiet_hours_end: quietHoursEnabled ? quietHoursEnd : undefined,
				// Database / network extra fields
				db_username: needsDbOptions ? dbUsername.trim() || undefined : undefined,
				db_password: needsDbOptions ? dbPassword.trim() || undefined : undefined,
				db_name: needsDbOptions ? dbName.trim() || undefined : undefined,
				mqtt_topic: needsMqttOptions ? mqttTopic.trim() || undefined : undefined,
				grpc_keyword: needsGrpcOptions ? grpcKeyword.trim() || undefined : undefined,
			}
			createMutation.mutate(data)
		}
	}

	const needsUrl = [
		"http",
		"https",
		"keyword",
		"json-query",
		"grpc-keyword",
		"real-browser",
		"websocket-upgrade",
		"push",
	].includes(type)
	const needsHostname = [
		"tcp",
		"ping",
		"dns",
		"mqtt",
		"rabbitmq",
		"kafka-producer",
		"smtp",
		"snmp",
		"sip-options",
		"tailscale-ping",
		"globalping",
		"mysql",
		"postgresql",
		"mongodb",
		"redis",
		"sqlserver",
		"oracledb",
		"radius",
		"gamedig",
		"steam",
	].includes(type)
	const needsPort = [
		"tcp",
		"smtp",
		"mysql",
		"postgresql",
		"redis",
		"sqlserver",
		"oracledb",
		"radius",
		"mqtt",
		"rabbitmq",
		"kafka-producer",
		"gamedig",
		"steam",
		"snmp",
	].includes(type)
	const needsHttpOptions = ["http", "https", "keyword", "json-query"].includes(type)
	const needsKeyword = type === "keyword"
	const needsJsonQuery = type === "json-query"
	const needsDnsOptions = type === "dns"
	const needsTlsOptions = type === "https"
	const needsDbOptions = ["mysql", "postgresql", "mongodb", "redis", "sqlserver", "oracledb", "radius"].includes(type)
	const needsMqttOptions = type === "mqtt"
	const needsGrpcOptions = type === "grpc-keyword"

	const isPending = createMutation.isPending || updateMutation.isPending

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle>{isEdit ? <Trans>Edit Monitor</Trans> : <Trans>Add Monitor</Trans>}</DialogTitle>
					<DialogDescription>
						<Trans>Configure a monitor to track website or service availability.</Trans>
					</DialogDescription>
				</DialogHeader>

				<form onSubmit={handleSubmit}>
					<Tabs value={activeTab} onValueChange={setActiveTab} className="mt-4">
						<TabsList className="grid w-full grid-cols-3">
							<TabsTrigger value="basic">
								<Trans>Basic</Trans>
							</TabsTrigger>
							<TabsTrigger value="advanced">
								<Trans>Advanced</Trans>
							</TabsTrigger>
							<TabsTrigger value="notifications">
								<Trans>Notifications</Trans>
							</TabsTrigger>
						</TabsList>

						<TabsContent value="basic" className="space-y-4 mt-4">
							<div className="grid gap-4">
								<div className="grid gap-2">
									<Label htmlFor="name">
										<Trans>Monitor Name</Trans> *
									</Label>
									<Input
										id="name"
										placeholder={t`e.g., My Website`}
										value={name}
										onChange={(e) => setName(e.target.value)}
										required
										tabIndex={0}
										autoFocus
									/>
								</div>

								<div className="grid gap-2">
									<Label htmlFor="type">
										<Trans>Monitor Type</Trans> *
									</Label>
									<Select value={type} onValueChange={(v) => setType(v as MonitorType)}>
										<SelectTrigger id="type">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{["General", "Network / Protocol", "Database", "Game Server"].map((group) => (
												<SelectGroup key={group}>
													<SelectLabel>{group}</SelectLabel>
													{MONITOR_TYPES.filter((mt) => mt.group === group).map((mt) => (
														<SelectItem key={mt.value} value={mt.value}>
															{mt.label}
														</SelectItem>
													))}
												</SelectGroup>
											))}
										</SelectContent>
									</Select>
								</div>

								{needsUrl && (
									<div className="grid gap-2">
										<Label htmlFor="url">
											<Trans>URL</Trans> *
										</Label>
										<Input
											id="url"
											placeholder={t`https://example.com`}
											value={url}
											onChange={(e) => setUrl(e.target.value)}
											required
										/>
									</div>
								)}

								{needsHostname && (
									<div className="grid gap-2">
										<Label htmlFor="hostname">
											<Trans>Hostname</Trans> *
										</Label>
										<Input
											id="hostname"
											placeholder={t`example.com`}
											value={hostname}
											onChange={(e) => setHostname(e.target.value)}
											required
										/>
									</div>
								)}

								{needsPort && (
									<div className="grid gap-2">
										<Label htmlFor="port">
											<Trans>Port</Trans> *
										</Label>
										<Input
											id="port"
											type="number"
											placeholder={t`443`}
											value={port}
											onChange={(e) => setPort(e.target.value ? Number(e.target.value) : "")}
											required
										/>
									</div>
								)}

								{needsHttpOptions && (
									<div className="grid gap-2">
										<Label htmlFor="method">
											<Trans>HTTP Method</Trans>
										</Label>
										<Select value={method} onValueChange={setMethod}>
											<SelectTrigger id="method">
												<SelectValue />
											</SelectTrigger>
											<SelectContent>
												{HTTP_METHODS.map((m) => (
													<SelectItem key={m} value={m}>
														{m}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
									</div>
								)}

								{needsKeyword && (
									<div className="grid gap-2">
										<Label htmlFor="keyword">
											<Trans>Keyword to Search</Trans> *
										</Label>
										<Input
											id="keyword"
											placeholder={t`Success`}
											value={keyword}
											onChange={(e) => setKeyword(e.target.value)}
											required
										/>
										<div className="flex items-center gap-2 mt-2">
											<Switch id="invertKeyword" checked={invertKeyword} onCheckedChange={setInvertKeyword} />
											<Label htmlFor="invertKeyword">
												<Trans>Invert match (alert if keyword found)</Trans>
											</Label>
										</div>
									</div>
								)}

								{needsJsonQuery && (
									<div className="space-y-4">
										<div className="grid gap-2">
											<Label htmlFor="jsonQuery">
												<Trans>JSON Path</Trans> *
											</Label>
											<Input
												id="jsonQuery"
												placeholder={t`data.status`}
												value={jsonQuery}
												onChange={(e) => setJsonQuery(e.target.value)}
												required
											/>
										</div>
										<div className="grid gap-2">
											<Label htmlFor="expectedValue">
												<Trans>Expected Value</Trans>
											</Label>
											<Input
												id="expectedValue"
												placeholder={t`active`}
												value={expectedValue}
												onChange={(e) => setExpectedValue(e.target.value)}
											/>
										</div>
									</div>
								)}

								{needsDnsOptions && (
									<div className="space-y-4">
										<div className="grid gap-2">
											<Label htmlFor="dnsResolverMode">
												<Trans>Record Type</Trans>
											</Label>
											<Select value={dnsResolverMode} onValueChange={setDnsResolverMode}>
												<SelectTrigger id="dnsResolverMode">
													<SelectValue />
												</SelectTrigger>
												<SelectContent>
													{DNS_RECORD_TYPES.map((t) => (
														<SelectItem key={t} value={t}>
															{t}
														</SelectItem>
													))}
												</SelectContent>
											</Select>
										</div>
										<div className="grid gap-2">
											<Label htmlFor="dnsResolveServer">
												<Trans>DNS Server (optional)</Trans>
											</Label>
											<Input
												id="dnsResolveServer"
												placeholder={t`8.8.8.8`}
												value={dnsResolveServer}
												onChange={(e) => setDnsResolveServer(e.target.value)}
											/>
										</div>
									</div>
								)}

								{needsDbOptions && (
									<div className="space-y-4 border rounded-lg p-4">
										<h4 className="font-medium text-sm">
											<Trans>Database Connection</Trans>
										</h4>
										<div className="grid gap-2">
											<Label htmlFor="dbConnectionString">
												<Trans>Host / Connection String</Trans> *
											</Label>
											<Input
												id="dbConnectionString"
												placeholder={t`localhost:3306`}
												value={dbConnectionString}
												onChange={(e) => setDbConnectionString(e.target.value)}
												required={needsDbOptions}
											/>
										</div>
										<div className="grid grid-cols-2 gap-4">
											<div className="grid gap-2">
												<Label htmlFor="dbUsername">
													<Trans>Username</Trans>
												</Label>
												<Input
													id="dbUsername"
													placeholder={t`root`}
													value={dbUsername}
													onChange={(e) => setDbUsername(e.target.value)}
												/>
											</div>
											<div className="grid gap-2">
												<Label htmlFor="dbPassword">
													<Trans>Password</Trans>
												</Label>
												<Input
													id="dbPassword"
													type="password"
													placeholder={t`password`}
													value={dbPassword}
													onChange={(e) => setDbPassword(e.target.value)}
												/>
											</div>
										</div>
										<div className="grid gap-2">
											<Label htmlFor="dbName">
												<Trans>Database Name</Trans>
											</Label>
											<Input
												id="dbName"
												placeholder={t`mydb`}
												value={dbName}
												onChange={(e) => setDbName(e.target.value)}
											/>
										</div>
									</div>
								)}

								{needsMqttOptions && (
									<div className="grid gap-2">
										<Label htmlFor="mqttTopic">
											<Trans>MQTT Topic</Trans>
										</Label>
										<Input
											id="mqttTopic"
											placeholder={t`sensor/temperature`}
											value={mqttTopic}
											onChange={(e) => setMqttTopic(e.target.value)}
										/>
									</div>
								)}

								{needsGrpcOptions && (
									<div className="grid gap-2">
										<Label htmlFor="grpcKeyword">
											<Trans>gRPC Keyword</Trans>
										</Label>
										<Input
											id="grpcKeyword"
											placeholder={t`health`}
											value={grpcKeyword}
											onChange={(e) => setGrpcKeyword(e.target.value)}
										/>
									</div>
								)}

								<div className="grid gap-2">
									<Label htmlFor="description">
										<Trans>Description</Trans>
									</Label>
									<Textarea
										id="description"
										placeholder={t`Optional description for this monitor`}
										value={description}
										onChange={(e) => setDescription(e.target.value)}
										rows={2}
									/>
								</div>
							</div>
						</TabsContent>

						<TabsContent value="advanced" className="space-y-4 mt-4">
							<div className="grid grid-cols-3 gap-4">
								<div className="grid gap-2">
									<Label htmlFor="interval">
										<Trans>Interval (seconds)</Trans>
									</Label>
									<Input
										id="interval"
										type="number"
										min={20}
										max={86400}
										value={interval}
										onChange={(e) => setInterval(Number(e.target.value))}
									/>
								</div>
								<div className="grid gap-2">
									<Label htmlFor="timeout">
										<Trans>Timeout (seconds)</Trans>
									</Label>
									<Input
										id="timeout"
										type="number"
										min={1}
										max={300}
										value={timeout}
										onChange={(e) => setTimeout(Number(e.target.value))}
									/>
								</div>
								<div className="grid gap-2">
									<Label htmlFor="retries">
										<Trans>Retries</Trans>
									</Label>
									<Input
										id="retries"
										type="number"
										min={0}
										max={10}
										value={retries}
										onChange={(e) => setRetries(Number(e.target.value))}
									/>
								</div>
							</div>

							{needsHttpOptions && (
								<div className="space-y-4">
									<div className="grid gap-2">
										<Label htmlFor="headers">
											<Trans>Headers (JSON)</Trans>
										</Label>
										<Textarea
											id="headers"
											placeholder={`{\n  "Authorization": "Bearer token"\n}`}
											value={headers}
											onChange={(e) => setHeaders(e.target.value)}
											rows={3}
										/>
									</div>
									<div className="grid gap-2">
										<Label htmlFor="body">
											<Trans>Body</Trans>
										</Label>
										<Textarea
											id="body"
											placeholder={t`Request body for POST/PUT requests`}
											value={body}
											onChange={(e) => setBody(e.target.value)}
											rows={3}
										/>
									</div>
								</div>
							)}

							{needsTlsOptions && (
								<div className="space-y-4 border rounded-lg p-4">
									<div className="flex items-center gap-2">
										<Switch id="ignoreTLSError" checked={ignoreTLSError} onCheckedChange={setIgnoreTLSError} />
										<Label htmlFor="ignoreTLSError">
											<Trans>Ignore TLS/SSL errors</Trans>
										</Label>
									</div>
								</div>
							)}
						</TabsContent>

						<TabsContent value="notifications" className="space-y-4 mt-4">
							{/* Status Change Notifications */}
							<div className="space-y-4 border rounded-lg p-4">
								<h4 className="font-medium text-sm">Status Change Alerts</h4>

								<div className="flex items-center justify-between">
									<div className="space-y-0.5">
										<Label htmlFor="notifyOnDown">Notify when monitor goes down</Label>
										<p className="text-xs text-muted-foreground">Send alert when service becomes unavailable</p>
									</div>
									<Switch id="notifyOnDown" checked={notifyOnDown} onCheckedChange={setNotifyOnDown} />
								</div>

								<div className="flex items-center justify-between">
									<div className="space-y-0.5">
										<Label htmlFor="notifyOnRecover">Notify when monitor recovers</Label>
										<p className="text-xs text-muted-foreground">Send alert when service comes back up</p>
									</div>
									<Switch id="notifyOnRecover" checked={notifyOnRecover} onCheckedChange={setNotifyOnRecover} />
								</div>

								<div className="space-y-3">
									<div className="flex items-center justify-between">
										<div className="space-y-0.5">
											<Label htmlFor="notifyRepeatedFailures">Repeated failures only</Label>
											<p className="text-xs text-muted-foreground">Only alert after multiple consecutive failures</p>
										</div>
										<Switch
											id="notifyRepeatedFailures"
											checked={notifyRepeatedFailures}
											onCheckedChange={setNotifyRepeatedFailures}
										/>
									</div>
									{notifyRepeatedFailures && (
										<div className="grid gap-2 pl-4 border-l-2">
											<Label htmlFor="consecutiveFailures">Consecutive failures before alert</Label>
											<Input
												id="consecutiveFailures"
												type="number"
												min={1}
												max={10}
												value={consecutiveFailures}
												onChange={(e) => setConsecutiveFailures(Number(e.target.value))}
												className="w-32"
											/>
										</div>
									)}
								</div>
							</div>

							{/* Performance Alerts */}
							<div className="space-y-4 border rounded-lg p-4">
								<h4 className="font-medium text-sm">Performance Alerts</h4>

								<div className="space-y-3">
									<div className="flex items-center justify-between">
										<div className="space-y-0.5">
											<Label htmlFor="notifyOnResponseTime">Response time threshold</Label>
											<p className="text-xs text-muted-foreground">Alert when response time exceeds limit</p>
										</div>
										<Switch
											id="notifyOnResponseTime"
											checked={notifyOnResponseTime}
											onCheckedChange={setNotifyOnResponseTime}
										/>
									</div>
									{notifyOnResponseTime && (
										<div className="grid gap-2 pl-4 border-l-2">
											<Label htmlFor="responseTimeThreshold">Max response time (ms)</Label>
											<Input
												id="responseTimeThreshold"
												type="number"
												min={100}
												max={60000}
												step={100}
												value={responseTimeThreshold}
												onChange={(e) => setResponseTimeThreshold(Number(e.target.value))}
												className="w-32"
											/>
										</div>
									)}
								</div>

								<div className="space-y-3">
									<div className="flex items-center justify-between">
										<div className="space-y-0.5">
											<Label htmlFor="notifyOnUptimeDrop">Uptime threshold</Label>
											<p className="text-xs text-muted-foreground">Alert when uptime percentage drops below</p>
										</div>
										<Switch
											id="notifyOnUptimeDrop"
											checked={notifyOnUptimeDrop}
											onCheckedChange={setNotifyOnUptimeDrop}
										/>
									</div>
									{notifyOnUptimeDrop && (
										<div className="grid gap-2 pl-4 border-l-2">
											<Label htmlFor="uptimeThreshold">Minimum uptime (%)</Label>
											<Input
												id="uptimeThreshold"
												type="number"
												min={1}
												max={100}
												value={uptimeThreshold}
												onChange={(e) => setUptimeThreshold(Number(e.target.value))}
												className="w-32"
											/>
										</div>
									)}
								</div>
							</div>

							{/* Certificate Expiry */}
							{needsTlsOptions && (
								<div className="space-y-4 border rounded-lg p-4">
									<h4 className="font-medium text-sm">Certificate Alerts</h4>
									<div className="flex items-center justify-between">
										<div className="space-y-0.5">
											<Label htmlFor="certExpiryNotification">Notify when certificate expires</Label>
											<p className="text-xs text-muted-foreground">Alert before SSL certificate expiry</p>
										</div>
										<Switch
											id="certExpiryNotification"
											checked={certExpiryNotification}
											onCheckedChange={setCertExpiryNotification}
										/>
									</div>
									{certExpiryNotification && (
										<div className="grid gap-2 pl-4 border-l-2">
											<Label htmlFor="certExpiryDays">Days before expiry to notify</Label>
											<Input
												id="certExpiryDays"
												type="number"
												min={1}
												max={90}
												value={certExpiryDays}
												onChange={(e) => setCertExpiryDays(Number(e.target.value))}
												className="w-32"
											/>
										</div>
									)}
								</div>
							)}

							{/* Provider Selection */}
							{notificationProviders && notificationProviders.length > 0 && (
								<div className="space-y-3 border rounded-lg p-4">
									<h4 className="font-medium text-sm">Notification Providers</h4>
									<p className="text-xs text-muted-foreground">
										Choose providers for this monitor, or leave empty to use your default providers.
									</p>
									<div className="space-y-2">
										{notificationProviders.map((n) => (
											<label key={n.id} className="flex items-center gap-2 text-sm cursor-pointer">
												<input
													type="checkbox"
													className="h-4 w-4"
													checked={selectedNotificationIds.includes(n.id)}
													onChange={(e) =>
														setSelectedNotificationIds((prev) =>
															e.target.checked ? [...prev, n.id] : prev.filter((id) => id !== n.id)
														)
													}
												/>
												{n.name}
												<span className="text-xs text-muted-foreground">({n.type})</span>
											</label>
										))}
									</div>
								</div>
							)}

							{/* Quiet Hours */}
							<div className="space-y-4 border rounded-lg p-4">
								<h4 className="font-medium text-sm">Quiet Hours</h4>
								<div className="flex items-center justify-between">
									<div className="space-y-0.5">
										<Label htmlFor="quietHoursEnabled">Enable quiet hours</Label>
										<p className="text-xs text-muted-foreground">Suppress notifications during specific hours</p>
									</div>
									<Switch id="quietHoursEnabled" checked={quietHoursEnabled} onCheckedChange={setQuietHoursEnabled} />
								</div>
								{quietHoursEnabled && (
									<div className="grid grid-cols-2 gap-4 pl-4 border-l-2">
										<div className="grid gap-2">
											<Label htmlFor="quietHoursStart">Start time</Label>
											<Input
												id="quietHoursStart"
												type="time"
												value={quietHoursStart}
												onChange={(e) => setQuietHoursStart(e.target.value)}
											/>
										</div>
										<div className="grid gap-2">
											<Label htmlFor="quietHoursEnd">End time</Label>
											<Input
												id="quietHoursEnd"
												type="time"
												value={quietHoursEnd}
												onChange={(e) => setQuietHoursEnd(e.target.value)}
											/>
										</div>
									</div>
								)}
							</div>
						</TabsContent>
					</Tabs>

					<DialogFooter className="mt-6">
						<Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isPending}>
							<Trans>Cancel</Trans>
						</Button>
						<Button type="submit" disabled={isPending}>
							{isPending ? (
								<Trans>Saving...</Trans>
							) : isEdit ? (
								<Trans>Update Monitor</Trans>
							) : (
								<Trans>Create Monitor</Trans>
							)}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
