import { useLingui } from "@lingui/react/macro"
import { useEffect, useState } from "react"
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
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { isReadOnlyUser, pb } from "@/lib/api"
import type { MonitorRecord, MonitorType } from "@/types"

const TYPES: { value: MonitorType; label: string; placeholder: string }[] = [
	{ value: "http", label: "HTTP(S)", placeholder: "https://example.com" },
	{ value: "keyword", label: "Keyword", placeholder: "https://example.com" },
	{ value: "ping", label: "Ping", placeholder: "example.com" },
	{ value: "dns", label: "DNS", placeholder: "example.com" },
	{ value: "tls", label: "TLS cert", placeholder: "example.com:443" },
]

interface FormState {
	name: string
	type: MonitorType
	target: string
	interval: string
	timeout: string
	maxRetries: string
	resendAfter: string
	keyword: string
	invertKeyword: boolean
	method: string
	acceptedCodes: string
	authType: string
	authUser: string
	authSecret: string
	ignoreTls: boolean
	checkCert: boolean
	warnDays: string
	critDays: string
	qtype: string
	resolver: string
	protocol: string
	expectedAnswer: string
	count: string
	packetSize: string
	port: string
	upsideDown: boolean
	notify: boolean
}

function str(v: unknown, def: string): string {
	if (v === undefined || v === null) {
		return def
	}
	return String(v)
}

function initialState(monitor?: MonitorRecord): FormState {
	const cfg = (monitor?.config ?? {}) as Record<string, unknown>
	return {
		name: monitor?.name ?? "",
		type: monitor?.type ?? "http",
		target: monitor?.target ?? "",
		interval: String(monitor?.interval ?? 60),
		timeout: String(monitor?.timeout ?? 10),
		maxRetries: String(monitor?.max_retries ?? 2),
		resendAfter: String(monitor?.resend_after ?? 0),
		keyword: str(cfg.keyword, ""),
		invertKeyword: Boolean(cfg.invert_keyword ?? false),
		method: str(cfg.method, "GET"),
		acceptedCodes: str(cfg.accepted_status_codes, "200-299"),
		authType: str(cfg.auth_type, "none"),
		authUser: str(cfg.username, ""),
		authSecret: "",
		ignoreTls: Boolean(cfg.ignore_tls_errors ?? false),
		checkCert: cfg.check_cert_expiry === undefined ? true : Boolean(cfg.check_cert_expiry),
		warnDays: str(cfg.warn_days, "21"),
		critDays: str(cfg.crit_days, "7"),
		qtype: str(cfg.qtype, "A"),
		resolver: str(cfg.resolver, ""),
		protocol: str(cfg.protocol, "udp"),
		expectedAnswer: str(cfg.expected_answer, ""),
		count: str(cfg.count, "3"),
		packetSize: str(cfg.packet_size, "56"),
		port: str(cfg.port, ""),
		upsideDown: Boolean(monitor?.upside_down ?? false),
		notify: monitor?.notify ?? true,
	}
}

