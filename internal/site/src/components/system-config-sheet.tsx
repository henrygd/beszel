import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ChevronDownIcon, ContainerIcon, type LucideIcon, NetworkIcon, ServerCogIcon } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { SystemMultiSelect } from "@/components/network-monitors-table/monitor-dialog"
import { Button } from "@/components/ui/button"
import { InputTags } from "@/components/ui/input-tags"
import { Label } from "@/components/ui/label"
import { SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { useToast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import { $systems } from "@/lib/stores"
import { cn, isAgentConfigUnsupported } from "@/lib/utils"
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

/**
 * One setting as a collapsible card, styled like the cards in the alerts sheet.
 * Collapsed, the header shows the current value instead of the help text.
 */
function ListSettingField({
	setting,
	icon: Icon,
	label,
	placeholder,
	help,
	emptySummary,
	envVar,
	systemCount,
	disabled,
}: {
	setting: ReturnType<typeof useListSetting>
	icon: LucideIcon
	label: React.ReactNode
	placeholder: string
	help: React.ReactNode
	/** summary shown while collapsed if the setting is empty */
	emptySummary: React.ReactNode
	/** env var that overrides this setting on the agent */
	envVar: string
	systemCount: number
	disabled: boolean
}) {
	const [open, setOpen] = useState(false)
	const id = `config-${setting.key}`
	const summary =
		setting.mixed && !setting.isEdited ? (
			<span className="text-amber-600 dark:text-amber-500">
				<Trans>Different on each system</Trans>
			</span>
		) : setting.value.length ? (
			setting.value.join(", ")
		) : (
			emptySummary
		)
	return (
		<div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100">
			<button
				type="button"
				aria-expanded={open}
				aria-controls={`${id}-content`}
				onClick={() => setOpen(!open)}
				className={cn("flex w-full items-center justify-between gap-4 p-4 text-start cursor-pointer", open && "pb-0")}
			>
				<div className="grid gap-1 min-w-0 select-none">
					<p id={`${id}-label`} className="font-semibold flex gap-3 items-center">
						<Icon className="h-4 w-4 opacity-85" /> {label}
						{setting.isEdited && (
							<span className="size-1.5 rounded-full bg-primary" role="img" aria-label={t`Unsaved changes`} />
						)}
					</p>
					{!open && <span className="block text-sm text-muted-foreground truncate">{summary}</span>}
				</div>
				<ChevronDownIcon
					className={cn("h-4 w-4 shrink-0 opacity-70 transition-transform duration-200", open && "rotate-180")}
				/>
			</button>
			{/* Hidden rather than unmounted so half-typed input survives collapsing */}
			<div id={`${id}-content`} className={cn("grid gap-3 p-4 pt-3", !open && "hidden")}>
				<p className="text-sm text-muted-foreground">{help}</p>
				<InputTags
					id={id}
					aria-labelledby={`${id}-label`}
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
					<Trans>Add each with Tab, Enter or comma.</Trans>{" "}
					<Trans>
						Ignored if <code className="bg-muted px-1 rounded-sm">{envVar}</code> is set on the agent.
					</Trans>
				</p>
			</div>
		</div>
	)
}

/**
 * Sheet for editing settings the hub pushes to agents. Opens for one system; more systems can be
 * added from the systems dropdown to apply the same settings to all of them.
 * Settings live in each system's `system_config` record (one per system, one column per setting).
 *
 * Only settings the user actually changes are written, so other settings on each system are left
 * as they were. Render inside a `Sheet`.
 */
export const SystemConfigSheet = ({
	system,
	setOpen,
}: {
	/** the system the sheet was opened for; preselected */
	system: SystemRecord
	setOpen: (open: boolean) => void
}) => {
	const allSystems = useStore($systems)
	const [selectedIds, setSelectedIds] = useState(() => new Set([system.id]))
	const [configs, setConfigs] = useState<Map<string, SystemConfigRecord>>(new Map())
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const { toast } = useToast()

	const allSystemIds = useMemo(() => allSystems.map((s) => s.id), [allSystems])
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
		<SheetContent className="w-160 !max-w-full gap-0">
			<SheetHeader className="p-4 sm:p-6 pb-3 sm:pb-4 border-b">
				<SheetTitle className="text-xl truncate pe-8">
					{multiple ? (
						<Trans>Agent settings for {systems.length} systems</Trans>
					) : (
						<Trans>Agent settings: {system.name}</Trans>
					)}
				</SheetTitle>
				<SheetDescription>
					<Trans>Settings sent from the hub to the agent. Changes apply without restarting the agent.</Trans>
				</SheetDescription>
			</SheetHeader>
			<form onSubmit={handleSubmit} className="flex h-full flex-col overflow-hidden">
				<div className="flex-1 grid content-start gap-4 overflow-auto p-4 sm:p-6">
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
								systemIds={allSystemIds}
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
					<div className="grid gap-3">
						<ListSettingField
							setting={excludeContainers}
							icon={ContainerIcon}
							emptySummary={<Trans>All containers</Trans>}
							label={<Trans>Exclude containers</Trans>}
							placeholder="test-*"
							systemCount={systems.length}
							disabled={loading}
							help={
								<Trans>Container names to skip. Wildcards are supported. Leave empty to monitor all containers.</Trans>
							}
							envVar="EXCLUDE_CONTAINERS"
						/>
						<ListSettingField
							setting={servicePatterns}
							icon={ServerCogIcon}
							emptySummary={<Trans>All services</Trans>}
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
						<ListSettingField
							setting={nics}
							icon={NetworkIcon}
							emptySummary={<Trans>Detected automatically</Trans>}
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
					</div>
				</div>
				<SheetFooter className="border-t sm:px-6">
					<Button type="submit" disabled={loading || saving || !hasChanges || !systems.length}>
						{multiple ? <Trans>Apply to {systems.length} systems</Trans> : <Trans>Save Settings</Trans>}
					</Button>
				</SheetFooter>
			</form>
		</SheetContent>
	)
}
