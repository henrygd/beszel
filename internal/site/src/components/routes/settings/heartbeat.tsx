import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { redirectPage } from "@nanostores/router"
import { LoaderCircleIcon, SendIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { $router } from "@/components/router"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"
import { cn } from "@/lib/utils"

interface HeartbeatStatus {
	enabled: boolean
	url?: string
	interval?: number
	method?: string
	msg?: string
}

export default function HeartbeatSettings() {
	const [status, setStatus] = useState<HeartbeatStatus | null>(null)
	const [isLoading, setIsLoading] = useState(true)
	const [isTesting, setIsTesting] = useState(false)

	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
	}

	useEffect(() => {
		fetchStatus()
	}, [])

	async function fetchStatus() {
		try {
			setIsLoading(true)
			const res = await pb.send<HeartbeatStatus>("/api/beszel/heartbeat-status", {})
			setStatus(res)
		} catch (error: unknown) {
			toast({
				title: t`Error`,
				description: (error as Error).message,
				variant: "destructive",
			})
		} finally {
			setIsLoading(false)
		}
	}

	async function sendTestHeartbeat() {
		setIsTesting(true)
		try {
			const res = await pb.send<{ err: string | false }>("/api/beszel/test-heartbeat", {
				method: "POST",
			})
			if ("err" in res && !res.err) {
				toast({
					title: t`Heartbeat sent successfully`,
					description: t`Check your monitoring service`,
				})
			} else {
				toast({
					title: t`Error`,
					description: (res.err as string) ?? t`Failed to send heartbeat`,
					variant: "destructive",
				})
			}
		} catch (error: unknown) {
			toast({
				title: t`Error`,
				description: (error as Error).message,
				variant: "destructive",
			})
		} finally {
			setIsTesting(false)
		}
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>Heartbeat Monitoring</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						Send periodic outbound pings to an external monitoring service so you can monitor Beszel without exposing it
						to the internet.
					</Trans>
				</p>
			</div>
			<Separator className="my-4" />

			{status?.enabled ? (
				<EnabledState status={status} isTesting={isTesting} sendTestHeartbeat={sendTestHeartbeat} />
			) : (
				<NotEnabledState isLoading={isLoading} />
			)}
		</div>
	)
}

function EnabledState({
	status,
	isTesting,
	sendTestHeartbeat,
}: {
	status: HeartbeatStatus
	isTesting: boolean
	sendTestHeartbeat: () => void
}) {
	const TestIcon = isTesting ? LoaderCircleIcon : SendIcon
	return (
		<div className="space-y-5">
			<div className="flex items-center gap-2">
				<Badge variant="success">
					<Trans>Active</Trans>
				</Badge>
			</div>
			<div className="grid gap-4 sm:grid-cols-2">
				<ConfigItem label={t`Endpoint URL`} value={status.url ?? ""} mono />
				<ConfigItem label={t`Interval`} value={`${status.interval}s`} />
				<ConfigItem label={t`HTTP Method`} value={status.method ?? "POST"} />
			</div>

			<Separator />

			<div>
				<h4 className="text-base font-medium mb-1">
					<Trans>Test heartbeat</Trans>
				</h4>
				<p className="text-sm text-muted-foreground leading-relaxed mb-3">
					<Trans>Send a single heartbeat ping to verify your endpoint is working.</Trans>
				</p>
				<Button
					type="button"
					variant="outline"
					className="flex items-center gap-1.5"
					onClick={sendTestHeartbeat}
					disabled={isTesting}
				>
					<TestIcon className={cn("size-4", isTesting && "animate-spin")} />
					<Trans>Send test heartbeat</Trans>
				</Button>
			</div>

			<Separator />

			<div>
				<h4 className="text-base font-medium mb-2">
					<Trans>Payload format</Trans>
				</h4>
				<p className="text-sm text-muted-foreground leading-relaxed mb-2">
					<Trans>
						When using POST, each heartbeat includes a JSON payload with system status summary, list of down systems,
						and triggered alerts.
					</Trans>
				</p>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						The overall status is <code className="bg-muted rounded-sm px-1 text-primary">ok</code> when all systems are
						up, <code className="bg-muted rounded-sm px-1 text-primary">warn</code> when alerts are triggered, and{" "}
						<code className="bg-muted rounded-sm px-1 text-primary">error</code> when any system is down.
					</Trans>
				</p>
			</div>
		</div>
	)
}

function NotEnabledState({ isLoading }: { isLoading?: boolean }) {
	return (
		<div className={cn("grid gap-4", isLoading && "animate-pulse")}>
			<div>
				<p className="text-sm text-muted-foreground leading-relaxed mb-3">
					<Trans>Set the following environment variables on your Beszel hub to enable heartbeat monitoring:</Trans>
				</p>
				<div className="grid gap-2.5">
					<EnvVarItem
						name="HEARTBEAT_URL"
						description={t`Endpoint URL to ping (required)`}
						example="https://uptime.betterstack.com/api/v1/heartbeat/xxxx"
					/>
					<EnvVarItem name="HEARTBEAT_INTERVAL" description={t`Seconds between pings (default: 60)`} example="60" />
					<EnvVarItem
						name="HEARTBEAT_METHOD"
						description={t`HTTP method: POST, GET, or HEAD (default: POST)`}
						example="POST"
					/>
				</div>
			</div>
			<p className="text-sm text-muted-foreground leading-relaxed">
				<Trans>After setting the environment variables, restart your Beszel hub for changes to take effect.</Trans>
			</p>
		</div>
	)
}

function ConfigItem({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
	return (
		<div>
			<p className="text-sm font-medium mb-0.5">{label}</p>
			<p className={cn("text-sm text-muted-foreground break-all", mono && "font-mono")}>{value}</p>
		</div>
	)
}

function EnvVarItem({ name, description, example }: { name: string; description: string; example: string }) {
	return (
		<div className="bg-muted/50 rounded-md px-3 py-2.5 grid gap-1.5">
			<code className="text-sm font-mono text-primary font-medium leading-tight">{name}</code>
			<p className="text-sm text-muted-foreground">{description}</p>
			<p className="text-xs text-muted-foreground">
				<Trans>Example:</Trans> <code className="font-mono">{example}</code>
			</p>
		</div>
	)
}
