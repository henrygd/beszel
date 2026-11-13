import { Trans } from "@lingui/react/macro"
import { CopyIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { copyToClipboard, getHubURL } from "@/lib/utils"

/** The URL a push monitor's sender requests to report a heartbeat. */
export function getPushUrl(token: string) {
	return `${getHubURL().replace(/\/$/, "")}/api/beszel/push/${token}`
}

/** Shows a push monitor's heartbeat URL with a copy button and an example command. */
export function PushUrl({ token }: { token: string }) {
	const url = getPushUrl(token)
	return (
		<div className="grid gap-2">
			<Label htmlFor="monitor-push-url">
				<Trans>Heartbeat URL</Trans>
			</Label>
			<div className="flex gap-2">
				<Input id="monitor-push-url" value={url} readOnly onFocus={(e) => e.target.select()} />
				<Button type="button" variant="outline" size="icon" onClick={() => copyToClipboard(url)}>
					<CopyIcon className="size-4" />
					<span className="sr-only">
						<Trans>Copy</Trans>
					</span>
				</Button>
			</div>
			<p className="text-xs text-muted-foreground">
				<Trans>Request this URL from your job or script on every run. For example:</Trans>{" "}
				<code className="break-all">curl -fsS {url}</code>
			</p>
		</div>
	)
}
