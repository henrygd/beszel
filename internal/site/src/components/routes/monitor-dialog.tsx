import { t } from "@lingui/core/macro"
import { useState } from "react"
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
import { pb } from "@/lib/api"
import type { MonitorRecord, MonitorType } from "@/types"

const TYPES: { value: MonitorType; label: string; placeholder: string; hint: string }[] = [
	{ value: "http", label: "HTTP(S)", placeholder: "https://example.com", hint: t`Full URL. Accepts 2xx by default.` },
	{ value: "keyword", label: "Keyword", placeholder: "https://example.com", hint: t`HTTP check that also looks for text in the response.` },
	{ value: "ping", label: "Ping", placeholder: "example.com", hint: t`ICMP echo. The hub may need NET_RAW capability in Docker.` },
	{ value: "dns", label: "DNS", placeholder: "example.com", hint: t`Resolves a record type, optionally against a custom resolver.` },
	{ value: "tls", label: "TLS cert", placeholder: "example.com:443", hint: t`Checks certificate expiry without an HTTP request.` },
]

interface FormState {
	name: string
	type: MonitorType
	target: string
	interval: string
	timeout: string
	keyword: string
	qtype: string
	resolver: string
	port: string
	notify: boolean
}

function initialState(monitor?: MonitorRecord): FormState {
	const cfg = monitor?.config ?? {}
	return {
		name: monitor?.name ?? "",
		type: monitor?.type ?? "http",
		target: monitor?.target ?? "",
		interval: String(monitor?.interval ?? 60),
		timeout: String(monitor?.timeout ?? 10),
		keyword: String(cfg.keyword ?? ""),
		qtype: String(cfg.qtype ?? "A"),
		resolver: String(cfg.resolver ?? ""),
		port: String(cfg.port ?? ""),
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
	const [form, setForm] = useState<FormState>(() => initialState(monitor))
	const [error, setError] = useState("")
	const [saving, setSaving] = useState(false)

	const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
		setForm((prev) => ({ ...prev, [key]: value }))
	}

	// Refresh defaults when switching between monitors.
	const [lastId, setLastId] = useState(monitor?.id)
	if (monitor?.id !== lastId) {
		setLastId(monitor?.id)
		setForm(initialState(monitor))
		setError("")
	}

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
		const config: Record<string, unknown> = {}
		if (form.type === "keyword") {
			if (!form.keyword) {
				setError(t`Keyword is required for keyword monitors.`)
				return
			}
			config.keyword = form.keyword
		}
		if (form.type === "dns") {
			config.qtype = form.qtype || "A"
			if (form.resolver.trim()) {
				config.resolver = form.resolver.trim()
			}
		}
		if (form.type === "tls" && form.port.trim()) {
			const port = Number.parseInt(form.port, 10)
			if (!(port > 0 && port <= 65535)) {
				setError(t`Port must be between 1 and 65535.`)
				return
			}
			config.port = port
		}
		setSaving(true)
		try {
			const body = {
				name: form.name.trim(),
				type: form.type,
				target: form.target.trim(),
				interval,
				timeout,
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
					<p className="-mt-2 text-xs text-muted-foreground">{activeType.hint}</p>
					{form.type === "keyword" && (
						<div className="grid gap-2">
							<Label htmlFor="mon-keyword">{t`Keyword`}</Label>
							<Input
								id="mon-keyword"
								value={form.keyword}
								onChange={(e) => set("keyword", e.target.value)}
								placeholder="Welcome"
							/>
						</div>
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
								<Label htmlFor="mon-resolver">{t`Resolver (optional)`}</Label>
								<Input
									id="mon-resolver"
									value={form.resolver}
									onChange={(e) => set("resolver", e.target.value)}
									placeholder="1.1.1.1"
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
