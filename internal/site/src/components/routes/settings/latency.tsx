import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { redirectPage } from "@nanostores/router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useEffect, useState } from "react"
import { $router } from "@/components/router"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"

interface LatencyConfig {
	ping_targets?: string
}

interface TargetRow {
	id: string
	name: string
	addr: string
}

function newId() {
	return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

/** Parse stored "name=addr" / bare addr (comma or newline) into rows. */
function parseToRows(raw: string): TargetRow[] {
	if (!raw.trim()) {
		return [{ id: newId(), name: "", addr: "" }]
	}
	const normalized = raw.replace(/\r\n/g, "\n").replace(/\r/g, "\n")
	const parts = normalized.split(/[,\n]/).map((s) => s.trim()).filter(Boolean)
	if (parts.length === 0) {
		return [{ id: newId(), name: "", addr: "" }]
	}
	return parts.map((p) => {
		const eq = p.indexOf("=")
		if (eq > 0) {
			const left = p.slice(0, eq).trim()
			const right = p.slice(eq + 1).trim()
			// host:port=something is not a name
			if (left.includes(":") || left.includes(".")) {
				return { id: newId(), name: "", addr: p }
			}
			return { id: newId(), name: left, addr: right }
		}
		return { id: newId(), name: "", addr: p }
	})
}

/** Serialize rows to hub storage format (name=addr per line). */
function rowsToConfig(rows: TargetRow[]): string {
	return rows
		.map((r) => {
			const name = r.name.trim()
			const addr = r.addr.trim()
			if (!addr) return ""
			return name ? `${name}=${addr}` : addr
		})
		.filter(Boolean)
		.join("\n")
}

export default function LatencySettings() {
	const [rows, setRows] = useState<TargetRow[]>([{ id: newId(), name: "", addr: "" }])
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
				setRows(parseToRows(res.ping_targets ?? ""))
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

	function updateRow(id: string, field: "name" | "addr", value: string) {
		setRows((prev) => prev.map((r) => (r.id === id ? { ...r, [field]: value } : r)))
	}

	function addRow() {
		setRows((prev) => [...prev, { id: newId(), name: "", addr: "" }])
	}

	function removeRow(id: string) {
		setRows((prev) => {
			const next = prev.filter((r) => r.id !== id)
			return next.length === 0 ? [{ id: newId(), name: "", addr: "" }] : next
		})
	}

	async function save() {
		setIsSaving(true)
		try {
			const payload = rowsToConfig(rows)
			const res = await pb.send<LatencyConfig>("/api/beszel/latency-config", {
				method: "POST",
				body: { ping_targets: payload },
			})
			setRows(parseToRows(res.ping_targets ?? ""))
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
						Add named TCP latency probes for all agents. Name is shown on the chart; address is host:port only.
					</Trans>
				</p>
			</div>

			<div className="space-y-3 max-w-2xl">
				<div className="hidden sm:grid sm:grid-cols-[1fr_1.4fr_auto] gap-2 px-0.5">
					<Label className="text-muted-foreground font-normal">
						<Trans>Name</Trans>
					</Label>
					<Label className="text-muted-foreground font-normal">
						<Trans>Address</Trans>
					</Label>
					<span />
				</div>

				{rows.map((row) => (
					<div key={row.id} className="grid grid-cols-1 sm:grid-cols-[1fr_1.4fr_auto] gap-2 items-center">
						<div className="space-y-1">
							<Label htmlFor={`name-${row.id}`} className="sm:hidden text-muted-foreground font-normal">
								<Trans>Name</Trans>
							</Label>
							<Input
								id={`name-${row.id}`}
								value={row.name}
								disabled={isLoading || isSaving}
								placeholder={t`e.g. 电信广东`}
								onChange={(e) => updateRow(row.id, "name", e.target.value)}
							/>
						</div>
						<div className="space-y-1">
							<Label htmlFor={`addr-${row.id}`} className="sm:hidden text-muted-foreground font-normal">
								<Trans>Address</Trans>
							</Label>
							<Input
								id={`addr-${row.id}`}
								value={row.addr}
								disabled={isLoading || isSaving}
								placeholder="gd-ct-v4.ip.zstaticcdn.com:80"
								className="font-mono text-sm"
								onChange={(e) => updateRow(row.id, "addr", e.target.value)}
							/>
						</div>
						<Button
							type="button"
							variant="ghost"
							size="icon"
							className="shrink-0 text-muted-foreground hover:text-destructive"
							disabled={isLoading || isSaving}
							onClick={() => removeRow(row.id)}
							aria-label={t`Remove`}
						>
							<Trash2Icon className="size-4" />
						</Button>
					</div>
				))}

				<Button type="button" variant="outline" size="sm" className="gap-1.5" disabled={isLoading || isSaving} onClick={addRow}>
					<PlusIcon className="size-4" />
					<Trans>Add target</Trans>
				</Button>
			</div>

			<Button onClick={save} disabled={isLoading || isSaving}>
				{isSaving ? <Trans>Saving...</Trans> : <Trans>Save Settings</Trans>}
			</Button>
		</div>
	)
}
