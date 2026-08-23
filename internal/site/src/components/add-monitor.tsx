import { Trans } from "@lingui/react/macro"
import { memo, useEffect, useState } from "react"
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { isReadOnlyUser, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import type { MonitorRecord } from "@/types"
import { navigate } from "./router"

export function AddMonitorDialog({ open, setOpen }: { open: boolean; setOpen: (open: boolean) => void }) {
	if (isReadOnlyUser()) {
		return null
	}
	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<MonitorDialog setOpen={setOpen} />
		</Dialog>
	)
}

export const MonitorDialog = memo(
	({ setOpen, monitor }: { setOpen: (open: boolean) => void; monitor?: MonitorRecord }) => {
		const [type, setType] = useState<string>(monitor?.type || "http")
		const [name, setName] = useState(monitor?.name || "")
		const [url, setUrl] = useState(monitor?.url || "")
		const [host, setHost] = useState(monitor?.host || "")
		const [port, setPort] = useState(monitor?.port ? String(monitor.port) : "80")
		const [interval, setIntervalVal] = useState(monitor?.interval ? String(monitor.interval) : "60")
		const [timeout, setTimeoutVal] = useState(monitor?.timeout ? String(monitor.timeout) : "10")
		const [method, setMethod] = useState(monitor?.method || "get")
		const [expectedStatus, setExpectedStatus] = useState(monitor?.expected_status || "")
		const [expectedBody, setExpectedBody] = useState(monitor?.expected_body || "")
		const [secure, setSecure] = useState<boolean>(monitor?.secure || false)
		const [retry, setRetry] = useState<boolean>(monitor?.retry || true)
		const [saving, setSaving] = useState(false)

		useEffect(() => {
			if (monitor) {
				setType(monitor.type || "http")
				setName(monitor.name || "")
				setUrl(monitor.url || "")
				setHost(monitor.host || "")
				setPort(monitor.port ? String(monitor.port) : "80")
				setIntervalVal(monitor.interval ? String(monitor.interval) : "60")
				setTimeoutVal(monitor.timeout ? String(monitor.timeout) : "10")
				setMethod(monitor.method || "get")
				setExpectedStatus(monitor.expected_status || "")
				setExpectedBody(monitor.expected_body || "")
				setSecure(monitor.secure || false)
				setRetry(monitor.retry || true)
			}
		}, [monitor?.id])

		async function handleSubmit(e: React.FormEvent) {
			e.preventDefault()
			if (saving) return
			setSaving(true)
			try {
				const data: Record<string, unknown> = {
					name,
					type,
					interval: Math.max(5, parseInt(interval) || 60),
					timeout: Math.max(1, parseInt(timeout) || 10),
					retry,
					secure,
					user: pb.authStore.record!.id,
				}

				if (type === "http") {
					data.url = url
					data.method = method
					if (expectedStatus) data.expected_status = expectedStatus
					if (expectedBody) data.expected_body = expectedBody
					data.host = ""
					data.port = 0
				} else if (type === "tcp") {
					data.host = host
					data.port = Math.max(1, parseInt(port) || 80)
					data.url = ""
					data.method = ""
					data.expected_status = ""
					data.expected_body = ""
				} else if (type === "ping") {
					data.host = host
					data.port = 0
					data.url = ""
					data.method = ""
					data.expected_status = ""
					data.expected_body = ""
				}

				if (monitor) {
					await pb.collection("monitors").update(monitor.id, { ...data, status: SystemStatus.Pending })
				} else {
					await pb.collection("monitors").create(data)
				}
				setOpen(false)
				navigate("/monitors")
			} catch (err) {
				console.error(err)
			} finally {
				setSaving(false)
			}
		}

		return (
			<DialogContent className="w-[90%] sm:w-auto sm:ns-dialog max-w-full rounded-lg">
				<DialogHeader>
					<DialogTitle className="mb-1 pb-1 max-w-100 truncate pr-8">
						{monitor ? <Trans>Edit Monitor</Trans> : <Trans>Add Monitor</Trans>}
					</DialogTitle>
					<DialogDescription className="mb-3 leading-relaxed w-0 min-w-full">
						<Trans>Configure an uptime monitor. The type determines which fields are required.</Trans>
					</DialogDescription>
				</DialogHeader>

				<form onSubmit={handleSubmit} className="grid gap-4">
					<div className="grid gap-2">
						<Label htmlFor="m-type">
							<Trans>Type</Trans>
						</Label>
						<Select value={type} onValueChange={(v) => setType(v)} disabled={!!monitor}>
							<SelectTrigger id="m-type">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="http">
									<Trans>HTTP(S)</Trans>
								</SelectItem>
								<SelectItem value="tcp">
									<Trans>TCP</Trans>
								</SelectItem>
								<SelectItem value="ping">
									<Trans>Ping</Trans>
								</SelectItem>
							</SelectContent>
						</Select>
					</div>

					<div className="grid gap-2">
						<Label htmlFor="m-name">
							<Trans>Name</Trans>
						</Label>
						<Input id="m-name" value={name} onChange={(e) => setName(e.target.value)} required />
					</div>

					{type === "http" && (
						<div className="grid gap-2">
							<Label htmlFor="m-url">
								<Trans>URL</Trans>
							</Label>
							<Input
								id="m-url"
								value={url}
								onChange={(e) => setUrl(e.target.value)}
								placeholder="https://example.com/health"
								required
							/>
						</div>
					)}

					{type === "tcp" && (
						<>
							<div className="grid gap-2">
								<Label htmlFor="m-host">
									<Trans>Host</Trans>
								</Label>
								<Input id="m-host" value={host} onChange={(e) => setHost(e.target.value)} placeholder="example.com" required />
							</div>
							<div className="grid gap-2">
								<Label htmlFor="m-port">
									<Trans>Port</Trans>
								</Label>
								<Input id="m-port" type="number" min="1" max="65535" value={port} onChange={(e) => setPort(e.target.value)} required />
							</div>
						</>
					)}

					{type === "ping" && (
						<div className="grid gap-2">
							<Label htmlFor="m-host">
								<Trans>Host / IP</Trans>
							</Label>
							<Input id="m-host" value={host} onChange={(e) => setHost(e.target.value)} placeholder="example.com or 8.8.8.8" required />
						</div>
					)}

					<div className="grid grid-cols-2 gap-3">
						<div className="grid gap-2">
							<Label htmlFor="m-interval">
								<Trans>Interval (sec)</Trans>
							</Label>
							<Input id="m-interval" type="number" min="5" max="86400" value={interval} onChange={(e) => setIntervalVal(e.target.value)} required />
						</div>
						<div className="grid gap-2">
							<Label htmlFor="m-timeout">
								<Trans>Timeout (sec)</Trans>
							</Label>
							<Input id="m-timeout" type="number" min="1" max="120" value={timeout} onChange={(e) => setTimeoutVal(e.target.value)} required />
						</div>
					</div>

					{type === "http" && (
						<>
							<div className="grid grid-cols-2 gap-3">
								<div className="grid gap-2">
									<Label htmlFor="m-method">
										<Trans>Method</Trans>
									</Label>
									<Select value={method} onValueChange={(v) => setMethod(v)}>
										<SelectTrigger id="m-method">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="get">GET</SelectItem>
											<SelectItem value="post">POST</SelectItem>
											<SelectItem value="put">PUT</SelectItem>
											<SelectItem value="delete">DELETE</SelectItem>
											<SelectItem value="head">HEAD</SelectItem>
											<SelectItem value="patch">PATCH</SelectItem>
										</SelectContent>
									</Select>
								</div>
								<div className="grid gap-2">
									<Label htmlFor="m-expected-status">
										<Trans>Expected Status</Trans>
									</Label>
									<Input
										id="m-expected-status"
										value={expectedStatus}
										onChange={(e) => setExpectedStatus(e.target.value)}
										placeholder="200, 201, 3xx"
									/>
								</div>
							</div>
							<div className="grid gap-2">
								<Label htmlFor="m-expected-body">
									<Trans>Expected Body</Trans>
								</Label>
								<Textarea
									id="m-expected-body"
									value={expectedBody}
									onChange={(e) => setExpectedBody(e.target.value)}
									placeholder="substring to search for in response"
									rows={2}
								/>
							</div>
						</>
					)}

					<div className="flex items-center justify-between">
						<div className="grid gap-0.5">
							<Label>
								<Trans>Secure (skip TLS verify)</Trans>
							</Label>
							<p className="text-xs text-muted-foreground">
								<Trans>Allow self-signed or invalid certificates</Trans>
							</p>
						</div>
						<Switch id="m-secure" checked={secure} onCheckedChange={setSecure} />
					</div>

					<div className="flex items-center justify-between">
						<div className="grid gap-0.5">
							<Label>
								<Trans>Retry on failure</Trans>
							</Label>
							<p className="text-xs text-muted-foreground">
								<Trans>Retry the check a few times before marking as down</Trans>
							</p>
						</div>
						<Switch id="m-retry" checked={retry} onCheckedChange={setRetry} />
					</div>

					<DialogFooter className="flex justify-end gap-2 mt-2">
						<Button type="submit" disabled={saving}>
							{saving ? (
								<Trans>Saving...</Trans>
							) : monitor ? (
								<Trans>Save Monitor</Trans>
							) : (
								<Trans>Add Monitor</Trans>
							)}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		)
	}
)
