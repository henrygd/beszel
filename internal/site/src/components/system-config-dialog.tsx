import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { InputTags } from "@/components/ui/input-tags"
import { Label } from "@/components/ui/label"
import { useToast } from "@/components/ui/use-toast"
import { SystemMultiSelect } from "@/components/network-monitors-table/monitor-dialog"
import { pb } from "@/lib/api"
import { $systems } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { isAgentConfigUnsupported } from "@/lib/utils"
import type { SystemConfigRecord, SystemRecord } from "@/types"

/** Split a comma-separated setting, dropping blanks. */
const splitList = (value = "") =>
	value
		.split(",")
		.map((item) => item.trim())
		.filter(Boolean)

/** How many config writes to have in flight at once when saving many systems. */
const SAVE_CONCURRENCY = 10

/**
 * Dialog for editing settings the hub pushes to agents. Opens for one system; more systems can be
 * added from the systems dropdown to apply the same settings to all of them.
 * Settings live in each system's `system_config` record (one per system, one column per setting).
 *
 * Only settings the user actually changes are written, so other settings on each system are left
 * as they were. Render inside a `Dialog`, like `SystemDialog`.
 */
export const SystemConfigDialog = ({
	system,
	setOpen,
}: {
	/** the system the dialog was opened for; preselected */
	system: SystemRecord
	setOpen: (open: boolean) => void
}) => {
	const allSystems = useStore($systems)
	const [selectedIds, setSelectedIds] = useState(() => new Set([system.id]))
	const [configs, setConfigs] = useState<Map<string, SystemConfigRecord>>(new Map())
	// null until the user edits the field; only edited fields are written
	const [edited, setEdited] = useState<string[] | null>(null)
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const { toast } = useToast()

	const systems = useMemo(() => allSystems.filter((s) => selectedIds.has(s.id)), [allSystems, selectedIds])
	const multiple = systems.length > 1
	const outdatedCount = systems.filter(isAgentConfigUnsupported).length

	// Load every config record once. Systems that were never configured have none yet.
	useEffect(() => {
		let cancelled = false
		pb.collection<SystemConfigRecord>("system_config")
			.getFullList()
			.then((all) => {
				if (!cancelled) setConfigs(new Map(all.map((rec) => [rec.system, rec])))
			})
			.catch((err: { message?: string }) => {
				if (!cancelled) toast({ variant: "destructive", title: t`Error`, description: err.message })
			})
			.finally(() => !cancelled && setLoading(false))
		return () => {
			cancelled = true
		}
	}, [])

	// Current value of the setting across the selected systems. Normalised so "a,b" and "a, b" match.
	const current = useMemo(() => {
		const values = new Set(systems.map((s) => splitList(configs.get(s.id)?.exclude_containers).join(", ")))
		return { mixed: values.size > 1, list: values.size === 1 ? splitList([...values][0]) : [] }
	}, [systems, configs])

	const excludeContainers = edited ?? current.list

	const handleExcludeChange: React.Dispatch<React.SetStateAction<string[]>> = (value) =>
		setEdited(typeof value === "function" ? value(excludeContainers) : value)

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault()
		if (!edited) return
		setSaving(true)
		const data = {
			exclude_containers: edited
				.map((item) => item.trim())
				.filter(Boolean)
				.join(", "),
		}
		const save = (s: SystemRecord) => {
			const record = configs.get(s.id)
			return record
				? pb.collection("system_config").update(record.id, data)
				: pb.collection("system_config").create({ system: s.id, ...data })
		}
		try {
			const failed: string[] = []
			let firstError = ""
			for (let i = 0; i < systems.length; i += SAVE_CONCURRENCY) {
				const chunk = systems.slice(i, i + SAVE_CONCURRENCY)
				const results = await Promise.allSettled(chunk.map(save))
				results.forEach((result, j) => {
					if (result.status === "rejected") {
						failed.push(chunk[j].name)
						firstError ||= (result.reason as Error)?.message
					}
				})
			}
			if (failed.length) {
				toast({
					variant: "destructive",
					title: t`Failed to save settings for ${failed.length} of ${systems.length} systems`,
					description: `${failed.slice(0, 5).join(", ")}${failed.length > 5 ? ", …" : ""}: ${firstError}`,
				})
				return
			}
			setOpen(false)
		} finally {
			setSaving(false)
		}
	}

	return (
		<DialogContent className="w-[90%] max-w-md rounded-lg">
			<DialogHeader>
				<DialogTitle className="max-w-100 truncate pr-8">
					{multiple ? (
						<Trans>Agent settings for {systems.length} systems</Trans>
					) : (
						<Trans>Agent settings: {system.name}</Trans>
					)}
				</DialogTitle>
				<DialogDescription>
					<Trans>Settings sent from the hub to the agent. Changes apply without restarting the agent.</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="grid gap-4">
				{allSystems.length > 1 && (
					<div className="grid gap-2">
						<Label htmlFor="config-systems">
							<Trans>Systems</Trans>
						</Label>
						<SystemMultiSelect
							id="config-systems"
							selectedSystemIds={selectedIds}
							onChange={setSelectedIds}
							disabled={saving}
							isSelectable={() => true}
						/>
					</div>
				)}
				{outdatedCount > 0 && (
					<p className="text-sm text-amber-600 dark:text-amber-500">
						{multiple ? (
							<Plural
								value={outdatedCount}
								one="# selected agent is too old to receive these settings. Update it to version 0.20.0 or newer."
								other="# selected agents are too old to receive these settings. Update them to version 0.20.0 or newer."
							/>
						) : (
							<Trans>This agent is too old to receive these settings. Update it to version 0.20.0 or newer.</Trans>
						)}
					</p>
				)}
				<div className="grid gap-2">
					<Label htmlFor="exclude-containers">
						<Trans>Exclude containers</Trans>
					</Label>
					<InputTags
						id="exclude-containers"
						value={excludeContainers}
						onChange={handleExcludeChange}
						placeholder={current.mixed && !edited ? t`Different on each system` : "test-*"}
						autoComplete="off"
						spellCheck={false}
						disabled={loading}
						className="w-full"
					/>
					{current.mixed && (
						<p className="text-[0.8rem] text-amber-600 dark:text-amber-500 leading-relaxed">
							<Trans>
								These systems have different values. Changing this replaces the value on all {systems.length} systems.
							</Trans>
						</p>
					)}
					<p className="text-[0.8rem] text-muted-foreground leading-relaxed">
						<Trans>
							Container names to skip. Wildcards are supported. Add each with Tab, Enter or comma. Ignored if{" "}
							<code className="bg-muted px-1 rounded-sm">EXCLUDE_CONTAINERS</code> is set on the agent.
						</Trans>
					</p>
				</div>
				<DialogFooter>
					<Button type="submit" disabled={loading || saving || !edited || !systems.length}>
						{multiple ? <Trans>Apply to {systems.length} systems</Trans> : <Trans>Save Settings</Trans>}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
