import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { useEffect, useMemo, useState } from "react"
import { SystemMultiSelect } from "@/components/network-monitors-table/monitor-dialog"
import { Button } from "@/components/ui/button"
import { DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { InputTags } from "@/components/ui/input-tags"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useToast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import { $systems } from "@/lib/stores"
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

/** Settings stored as a comma-separated string in the system_config record. */
type SettingKey = "exclude_containers" | "service_patterns" | "nics"

/**
 * State for one list setting across the selected systems. `edited` stays null until the user
 * changes the field, and only edited settings are written on save.
 */
function useListSetting(key: SettingKey, systems: SystemRecord[], configs: Map<string, SystemConfigRecord>) {
	const [edited, setEdited] = useState<string[] | null>(null)

	// Current value across the selected systems. Normalised so "a,b" and "a, b" match.
	const current = useMemo(() => {
		const values = new Set(systems.map((s) => splitList(configs.get(s.id)?.[key]).join(", ")))
		return { mixed: values.size > 1, list: values.size === 1 ? splitList([...values][0]) : [] }
	}, [systems, configs, key])

	const value = edited ?? current.list
	const onChange: React.Dispatch<React.SetStateAction<string[]>> = (next) =>
		setEdited(typeof next === "function" ? next(value) : next)

	return {
		key,
		value,
		mixed: current.mixed,
		onChange,
		isEdited: edited !== null,
		toValue: () =>
			(edited ?? [])
				.map((item) => item.trim())
				.filter(Boolean)
				.join(", "),
	}
}

/** Shown when the selected systems have different values for a setting. */
const MixedNote = ({ systemCount }: { systemCount: number }) => (
	<p className="text-[0.8rem] text-amber-600 dark:text-amber-500 leading-relaxed">
		<Trans>These systems have different values. Changing this replaces the value on all {systemCount} systems.</Trans>
	</p>
)

/** Marks a tab that has unsaved changes. */
const EditedDot = () => <span className="size-1.5 rounded-full bg-primary" role="img" aria-label={t`Unsaved changes`} />

function ListSettingField({
	setting,
	label,
	placeholder,
	help,
	envVar,
	systemCount,
	disabled,
}: {
	setting: ReturnType<typeof useListSetting>
	label: React.ReactNode
	placeholder: string
	help: React.ReactNode
	/** env var that overrides this setting on the agent */
	envVar: string
	systemCount: number
	disabled: boolean
}) {
	const id = `config-${setting.key}`
	return (
		<div className="grid gap-2">
			<Label htmlFor={id}>{label}</Label>
			<InputTags
				id={id}
				value={setting.value}
				onChange={setting.onChange}
				placeholder={setting.mixed && !setting.isEdited ? t`Different on each system` : placeholder}
				autoComplete="off"
				spellCheck={false}
				disabled={disabled}
				className="w-full"
			/>
			{setting.mixed && <MixedNote systemCount={systemCount} />}
			<p className="text-[0.8rem] text-muted-foreground leading-relaxed">
				{help} <Trans>Add each with Tab, Enter or comma.</Trans>{" "}
				<Trans>
					Ignored if <code className="bg-muted px-1 rounded-sm">{envVar}</code> is set on the agent.
				</Trans>
			</p>
		</div>
	)
}

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
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const { toast } = useToast()

	const systems = useMemo(() => allSystems.filter((s) => selectedIds.has(s.id)), [allSystems, selectedIds])
	const multiple = systems.length > 1
	const outdatedCount = systems.filter(isAgentConfigUnsupported).length

	const excludeContainers = useListSetting("exclude_containers", systems, configs)
	const servicePatterns = useListSetting("service_patterns", systems, configs)
	const nics = useListSetting("nics", systems, configs)
	const settings = [excludeContainers, servicePatterns, nics]
	const hasChanges = settings.some((setting) => setting.isEdited)

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

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault()
		if (!hasChanges) return
		setSaving(true)
		const data: Partial<Record<SettingKey, string>> = {}
		for (const setting of settings) {
			if (setting.isEdited) data[setting.key] = setting.toValue()
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
				<Tabs defaultValue="container">
					<TabsList className="grid w-full grid-cols-3">
						<TabsTrigger value="container" className="gap-2">
							<Trans>Container</Trans>
							{excludeContainers.isEdited && <EditedDot />}
						</TabsTrigger>
						<TabsTrigger value="systemd" className="gap-2">
							Systemd
							{servicePatterns.isEdited && <EditedDot />}
						</TabsTrigger>
						<TabsTrigger value="network" className="gap-2">
							<Trans>Network</Trans>
							{nics.isEdited && <EditedDot />}
						</TabsTrigger>
					</TabsList>
					{/* forceMount keeps half-typed input in each field when switching tabs */}
					<TabsContent value="container" forceMount className="mt-4 data-[state=inactive]:hidden">
						<ListSettingField
							setting={excludeContainers}
							label={<Trans>Exclude containers</Trans>}
							placeholder="test-*"
							systemCount={systems.length}
							disabled={loading}
							help={
								<Trans>Container names to skip. Wildcards are supported. Leave empty to monitor all containers.</Trans>
							}
							envVar="EXCLUDE_CONTAINERS"
						/>
					</TabsContent>
					<TabsContent value="systemd" forceMount className="mt-4 data-[state=inactive]:hidden">
						<ListSettingField
							setting={servicePatterns}
							label={<Trans>Services to monitor</Trans>}
							placeholder="nginx, docker*"
							systemCount={systems.length}
							disabled={loading}
							help={
								<Trans>
									Services to monitor. Wildcards are supported, and{" "}
									<code className="bg-muted px-1 rounded-sm">.service</code> is added if missing. Leave empty to monitor
									all services.
								</Trans>
							}
							envVar="SERVICE_PATTERNS"
						/>
					</TabsContent>
					<TabsContent value="network" forceMount className="mt-4 data-[state=inactive]:hidden">
						<ListSettingField
							setting={nics}
							label={<Trans>Network interfaces</Trans>}
							placeholder="eth0, wlan*"
							systemCount={systems.length}
							disabled={loading}
							help={
								<Trans>
									Interfaces to monitor. Wildcards are supported. Start the first entry with{" "}
									<code className="bg-muted px-1 rounded-sm">-</code> to exclude the listed interfaces instead. Leave
									empty to detect interfaces automatically.
								</Trans>
							}
							envVar="NICS"
						/>
					</TabsContent>
				</Tabs>
				<DialogFooter>
					<Button type="submit" disabled={loading || saving || !hasChanges || !systems.length}>
						{multiple ? <Trans>Apply to {systems.length} systems</Trans> : <Trans>Save Settings</Trans>}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
