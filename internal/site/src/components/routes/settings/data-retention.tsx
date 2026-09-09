import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { redirectPage } from "@nanostores/router"
import { DatabaseIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { $router } from "@/components/router"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"

const retentionOptions = [
	{ value: "30d", label: "30 days (default)" },
	{ value: "60d", label: "60 days" },
	{ value: "90d", label: "90 days (3 months)" },
	{ value: "180d", label: "180 days (6 months)" },
	{ value: "365d", label: "365 days (1 year)" },
	{ value: "730d", label: "730 days (2 years)" },
	{ value: "1095d", label: "1095 days (3 years)" },
	{ value: "1825d", label: "1825 days (5 years)" },
	{ value: "never", label: "Never delete" },
] as const

const tierInfo = [
	{ type: "1m", interval: "1 minute", retention: "1 hour", note: "Realtime" },
	{ type: "10m", interval: "10 minutes", retention: "12 hours", note: "" },
	{ type: "20m", interval: "20 minutes", retention: "24 hours", note: "" },
	{ type: "120m", interval: "2 hours", retention: "7 days", note: "" },
	{ type: "480m", interval: "8 hours", retention: "Configurable", note: "Long-term" },
] as const

export default function DataRetentionSettings() {
	const [retention, setRetention] = useState("30d")
	const [hubSettingsId, setHubSettingsId] = useState("hubsettings0001")
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const [envOverride, setEnvOverride] = useState(false)
	const [effectiveRetention, setEffectiveRetention] = useState<string | null>(null)

	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
		return null
	}

	useEffect(() => {
		// Prefer effective endpoint for env-aware value (#1, #11), fallback to direct collection for old hubs
		pb.send<{ retention: string; dbRetention: string; envOverride: boolean }>("/api/beszel/retention", {})
			.then((res) => {
				setRetention(res.dbRetention)
				setEffectiveRetention(res.retention)
				setEnvOverride(res.envOverride)
				return pb.collection("hub_settings").getFirstListItem("", { fields: "id" })
			})
			.then((rec) => {
				if (rec?.id) setHubSettingsId(rec.id)
			})
			.catch(() => {
				// fallback: old hub without endpoint or hub_settings not yet migrated
				pb.collection("hub_settings")
					.getFirstListItem("", { fields: "id,retention" })
					.then((rec) => {
						setRetention((rec as unknown as { retention: string }).retention)
						setHubSettingsId(rec.id)
					})
					.catch(() => {
						pb.collection("hub_settings")
							.getOne("hubsettings0001", { fields: "id,retention" })
							.then((rec) => {
								setRetention((rec as unknown as { retention: string }).retention)
								setHubSettingsId(rec.id)
							})
							.catch(() => {})
					})
			})
			.finally(() => setLoading(false))
	}, [])

	async function handleSave() {
		setSaving(true)
		try {
			await pb.collection("hub_settings").update(hubSettingsId, { retention })
			toast({
				title: t`Retention saved`,
				description: t`Data retention updated to ${retention}. New setting applies on next hourly cleanup.`,
			})
		} catch {
			toast({ title: t`Failed to save retention`, variant: "destructive" })
		}
		setSaving(false)
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2 flex items-center gap-2">
					<DatabaseIcon className="h-5 w-5" />
					<Trans>Data Retention</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						Control how long Beszel keeps historical metrics. Data is downsampled into tiers; only the 8-hour (480m)
						tier is configurable — shorter tiers keep high resolution for recent data and are fixed.
					</Trans>
				</p>
			</div>
			<Separator className="my-4" />

			<div className="grid gap-2 mb-6">
				<h4 className="text-base font-medium">
					<Trans>Storage tiers</Trans>
				</h4>
				<div className="rounded-md border overflow-hidden">
					<div className="grid grid-cols-3 bg-muted/50 px-3 py-2 text-xs font-medium text-muted-foreground">
						<span>
							<Trans>Type</Trans>
						</span>
						<span>
							<Trans>Interval</Trans>
						</span>
						<span>
							<Trans>Retention</Trans>
						</span>
					</div>
					{tierInfo.map((row) => (
						<div key={row.type} className="grid grid-cols-3 px-3 py-2 text-sm border-t">
							<span className="font-mono">{row.type}</span>
							<span>{row.interval}</span>
							<span className={row.type === "480m" ? "font-medium text-primary" : ""}>
								{row.retention} {row.note ? `· ${row.note}` : ""}
							</span>
						</div>
					))}
				</div>
				<p className="text-xs text-muted-foreground">
					<Trans>About 5-10 MB per system per year at 480m resolution. Shorter tiers are pruned hourly.</Trans>
				</p>
			</div>

			<Separator className="my-4" />

			<div className="grid gap-2">
				<Label htmlFor="retention" className="text-base font-medium">
					<Trans>Long-term (480m) retention</Trans>
				</Label>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						How long to keep 8-hour aggregated stats. Available chart periods (60d, 90d, 180d, 365d, 730d, 3y, 5y) are
						filtered to ≤ retention. Lowering retention deletes excess data on next hourly cleanup.
					</Trans>
				</p>
				{envOverride && effectiveRetention && (
					<div className="rounded-md border border-amber-500/50 bg-amber-500/10 px-3 py-2.5 mb-2">
						<p className="text-sm font-medium text-amber-900 dark:text-amber-100">
							<Trans>Env override active</Trans>
						</p>
						<p className="text-xs text-muted-foreground mt-1">
							<Trans>
								BESZEL_HUB_RETENTION is set to{" "}
								<code className="font-mono bg-muted px-1 rounded">{effectiveRetention}</code> — DB value (
								<code className="font-mono bg-muted px-1 rounded">{retention}</code>) is ignored until the env var is
								removed and the hub is restarted.
							</Trans>
						</p>
					</div>
				)}
				{retention === "never" && !envOverride && (
					<div className="rounded-md border border-amber-500/50 bg-amber-500/10 px-3 py-2.5 mb-2">
						<p className="text-sm font-medium text-amber-900 dark:text-amber-100">
							<Trans>Unbounded growth</Trans>
						</p>
						<p className="text-xs text-muted-foreground mt-1">
							<Trans>
								“Never delete” keeps 480m data forever. Ensure disk monitoring is in place — about 5-10 MB per system
								per year, growing without bound.
							</Trans>
						</p>
					</div>
				)}
				<div className="grid sm:grid-cols-3 gap-4 items-end max-w-xl mt-2">
					<div className="grid gap-2">
						<Label className="block" htmlFor="retention">
							<Trans>Retention</Trans>
						</Label>
						<Select value={retention} onValueChange={setRetention} disabled={loading || envOverride}>
							<SelectTrigger id="retention">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{retentionOptions.map((opt) => (
									<SelectItem key={opt.value} value={opt.value}>
										{opt.label}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
					<div className="flex items-end">
						<Button
							type="button"
							onClick={handleSave}
							disabled={saving || loading || envOverride}
							className="flex items-center gap-1.5"
						>
							{saving ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : <SaveIcon className="h-4 w-4" />}
							<Trans>Save Retention</Trans>
						</Button>
					</div>
				</div>
				<p className="text-xs text-muted-foreground mt-2">
					{envOverride ? (
						<Trans>
							Editing disabled while env override is active. Remove BESZEL_HUB_RETENTION and restart to edit.
						</Trans>
					) : (
						<Trans>Requires hub restart to apply env change if BESZEL_HUB_RETENTION is used.</Trans>
					)}
				</p>
			</div>
		</div>
	)
}
