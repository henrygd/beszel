import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { redirectPage } from "@nanostores/router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { $router } from "@/components/router"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"
import { $systems } from "@/lib/stores"
import { cn } from "@/lib/utils"

interface LatencyConfig {
	ping_targets?: string
	scope?: "all" | "selected"
	system_ids?: string[]
}

interface TargetRow {
	id: string
	name: string
	addr: string
}

function newId() {
	return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function parseToRows(raw: string): TargetRow[] {
	if (!raw.trim()) {
		return [{ id: newId(), name: "", addr: "" }]
	}
	const normalized = raw.replace(/\r\n/g, "\n").replace(/\r/g, "\n")
	const parts = normalized
		.split(/[,\n]/)
		.map((s) => s.trim())
		.filter(Boolean)
	if (parts.length === 0) {
		return [{ id: newId(), name: "", addr: "" }]
	}
	return parts.map((p) => {
		const eq = p.indexOf("=")
		if (eq > 0) {
			const left = p.slice(0, eq).trim()
			const right = p.slice(eq + 1).trim()
			if (left.includes(":") || left.includes(".")) {
				return { id: newId(), name: "", addr: p }
			}
			return { id: newId(), name: left, addr: right }
		}
		return { id: newId(), name: "", addr: p }
	})
}

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
	const systems = useStore($systems)
	const sortedSystems = useMemo(
		() => [...systems].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" })),
		[systems],
	)

	const [rows, setRows] = useState<TargetRow[]>([{ id: newId(), name: "", addr: "" }])
	const [scope, setScope] = useState<"all" | "selected">("all")
	const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
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
				setScope(res.scope === "selected" ? "selected" : "all")
				setSelectedIds(new Set(res.system_ids ?? []))
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

	function toggleSystem(id: string, checked: boolean) {
		setSelectedIds((prev) => {
			const next = new Set(prev)
			if (checked) next.add(id)
			else next.delete(id)
			return next
		})
	}

	function selectAllSystems() {
		setSelectedIds(new Set(sortedSystems.map((s) => s.id)))
	}

	function clearSystems() {
		setSelectedIds(new Set())
	}

	async function save() {
		if (scope === "selected" && selectedIds.size === 0) {
			toast({
				title: t`Select at least one system`,
				description: t`Or switch to all systems.`,
				variant: "destructive",
			})
			return
		}
		setIsSaving(true)
		try {
			const res = await pb.send<LatencyConfig>("/api/beszel/latency-config", {
				method: "POST",
				body: {
					ping_targets: rowsToConfig(rows),
					scope,
					system_ids: scope === "selected" ? [...selectedIds] : [],
				},
			})
			setRows(parseToRows(res.ping_targets ?? ""))
			setScope(res.scope === "selected" ? "selected" : "all")
			setSelectedIds(new Set(res.system_ids ?? []))
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
		<div className="space-y-8">
			<div>
				<h3 className="text-lg font-medium">
					<Trans>Latency</Trans>
				</h3>
				<p className="text-sm text-muted-foreground mt-1">
					<Trans>
						Add named TCP latency probes. Choose all systems or only selected machines.
					</Trans>
				</p>
			</div>

			{/* Probe targets */}
			<div className="space-y-3 max-w-2xl">
				<h4 className="text-sm font-medium">
					<Trans>Probe targets</Trans>
				</h4>
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

				<Button
					type="button"
					variant="outline"
					size="sm"
					className="gap-1.5"
					disabled={isLoading || isSaving}
					onClick={addRow}
				>
					<PlusIcon className="size-4" />
					<Trans>Add target</Trans>
				</Button>
			</div>

			{/* Apply scope */}
			<div className="space-y-3 max-w-2xl">
				<h4 className="text-sm font-medium">
					<Trans>Apply to systems</Trans>
				</h4>
				<div className="flex flex-col gap-2">
					<label
						className={cn(
							"flex items-start gap-3 rounded-md border p-3 cursor-pointer transition-colors",
							scope === "all" ? "border-primary bg-primary/5" : "hover:bg-muted/40",
						)}
					>
						<input
							type="radio"
							name="latency-scope"
							className="mt-1"
							checked={scope === "all"}
							disabled={isLoading || isSaving}
							onChange={() => setScope("all")}
						/>
						<span>
							<span className="font-medium text-sm">
								<Trans>All systems</Trans>
							</span>
							<p className="text-xs text-muted-foreground mt-0.5">
								<Trans>Every connected agent probes these targets.</Trans>
							</p>
						</span>
					</label>
					<label
						className={cn(
							"flex items-start gap-3 rounded-md border p-3 cursor-pointer transition-colors",
							scope === "selected" ? "border-primary bg-primary/5" : "hover:bg-muted/40",
						)}
					>
						<input
							type="radio"
							name="latency-scope"
							className="mt-1"
							checked={scope === "selected"}
							disabled={isLoading || isSaving}
							onChange={() => setScope("selected")}
						/>
						<span>
							<span className="font-medium text-sm">
								<Trans>Selected systems only</Trans>
							</span>
							<p className="text-xs text-muted-foreground mt-0.5">
								<Trans>Only checked machines probe; others stop hub-driven latency checks.</Trans>
							</p>
						</span>
					</label>
				</div>

				{scope === "selected" && (
					<div className="rounded-md border p-3 space-y-2">
						<div className="flex items-center justify-between gap-2">
							<span className="text-xs text-muted-foreground">
								{t`${selectedIds.size} selected`}
							</span>
							<div className="flex gap-2">
								<Button type="button" variant="ghost" size="sm" disabled={isLoading || isSaving} onClick={selectAllSystems}>
									<Trans>Select all</Trans>
								</Button>
								<Button type="button" variant="ghost" size="sm" disabled={isLoading || isSaving} onClick={clearSystems}>
									<Trans>Clear</Trans>
								</Button>
							</div>
						</div>
						{sortedSystems.length === 0 ? (
							<p className="text-sm text-muted-foreground py-2">
								<Trans>No systems yet.</Trans>
							</p>
						) : (
							<ul className="max-h-56 overflow-auto space-y-1.5">
								{sortedSystems.map((sys) => {
									const checked = selectedIds.has(sys.id)
									return (
										<li key={sys.id}>
											<label className="flex items-center gap-2.5 rounded-sm px-1 py-1.5 hover:bg-muted/50 cursor-pointer">
												<Checkbox
													checked={checked}
													disabled={isLoading || isSaving}
													onCheckedChange={(v) => toggleSystem(sys.id, v === true)}
												/>
												<span className="text-sm truncate">{sys.name}</span>
												<span className="text-xs text-muted-foreground ms-auto shrink-0">{sys.host}</span>
											</label>
										</li>
									)
								})}
							</ul>
						)}
					</div>
				)}
			</div>

			<Button onClick={save} disabled={isLoading || isSaving}>
				{isSaving ? <Trans>Saving...</Trans> : <Trans>Save Settings</Trans>}
			</Button>
		</div>
	)
}
