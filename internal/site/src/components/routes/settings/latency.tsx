import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { redirectPage } from "@nanostores/router"
import { useEffect, useState } from "react"
import { $router } from "@/components/router"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"

interface LatencyConfig {
	ping_targets?: string
}

const EXAMPLE = `电信广东=gd-ct-v4.ip.zstaticcdn.com:80
移动广东=gd-cm-v4.ip.zstaticcdn.com:80
联通广东=gd-cu-v4.ip.zstaticcdn.com:80`

export default function LatencySettings() {
	const [targets, setTargets] = useState("")
	const [isLoading, setIsLoading] = useState(true)
	const [isSaving, setIsSaving] = useState(false)

	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
	}

	useEffect(() => {
		;(async () => {
			try {
				setIsLoading(true)
				const res = await pb.send<LatencyConfig>("/api/beszel/latency-config", {})
				// show multi-line for readability (commas → newlines)
				const raw = res.ping_targets ?? ""
				setTargets(raw.includes("\n") ? raw : raw.split(",").map((s) => s.trim()).filter(Boolean).join("\n"))
			} catch (error: unknown) {
				toast({
					title: t`Error`,
					description: (error as Error).message,
					variant: "destructive",
				})
			} finally {
				setIsLoading(false)
			}
		})()
	}, [])

	async function save() {
		setIsSaving(true)
		try {
			const res = await pb.send<LatencyConfig>("/api/beszel/latency-config", {
				method: "POST",
				body: { ping_targets: targets.trim() },
			})
			const raw = res.ping_targets ?? ""
			setTargets(raw.includes("\n") ? raw : raw.split(",").map((s) => s.trim()).filter(Boolean).join("\n"))
			toast({
				title: t`Settings saved`,
				description: t`Latency probe targets updated. Agents pick them up on the next poll.`,
			})
		} catch (error: unknown) {
			toast({
				title: t`Failed to save settings`,
				description: (error as Error).message,
				variant: "destructive",
			})
		} finally {
			setIsSaving(false)
		}
	}

	return (
		<div className="space-y-6">
			<div>
				<h3 className="text-lg font-medium">
					<Trans>Latency</Trans>
				</h3>
				<p className="text-sm text-muted-foreground mt-1">
					<Trans>
						Configure named TCP latency probes for all agents. Each line is shown separately on the latency chart.
					</Trans>
				</p>
			</div>

			<div className="space-y-2 max-w-xl">
				<Label htmlFor="ping_targets">
					<Trans>Ping Targets</Trans>
				</Label>
				<textarea
					id="ping_targets"
					value={targets}
					disabled={isLoading || isSaving}
					onChange={(e) => setTargets(e.target.value)}
					rows={6}
					placeholder={EXAMPLE}
					className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 font-mono leading-relaxed"
				/>
				<p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-line">
					{t`Format: 显示名=主机:端口 (one per line). Example:`}
					{"\n"}
					{EXAMPLE}
				</p>
			</div>

			<div className="flex gap-2">
				<Button onClick={save} disabled={isLoading || isSaving}>
					{isSaving ? <Trans>Saving...</Trans> : <Trans>Save Settings</Trans>}
				</Button>
				<Button
					type="button"
					variant="outline"
					disabled={isLoading || isSaving}
					onClick={() => setTargets(EXAMPLE)}
				>
					<Trans>Fill example</Trans>
				</Button>
			</div>
		</div>
	)
}