export function MonitorDialog({
	open,
	setOpen,
	monitor,
	onSaved,
}: {
	open: boolean
	setOpen: (open: boolean) => void
	monitor?: MonitorRecord
	onSaved: () => void
}) {
	const { t } = useLingui()
	const [form, setForm] = useState<FormState>(() => initialState(monitor))
	const [error, setError] = useState("")
	const [saving, setSaving] = useState(false)

	const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
		setForm((prev) => ({ ...prev, [key]: value }))
	}

	// Refresh defaults when switching between monitors.
	useEffect(() => {
		setForm(initialState(monitor))
		setError("")
	}, [monitor?.id])

	const save = async () => {
		setError("")
		const interval = Number.parseInt(form.interval, 10)
		const timeout = Number.parseInt(form.timeout, 10)
		if (!form.name.trim()) {
			setError(t`Name is required.`)
			return
		}
		if (!form.target.trim()) {
			setError(t`Target is required.`)
			return
		}
		if (!(interval >= 20)) {
			setError(t`Interval must be at least 20 seconds.`)
			return
		}
		if (!(timeout > 0 && timeout < interval)) {
			setError(t`Timeout must be positive and less than the interval.`)
			return
		}
	// Merge form keys over the stored config so settings made via API or
		// YAML survive a UI save. Secrets are never shown here; the API keeps
		// stored secrets when they are absent from the patch.
		const config: Record<string, unknown> = { ...(monitor?.config ?? {}) }
		const num = (v: string) => Number.parseInt(v, 10)
		// Drop keys that belong to other monitor types so switching type
		// cannot leave behind invisible behavior (e.g. a keyword check on
		// an http monitor, or a qtype on a ping monitor).
		for (const key of [
			"keyword",
			"invert_keyword",
			"method",
			"accepted_status_codes",
			"auth_type",
			"username",
			"password",
			"token",
			"ignore_tls_errors",
			"check_cert_expiry",
			"warn_days",
			"crit_days",
			"qtype",
			"protocol",
			"resolver",
			"expected_answer",
			"count",
			"packet_size",
			"port",
		]) {
			delete config[key]
		}
		if (form.type === "keyword" || (form.type === "http" && form.keyword.trim())) {
			if (form.type === "keyword" && !form.keyword) {
				setError(t`Keyword is required for keyword monitors.`)
				return
			}
			config.keyword = form.keyword
			config.invert_keyword = form.invertKeyword
		} else {
			delete config.keyword
			delete config.invert_keyword
		}
		if (form.type === "http" || form.type === "keyword" || form.type === "tls") {
			config.method = form.method || "GET"
			if (form.acceptedCodes.trim()) {
				config.accepted_status_codes = form.acceptedCodes.trim()
			}
			config.auth_type = form.authType
			if (form.authType === "basic") {
				config.username = form.authUser.trim()
				if (form.authSecret) {
					config.password = form.authSecret
				}
			} else {
				delete config.username
			}
			if (form.authType === "bearer") {
				if (form.authSecret) {
					config.token = form.authSecret
				}
			} else {
				delete config.token
			}
			if (form.authType === "none") {
				delete config.password
			}
			config.ignore_tls_errors = form.ignoreTls
		}
		if (form.type === "http" || form.type === "keyword" || form.type === "tls") {
			config.check_cert_expiry = form.checkCert
			const warn = num(form.warnDays)
			const crit = num(form.critDays)
			if (form.checkCert && (!(warn > 0) || !(crit > 0) || !(crit < warn))) {
				setError(t`Cert thresholds must be positive with critical below warning.`)
				return
			}
			config.warn_days = warn
			config.crit_days = crit
		}
		if (form.type === "dns") {
			config.qtype = form.qtype || "A"
			config.protocol = form.protocol || "udp"
			if (form.resolver.trim()) {
				config.resolver = form.resolver.trim()
			} else {
				delete config.resolver
			}
			if (form.expectedAnswer.trim()) {
				config.expected_answer = form.expectedAnswer.trim()
			} else {
				delete config.expected_answer
			}
		}
		if (form.type === "ping") {
			const count = num(form.count)
			const size = num(form.packetSize)
			if (!(count >= 1 && count <= 10)) {
				setError(t`Ping count must be between 1 and 10.`)
				return
			}
			if (!(size >= 0 && size <= 65400)) {
				setError(t`Packet size must be between 0 and 65400.`)
				return
			}
			config.count = count
			config.packet_size = size
		}
		if (form.type === "tls" && form.port.trim()) {
			const port = Number.parseInt(form.port, 10)
			if (!(port > 0 && port <= 65535)) {
				setError(t`Port must be between 1 and 65535.`)
				return
			}
			config.port = port
		} else {
			delete config.port
		}
		const maxRetries = num(form.maxRetries)
		const resendAfter = num(form.resendAfter)
		if (!(maxRetries >= 0 && maxRetries <= 10)) {
			setError(t`Retries must be between 0 and 10.`)
			return
		}
		if (!(resendAfter >= 0 && resendAfter <= 1440)) {
			setError(t`Resend delay must be between 0 and 1440 minutes.`)
			return
		}
		setSaving(true)
		try {
			const body = {
				name: form.name.trim(),
				type: form.type,
				target: form.target.trim(),
				interval,
				timeout,
				max_retries: maxRetries,
				resend_after: resendAfter,
				upside_down: form.upsideDown,
				notify: form.notify,
				config,
			}
			if (monitor) {
				await pb.collection("monitors").update(monitor.id, body)
			} else {
				await pb.collection("monitors").create(body)
			}
			onSaved()
		} catch (e) {
			setError(e instanceof Error ? e.message : String(e))
		} finally {
			setSaving(false)
		}
	}

	const activeType = TYPES.find((x) => x.value === form.type) ?? TYPES[0]

	if (isReadOnlyUser()) {
		return null
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>{monitor ? t`Edit monitor` : t`Add monitor`}</DialogTitle>
					<DialogDescription>{t`External uptime check executed from the hub.`}</DialogDescription>
				</DialogHeader>
				<div className="grid gap-4">
					<div className="grid gap-2">
						<Label htmlFor="mon-name">{t`Name`}</Label>
						<Input
							id="mon-name"
							value={form.name}
							onChange={(e) => set("name", e.target.value)}
							placeholder="Homepage"
						/>
					</div>
					<div className="grid grid-cols-2 gap-4">
						<div className="grid gap-2">
							<Label htmlFor="mon-type">{t`Type`}</Label>
							<Select value={form.type} onValueChange={(v) => set("type", v as MonitorType)}>
								<SelectTrigger id="mon-type">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{TYPES.map((x) => (
										<SelectItem key={x.value} value={x.value}>
											{x.label}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="mon-target">{t`Target`}</Label>
							<Input
								id="mon-target"
								value={form.target}
								onChange={(e) => set("target", e.target.value)}
								placeholder={activeType.placeholder}
							/>
						</div>
					</div>
					<p className="-mt-2 text-xs text-muted-foreground">
						{form.type === "http" && t`Full URL. Accepts 2xx by default.`}
						{form.type === "keyword" && t`HTTP check that also looks for text in the first 2 MB of the response.`}
						{form.type === "ping" && t`ICMP echo. The hub may need NET_RAW capability in Docker.`}
						{form.type === "dns" && t`Resolves a record type, optionally against a custom resolver.`}
						{form.type === "tls" && t`Checks certificate expiry without an HTTP request.`}
					</p>
					{form.type === "keyword" && (
						<div className="grid gap-2">
							<Label htmlFor="mon-keyword">{t`Keyword`}</Label>
							<Input
								id="mon-keyword"
								value={form.keyword}
								onChange={(e) => set("keyword", e.target.value)}
								placeholder="Welcome"
							/>
							<div className="flex items-center justify-between">
								<Label htmlFor="mon-invert">{t`Alert when keyword is missing`}</Label>
								<Switch
									id="mon-invert"
									checked={!form.invertKeyword}
									onCheckedChange={(v) => set("invertKeyword", !v)}
								/>
							</div>
						</div>
					)}
					{(form.type === "http" || form.type === "keyword") && (
						<>
							<div className="grid grid-cols-2 gap-4">
								<div className="grid gap-2">
									<Label htmlFor="mon-method">{t`Method`}</Label>
									<Select value={form.method} onValueChange={(v) => set("method", v)}>
										<SelectTrigger id="mon-method">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"].map((m) => (
												<SelectItem key={m} value={m}>
													{m}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</div>
								<div className="grid gap-2">
									<Label htmlFor="mon-codes">{t`Accepted codes`}</Label>
									<Input
										id="mon-codes"
										value={form.acceptedCodes}
										onChange={(e) => set("acceptedCodes", e.target.value)}
										placeholder="200-299"
									/>
								</div>
							</div>
							<div className="grid grid-cols-2 gap-4">
								<div className="grid gap-2">
									<Label htmlFor="mon-auth">{t`Auth`}</Label>
									<Select value={form.authType} onValueChange={(v) => set("authType", v)}>
										<SelectTrigger id="mon-auth">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="none">{t`None`}</SelectItem>
											<SelectItem value="basic">{t`Basic`}</SelectItem>
											<SelectItem value="bearer">{t`Bearer token`}</SelectItem>
										</SelectContent>
									</Select>
								</div>
								{form.authType === "basic" && (
									<div className="grid gap-2">
										<Label htmlFor="mon-auth-user">{t`Username`}</Label>
										<Input
											id="mon-auth-user"
											value={form.authUser}
											onChange={(e) => set("authUser", e.target.value)}
											autoComplete="off"
										/>
									</div>
								)}
							</div>
							{(form.authType === "basic" || form.authType === "bearer") && (
								<div className="grid gap-2">
									<Label htmlFor="mon-auth-secret">
										{form.authType === "basic" ? t`Password` : t`Token`}
										{monitor && (
											<span className="ml-1 font-normal text-muted-foreground">{t`(leave empty to keep)`}</span>
										)}
									</Label>
									<Input
										id="mon-auth-secret"
										type="password"
										value={form.authSecret}
										onChange={(e) => set("authSecret", e.target.value)}
										autoComplete="new-password"
									/>
								</div>
							)}
							<div className="flex items-center justify-between">
								<Label htmlFor="mon-ignore-tls">{t`Ignore TLS errors (insecure)`}</Label>
								<Switch id="mon-ignore-tls" checked={form.ignoreTls} onCheckedChange={(v) => set("ignoreTls", v)} />
							</div>
						</>
					)}
					{(form.type === "http" || form.type === "keyword" || form.type === "tls") && (
						<>
							<div className="flex items-center justify-between">
								<Label htmlFor="mon-check-cert">{t`Watch certificate expiry`}</Label>
								<Switch id="mon-check-cert" checked={form.checkCert} onCheckedChange={(v) => set("checkCert", v)} />
							</div>
							{form.checkCert && (
								<div className="grid grid-cols-2 gap-4">
									<div className="grid gap-2">
										<Label htmlFor="mon-warn">{t`Warn below (days)`}</Label>
										<Input
											id="mon-warn"
											value={form.warnDays}
											onChange={(e) => set("warnDays", e.target.value)}
											inputMode="numeric"
										/>
									</div>
									<div className="grid gap-2">
										<Label htmlFor="mon-crit">{t`Critical below (days)`}</Label>
										<Input
											id="mon-crit"
											value={form.critDays}
											onChange={(e) => set("critDays", e.target.value)}
											inputMode="numeric"
										/>
									</div>
								</div>
							)}
						</>
					)}
					{form.type === "dns" && (
						<div className="grid grid-cols-2 gap-4">
							<div className="grid gap-2">
								<Label htmlFor="mon-qtype">{t`Record type`}</Label>
								<Select value={form.qtype} onValueChange={(v) => set("qtype", v)}>
									<SelectTrigger id="mon-qtype">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{["A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "SRV", "CAA", "PTR"].map((q) => (
											<SelectItem key={q} value={q}>
												{q}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<div className="grid gap-2">
								<Label htmlFor="mon-proto">{t`Protocol`}</Label>
								<Select value={form.protocol} onValueChange={(v) => set("protocol", v)}>
									<SelectTrigger id="mon-proto">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="udp">UDP</SelectItem>
										<SelectItem value="tcp">TCP</SelectItem>
									</SelectContent>
								</Select>
							</div>
							<div className="grid gap-2">
								<Label htmlFor="mon-resolver">{t`Resolver (optional)`}</Label>
								<Input
									id="mon-resolver"
									value={form.resolver}
									onChange={(e) => set("resolver", e.target.value)}
									placeholder="1.1.1.1"
								/>
							</div>
							<div className="grid gap-2">
								<Label htmlFor="mon-expected">{t`Expected answer (optional)`}</Label>
								<Input
									id="mon-expected"
									value={form.expectedAnswer}
									onChange={(e) => set("expectedAnswer", e.target.value)}
									placeholder="mail.example.com"
								/>
							</div>
						</div>
					)}
					{form.type === "ping" && (
						<div className="grid grid-cols-2 gap-4">
							<div className="grid gap-2">
								<Label htmlFor="mon-count">{t`Packets`}</Label>
								<Input
									id="mon-count"
									value={form.count}
									onChange={(e) => set("count", e.target.value)}
									inputMode="numeric"
								/>
							</div>
							<div className="grid gap-2">
								<Label htmlFor="mon-size">{t`Packet size`}</Label>
								<Input
									id="mon-size"
									value={form.packetSize}
									onChange={(e) => set("packetSize", e.target.value)}
									inputMode="numeric"
								/>
							</div>
						</div>
					)}
					{form.type === "tls" && (
						<div className="grid gap-2">
							<Label htmlFor="mon-port">{t`Port (default 443)`}</Label>
							<Input
								id="mon-port"
								value={form.port}
								onChange={(e) => set("port", e.target.value)}
								placeholder="443"
								inputMode="numeric"
							/>
						</div>
					)}
					<div className="grid grid-cols-2 gap-4">
						<div className="grid gap-2">
							<Label htmlFor="mon-interval">{t`Interval (seconds)`}</Label>
							<Input
								id="mon-interval"
								value={form.interval}
								onChange={(e) => set("interval", e.target.value)}
								inputMode="numeric"
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="mon-timeout">{t`Timeout (seconds)`}</Label>
							<Input
								id="mon-timeout"
								value={form.timeout}
								onChange={(e) => set("timeout", e.target.value)}
								inputMode="numeric"
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="mon-retries">{t`Retries before down`}</Label>
							<Input
								id="mon-retries"
								value={form.maxRetries}
								onChange={(e) => set("maxRetries", e.target.value)}
								inputMode="numeric"
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="mon-resend">{t`Resend every (minutes, 0 = never)`}</Label>
							<Input
								id="mon-resend"
								value={form.resendAfter}
								onChange={(e) => set("resendAfter", e.target.value)}
								inputMode="numeric"
							/>
						</div>
					</div>
					<div className="flex items-center justify-between">
						<Label htmlFor="mon-upside">{t`Invert status (upside down)`}</Label>
						<Switch id="mon-upside" checked={form.upsideDown} onCheckedChange={(v) => set("upsideDown", v)} />
					</div>
					<div className="flex items-center justify-between">
						<Label htmlFor="mon-notify">{t`Send notifications`}</Label>
						<Switch id="mon-notify" checked={form.notify} onCheckedChange={(v) => set("notify", v)} />
					</div>
					{error && <p className="text-sm text-destructive">{error}</p>}
				</div>
				<DialogFooter>
					<Button variant="outline" onClick={() => setOpen(false)}>
						{t`Cancel`}
					</Button>
					<Button onClick={save} disabled={saving}>
						{t`Save`}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
