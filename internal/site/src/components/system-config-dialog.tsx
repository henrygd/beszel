import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { InputTags } from "@/components/ui/input-tags"
import { Label } from "@/components/ui/label"
import { useToast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import { isAgentConfigUnsupported } from "@/lib/utils"
import type { SystemConfigRecord, SystemRecord } from "@/types"

/** Split a comma-separated setting, dropping blanks. */
const splitList = (value = "") =>
	value
		.split(",")
		.map((item) => item.trim())
		.filter(Boolean)

/**
 * Dialog for editing settings the hub pushes to a system's agent.
 * Settings live in the system's `system_config` record (one per system, one column per setting).
 * Render inside a `Dialog`, like `SystemDialog`.
 */
export const SystemConfigDialog = ({ system, setOpen }: { system: SystemRecord; setOpen: (open: boolean) => void }) => {
	const [record, setRecord] = useState<SystemConfigRecord | null>(null)
	const [excludeContainers, setExcludeContainers] = useState<string[]>([])
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const { toast } = useToast()

	// Load the system's config record. Systems that were never configured have none yet.
	useEffect(() => {
		let cancelled = false
		pb.collection<SystemConfigRecord>("system_config")
			.getFirstListItem(pb.filter("system = {:id}", { id: system.id }))
			.then((rec) => {
				if (cancelled) return
				setRecord(rec)
				setExcludeContainers(splitList(rec.exclude_containers))
			})
			.catch((err: { status?: number; message?: string }) => {
				if (cancelled || err.status === 404) return
				toast({ variant: "destructive", title: t`Error`, description: err.message })
			})
			.finally(() => !cancelled && setLoading(false))
		return () => {
			cancelled = true
		}
	}, [system.id])

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault()
		setSaving(true)
		try {
			const data = {
				exclude_containers: excludeContainers
					.map((item) => item.trim())
					.filter(Boolean)
					.join(", "),
			}
			if (record) {
				await pb.collection("system_config").update(record.id, data)
			} else {
				await pb.collection("system_config").create({ system: system.id, ...data })
			}
			setOpen(false)
		} catch (err: unknown) {
			toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
		} finally {
			setSaving(false)
		}
	}

	return (
		<DialogContent className="w-[90%] max-w-md rounded-lg">
			<DialogHeader>
				<DialogTitle className="max-w-100 truncate pr-8">
					<Trans>Configure {system.name}</Trans>
				</DialogTitle>
				<DialogDescription>
					<Trans>Settings sent from the hub to the agent. Changes apply without restarting the agent.</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="grid gap-4">
				{isAgentConfigUnsupported(system) && (
					<p className="text-sm text-amber-600 dark:text-amber-500">
						<Trans>This agent is too old to receive these settings. Update it to version 0.20.0 or newer.</Trans>
					</p>
				)}
				<div className="grid gap-2">
					<Label htmlFor="exclude-containers">
						<Trans>Exclude containers</Trans>
					</Label>
					<InputTags
						id="exclude-containers"
						value={excludeContainers}
						onChange={setExcludeContainers}
						placeholder="test-*"
						autoComplete="off"
						spellCheck={false}
						disabled={loading}
						className="w-full"
					/>
					<p className="text-[0.8rem] text-muted-foreground leading-relaxed">
						<Trans>
							Container names to skip. Wildcards are supported. Add each with Tab, Enter or comma. Ignored if{" "}
							<code className="bg-muted px-1 rounded-sm">EXCLUDE_CONTAINERS</code> is set on the agent.
						</Trans>
					</p>
				</div>
				<DialogFooter>
					<Button type="submit" disabled={loading || saving}>
						<Trans>Save Settings</Trans>
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
