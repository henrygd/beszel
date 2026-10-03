import { useEffect, useRef, useState } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { pb } from "@/lib/api"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { ChevronDownIcon, GlobeIcon, ListIcon, PlusIcon } from "lucide-react"
import { useToast } from "@/components/ui/use-toast"
import { $systems } from "@/lib/stores"
import { cn, supportsNetworkMonitors } from "@/lib/utils"
import { getMonitorTarget } from "@/lib/network-monitor-utils"
import { MultiSelect, SystemMultiSelect } from "@/components/multi-select"
import type { NetworkMonitorRecord } from "@/types"
import * as v from "valibot"

type MonitorProtocol = "icmp" | "tcp" | "http" | "dns"

type MonitorValues = {
	system: string
	target: string
	protocol: MonitorProtocol
	port: number
	server: string
	interval: string
}

type NormalizedMonitorValues = Omit<MonitorValues, "system" | "interval"> & {
	interval: number
}

type BulkMonitorLineSource = Pick<NetworkMonitorRecord, "target" | "protocol" | "port" | "interval" | "server">

const defaultInterval = 30

const MonitorProtocolSchema = v.picklist(["icmp", "tcp", "http", "dns"])

const MonitorIntervalSchema = v.pipe(v.string(), v.toNumber(), v.minValue(1), v.maxValue(3600))

// Both the single-monitor form and the bulk importer flow through this schema so
// defaults and HTTP target normalization stay in one place.
const NormalizedMonitorValuesSchema = v.pipe(
	v.object({
		target: v.pipe(v.string(), v.trim(), v.nonEmpty("target is required")),
		protocol: MonitorProtocolSchema,
		port: v.number(),
		server: v.pipe(v.string(), v.trim()),
		interval: MonitorIntervalSchema,
	}),
	v.transform((input): NormalizedMonitorValues => {
		let { protocol, port } = input
		let httpTarget = input.target
		if (protocol === "icmp" || protocol === "http" || protocol === "dns") {
			if (protocol === "http") {
				httpTarget = normalizeHttpTarget(input.target, port)
			}
			port = 0
		} else if (protocol === "tcp" && !port) {
			port = 443
		}
		return {
			// HTTP monitors may be entered as bare hostnames, so normalize them to a
			// scheme-bearing URL before the payload is sent to PocketBase.
			target: protocol === "http" ? httpTarget : input.target,
			protocol,
			port,
			// Only DNS monitors use a custom server; clear it for other protocols.
			server: protocol === "dns" ? input.server : "",
			interval: input.interval,
		}
	}),
	v.forward(
		v.check((input) => {
			if (input.protocol === "icmp" || input.protocol === "http" || input.protocol === "dns") {
				return input.port === 0
			}

			return Number.isInteger(input.port) && input.port >= 1 && input.port <= 65535
		}, "Port must be between 1 and 65535"),
		["port"]
	),
	// Resolving an IP literal returns it without querying anything, so the check would measure nothing.
	v.forward(
		v.check(
			(input) => input.protocol !== "dns" || !isIpAddress(input.target),
			"DNS target must be a domain name; put the resolver's IP in DNS Server"
		),
		["target"]
	)
)

function isIpAddress(value: string) {
	// Hostnames never contain ":", so any colon means an IPv6 literal (optionally bracketed).
	return /^(\d{1,3}\.){3}\d{1,3}$/.test(value) || value.includes(":")
}

// Bulk parsing only trims raw CSV fields. Inference, defaults, and protocol-
// specific validation still go through the shared normalization schema above.
const BulkMonitorSchema = v.object({
	target: v.pipe(v.string(), v.trim(), v.nonEmpty("target is required")),
	protocol: v.optional(v.pipe(v.string(), v.trim())),
	port: v.optional(v.pipe(v.string(), v.trim())),
	interval: v.optional(v.pipe(v.string(), v.trim())),
	server: v.optional(v.pipe(v.string(), v.trim())),
})

function normalizeHttpTarget(target: string, port = 0) {
	const useExplicitPort = port > 0 && port !== 80 && port !== 443
	const hasOriginOnlyTarget = /^https?:\/\/[^/?#]+$/i.test(target)
	if (!/^https?:\/\//i.test(target)) {
		const scheme = port === 80 ? "http" : "https"
		return `${scheme}://${target}${useExplicitPort ? `:${port}` : ""}`
	}

	let parsedUrl: URL
	try {
		parsedUrl = new URL(target)
	} catch {
		return target
	}

	if (!parsedUrl.port && useExplicitPort) {
		parsedUrl.port = `${port}`
	}

	// avoid converting "http://localhost:8090" to "http://localhost:8090/" - keep the original formatting if the URL is just an origin
	if (hasOriginOnlyTarget && parsedUrl.pathname === "/" && !parsedUrl.search && !parsedUrl.hash) {
		return parsedUrl.origin
	}

	return parsedUrl.toString()
}

function trimTrailingEmptyFields(fields: string[]) {
	let lastValueIndex = fields.length - 1
	while (lastValueIndex > 0 && !fields[lastValueIndex]) {
		lastValueIndex--
	}
	return fields.slice(0, lastValueIndex + 1)
}

function buildMonitorPayload(values: MonitorValues, enabled = true) {
	const normalizedValues = v.safeParse(NormalizedMonitorValuesSchema, values)
	if (!normalizedValues.success) {
		throw new Error(normalizedValues.issues[0]?.message || "Invalid monitor")
	}

	const payload = {
		system: values.system,
		enabled,
		...normalizedValues.output,
	}

	return payload
}

type MonitorIdentity = Pick<MonitorValues, "system" | "target" | "protocol" | "port" | "server">
function getMonitorIdentityKey({ system, target, protocol, port, server }: MonitorIdentity) {
	return `${system}${target}${protocol}${port}${protocol === "dns" ? server : ""}`
}

function parseBulkMonitorLine(line: string, lineNumber: number, system: string) {
	const [rawTarget = "", rawProtocol = "", rawPort = "", rawInterval = "", rawServer = ""] = line.split(",")
	const parsed = v.safeParse(BulkMonitorSchema, {
		target: rawTarget,
		protocol: rawProtocol,
		port: rawPort,
		interval: rawInterval,
		server: rawServer,
	})
	if (!parsed.success) {
		throw new Error(`Line ${lineNumber}: ${parsed.issues[0]?.message || "invalid monitor entry"}`)
	}
	const protocol = (parsed.output.protocol?.toLowerCase() ||
		(/^https?:\/\//i.test(parsed.output.target) ? "http" : "icmp")) as MonitorProtocol

	return buildMonitorPayload({
		system,
		target: parsed.output.target,
		protocol,
		port: parsed.output.port ? Number(parsed.output.port) : 0,
		server: parsed.output.server || "",
		interval: parsed.output.interval || `${defaultInterval}`,
	})
}

export function formatBulkMonitorLine(monitor: BulkMonitorLineSource) {
	const port = monitor.protocol !== "tcp" || monitor.port === 443 ? "" : `${monitor.port}`
	const interval = monitor.interval === defaultInterval ? "" : `${monitor.interval}`
	const server = monitor.protocol !== "dns" ? "" : monitor.server
	return trimTrailingEmptyFields([monitor.target, monitor.protocol, port, interval, server]).join(",")
}

/** Pick monitors by target, e.g. other targets on the same system to compare against. */
export function MonitorMultiSelect({
	id,
	monitors,
	selectedMonitorIds,
	onChange,
	disabled,
	className,
	placeholder,
	canSelectMore,
}: {
	id: string
	monitors: NetworkMonitorRecord[]
	selectedMonitorIds: Set<string>
	onChange: (ids: Set<string>) => void
	disabled?: boolean
	className?: string
	placeholder?: string
	canSelectMore?: boolean
}) {
	const { t } = useLingui()
	const options = monitors
		.map((monitor) => ({ id: monitor.id, label: getMonitorTarget(monitor), server: monitor.server }))
		.sort((a, b) => a.label.localeCompare(b.label))
	return (
		<MultiSelect
			id={id}
			options={options}
			selectedIds={selectedMonitorIds}
			onChange={onChange}
			disabled={disabled}
			className={cn("ps-9.5", className)}
			icon={GlobeIcon}
			canSelectMore={canSelectMore}
			placeholder={placeholder ?? t`Select targets`}
			searchPlaceholder={t`Search targets`}
			emptyText={<Trans>No targets found.</Trans>}
			renderOption={(option) => (
				<>
					<span className="truncate">{option.label}</span>
					{option.server && <span className="ms-auto shrink-0 text-xs text-muted-foreground">{option.server}</span>}
				</>
			)}
		/>
	)
}

export function AddMonitorDialog({ systemId, monitors }: { systemId?: string; monitors: NetworkMonitorRecord[] }) {
	const [open, setOpen] = useState(false)
	const [bulkOpen, setBulkOpen] = useState(false)
	const [bulkInput, setBulkInput] = useState("")
	const [bulkLoading, setBulkLoading] = useState(false)
	const [bulkSelectedSystemIds, setBulkSelectedSystemIds] = useState<Set<string>>(new Set())
	const bulkFormRef = useRef<HTMLFormElement>(null)
	const { toast } = useToast()
	const { t } = useLingui()
	const systems = useStore($systems)
	const hasEligibleSystems = systemId ? true : systems.some(supportsNetworkMonitors)

	const resetBulkForm = () => {
		setBulkInput("")
	}

	const openBulkAdd = (selectedSystemIds?: Set<string>) => {
		if (!systemId && selectedSystemIds) {
			setBulkSelectedSystemIds(new Set(selectedSystemIds))
		}
		setOpen(false)
		setBulkOpen(true)
	}

	const openAdd = () => {
		setBulkOpen(false)
		setOpen(true)
	}

	async function handleBulkSubmit(e: React.FormEvent) {
		e.preventDefault()
		setBulkLoading(true)
		let closedForSubmit = false

		try {
			const targetSystems = systemId ? [systemId] : Array.from(bulkSelectedSystemIds)
			if (!targetSystems.length) {
				throw new Error("Select at least one system.")
			}
			const rawLines = bulkInput.split(/\r?\n/).filter((line) => line.trim())
			if (!rawLines.length) {
				throw new Error("Enter at least one monitor.")
			}

			let totalCreated = 0
			closedForSubmit = true

			for (const system of targetSystems) {
				const payloads = rawLines.map((line, index) => parseBulkMonitorLine(line, index + 1, system))
				const existingMonitorKeys = new Set(
					monitors.filter((monitor) => monitor.system === system).map((monitor) => getMonitorIdentityKey(monitor))
				)
				const newPayloads: typeof payloads = []

				for (const payload of payloads) {
					const monitorKey = getMonitorIdentityKey(payload)
					if (existingMonitorKeys.has(monitorKey)) {
						continue
					}
					existingMonitorKeys.add(monitorKey)
					newPayloads.push(payload)
				}

				if (!newPayloads.length) continue

				let batch = pb.createBatch()
				let inBatch = 0
				for (const payload of newPayloads) {
					batch.collection("network_monitors").create(payload)
					inBatch++
					if (inBatch > 20) {
						await batch.send()
						batch = pb.createBatch()
						inBatch = 0
					}
				}
				if (inBatch) {
					await batch.send()
				}
				totalCreated += newPayloads.length
			}

			if (!totalCreated) {
				throw new Error("No new monitors. All entries already exist.")
			}

			resetBulkForm()
			toast({ title: t`Monitors created`, description: `${totalCreated} monitor(s) added.` })
		} catch (err: unknown) {
			if (closedForSubmit) {
				setBulkOpen(true)
			}
			toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
		} finally {
			setBulkLoading(false)
		}
	}

	return (
		<>
			<div className="flex gap-0 rounded-lg">
				<Button variant="outline" onClick={openAdd} className="rounded-e-none grow" disabled={!hasEligibleSystems}>
					<PlusIcon className="size-4 me-1" />
					<span className="sm:hidden">
						<Trans>Add</Trans>
					</span>
					<span className="hidden sm:inline">
						<Trans>Add {{ foo: t`Monitor` }}</Trans>
					</span>
				</Button>
				<div className="w-px h-full bg-muted"></div>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							variant="outline"
							className="px-2 rounded-s-none border-s-0"
							aria-label={`More actions`}
							disabled={!hasEligibleSystems}
						>
							<ChevronDownIcon className="size-4" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem onClick={() => openBulkAdd()}>
							<ListIcon className="size-4 me-2" />
							<Trans>Bulk Add</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</div>
			<Dialog
				open={open}
				onOpenChange={(nextOpen) => {
					setOpen(nextOpen)
				}}
			>
				<MonitorDialogContent open={open} setOpen={setOpen} systemId={systemId} onOpenBulkAdd={openBulkAdd} />
			</Dialog>

			<Sheet
				open={bulkOpen}
				onOpenChange={(nextOpen) => {
					setBulkOpen(nextOpen)
					if (!nextOpen) {
						resetBulkForm()
					}
				}}
			>
				<SheetContent className="w-full sm:max-w-xl gap-0">
					<SheetHeader className="border-b">
						<SheetTitle>
							<Trans>Bulk Add {{ foo: t`Network Monitors` }}</Trans>
						</SheetTitle>
						<SheetDescription>
							<Trans>target[,protocol[,port[,interval[,server]]]]</Trans>
						</SheetDescription>
					</SheetHeader>
					<form ref={bulkFormRef} onSubmit={handleBulkSubmit} className="flex h-full flex-col overflow-hidden">
						<div className="flex-1 flex flex-col space-y-4 overflow-auto p-4">
							{!systemId && (
								<div className="grid gap-2">
									<Label htmlFor="bulk-monitor-systems" className="sr-only">
										<Trans>Systems</Trans>
									</Label>
									<SystemMultiSelect
										id="bulk-monitor-systems"
										selectedSystemIds={bulkSelectedSystemIds}
										onChange={setBulkSelectedSystemIds}
										disabled={bulkLoading}
										className="bg-card"
									/>
								</div>
							)}
							<div className="grow flex flex-col gap-2">
								<Label htmlFor="bulk-monitors" className="sr-only">
									Entries
								</Label>
								<Textarea
									id="bulk-monitors"
									value={bulkInput}
									onChange={(e) => setBulkInput(e.target.value)}
									onKeyDown={(e) => {
										if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
											e.preventDefault()
											bulkFormRef.current?.requestSubmit()
										}
									}}
									className="font-mono grow text-sm bg-card"
									placeholder={[
										"1.1.1.1",
										"example.com,tcp",
										"https://example.com,http,,60",
										"example.com,dns,,,1.1.1.1",
									].join("\n")}
									required
								/>
								<p className="text-xs text-muted-foreground">
									<Trans>target[,protocol[,port[,interval[,server]]]]</Trans>
								</p>
							</div>
						</div>
						<SheetFooter className="border-t">
							<Button type="submit" disabled={bulkLoading || (!systemId && !bulkSelectedSystemIds.size)}>
								<Trans>Add {{ foo: t`Network Monitors` }}</Trans>
							</Button>
						</SheetFooter>
					</form>
				</SheetContent>
			</Sheet>
		</>
	)
}

export function EditMonitorDialog({
	open,
	setOpen,
	systemId,
	monitor,
}: {
	open: boolean
	setOpen: (open: boolean) => void
	systemId?: string
	monitor?: NetworkMonitorRecord
}) {
	const hasOpened = useRef(false)
	if (!monitor && !hasOpened.current) {
		return null
	}
	hasOpened.current = true
	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<MonitorDialogContent open={open} setOpen={setOpen} systemId={systemId} monitor={monitor} />
		</Dialog>
	)
}

function MonitorDialogContent({
	open,
	setOpen,
	systemId,
	monitor,
	onOpenBulkAdd,
}: {
	open: boolean
	setOpen: (open: boolean) => void
	systemId?: string
	monitor?: NetworkMonitorRecord
	onOpenBulkAdd?: (selectedSystemIds: Set<string>) => void
}) {
	const [protocol, setProtocol] = useState<MonitorProtocol>(monitor?.protocol ?? "icmp")
	const [target, setTarget] = useState(monitor?.target ?? "")
	const [port, setPort] = useState(monitor?.protocol === "tcp" && monitor.port ? String(monitor.port) : "")
	const [server, setServer] = useState(monitor?.protocol === "dns" ? (monitor.server ?? "") : "")
	const [monitorInterval, setMonitorInterval] = useState(String(monitor?.interval ?? defaultInterval))
	const [loading, setLoading] = useState(false)
	const [selectedSystemId, setSelectedSystemId] = useState(monitor?.system ?? "")
	const [selectedSystemIds, setSelectedSystemIds] = useState<Set<string>>(new Set())
	const systems = useStore($systems)
	const { toast } = useToast()
	const { t } = useLingui()
	const isEditing = !!monitor
	const dnsTargetIsIp = protocol === "dns" && isIpAddress(target.trim())

	// When the dialog is opened, initialize form fields with monitor values (if editing) or defaults (if adding).
	useEffect(() => {
		if (!open) {
			return
		}

		setProtocol(monitor?.protocol ?? "icmp")
		setTarget(monitor?.target ?? "")
		setPort(monitor?.protocol === "tcp" && monitor.port ? String(monitor.port) : "")
		setServer(monitor?.protocol === "dns" ? (monitor.server ?? "") : "")
		setMonitorInterval(String(monitor?.interval ?? defaultInterval))
		setSelectedSystemId(monitor?.system ?? "")
		setSelectedSystemIds(new Set())
		setLoading(false)
	}, [open, monitor])

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault()
		setLoading(true)

		const targetSystems = systemId ? [systemId] : monitor ? [selectedSystemId] : Array.from(selectedSystemIds)
		const remainingSystemIds = new Set(targetSystems)
		try {
			if (!targetSystems.length || !targetSystems[0]) throw new Error("Select at least one system.")
			const payload = buildMonitorPayload(
				{
					system: targetSystems[0],
					target,
					protocol,
					port: protocol === "tcp" ? Number(port) : 0,
					server: protocol === "dns" ? server.trim() : "",
					interval: monitorInterval,
				},
				monitor ? monitor.enabled : true
			)
			if (monitor) {
				await pb.collection("network_monitors").update(monitor.id, payload)
			} else {
				for (const system of targetSystems) {
					await pb.collection("network_monitors").create({ ...payload, system })
					remainingSystemIds.delete(system)
				}
			}
			setOpen(false)
		} catch (err: unknown) {
			if (!monitor && !systemId) {
				// Retain only unfinished systems so retrying cannot duplicate successful creates.
				setSelectedSystemIds(remainingSystemIds)
			}
			toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
		} finally {
			setLoading(false)
		}
	}

	return (
		<DialogContent className="max-w-md">
			<DialogHeader>
				<DialogTitle>
					{isEditing ? (
						<Trans>Edit {{ foo: t`Network Monitor` }}</Trans>
					) : (
						<Trans>Add {{ foo: t`Network Monitor` }}</Trans>
					)}
				</DialogTitle>
				<DialogDescription>
					<Trans>Configure response monitoring from this agent.</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="grid gap-4 tabular-nums">
				{!systemId && !isEditing && (
					<div className="grid gap-2">
						<Label htmlFor="monitor-systems">
							<Trans>Systems</Trans>
						</Label>
						<SystemMultiSelect
							id="monitor-systems"
							selectedSystemIds={selectedSystemIds}
							onChange={setSelectedSystemIds}
							disabled={loading}
						/>
					</div>
				)}
				{!systemId && isEditing && (
					<div className="grid gap-2">
						<Label>
							<Trans>System</Trans>
						</Label>
						<Select value={selectedSystemId} onValueChange={setSelectedSystemId} required>
							<SelectTrigger>
								<SelectValue placeholder={t`Select a system`} />
							</SelectTrigger>
							<SelectContent>
								{systems
									.filter((sys) => sys.id === monitor?.system || supportsNetworkMonitors(sys))
									.map((sys) => (
										<SelectItem key={sys.id} value={sys.id}>
											{sys.name}
										</SelectItem>
									))}
							</SelectContent>
						</Select>
					</div>
				)}
				<div className="grid gap-2">
					<Label>
						<Trans>Target</Trans>
					</Label>
					<Input
						value={target}
						onChange={(e) => setTarget(e.target.value)}
						placeholder={protocol === "http" ? "http://localhost:8090" : protocol === "dns" ? "example.com" : "1.1.1.1"}
						aria-invalid={dnsTargetIsIp}
						required
					/>
					{dnsTargetIsIp && (
						<p className="text-xs text-destructive">
							<Trans>Enter a domain name to look up. Put the resolver's IP in DNS Server.</Trans>
						</p>
					)}
				</div>
				<div className="grid gap-2">
					<Label>
						<Trans>Protocol</Trans>
					</Label>

					<Select value={protocol} onValueChange={(value) => setProtocol(value as MonitorProtocol)}>
						<SelectTrigger>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="icmp">ICMP</SelectItem>
							<SelectItem value="tcp">TCP</SelectItem>
							<SelectItem value="http">HTTP</SelectItem>
							<SelectItem value="dns">DNS</SelectItem>
						</SelectContent>
					</Select>
				</div>
				{protocol === "tcp" && (
					<div className="grid gap-2">
						<Label>
							<Trans>Port</Trans>
						</Label>
						<Input
							type="number"
							value={port}
							onChange={(e) => setPort(e.target.value)}
							placeholder="443"
							min={1}
							max={65535}
						/>
					</div>
				)}
				{protocol === "dns" && (
					<div className="grid gap-2">
						<Label>
							<Trans>DNS Server</Trans>
						</Label>
						<Input value={server} onChange={(e) => setServer(e.target.value)} placeholder="1.1.1.1" />
						<p className="text-xs text-muted-foreground">
							<Trans>Optional. Defaults to the agent's system resolver.</Trans>
						</p>
					</div>
				)}
				<div className="grid gap-2">
					<Label>
						<Trans>Interval (seconds)</Trans>
					</Label>
					<Input
						type="number"
						value={monitorInterval}
						onChange={(e) => setMonitorInterval(e.target.value)}
						min={1}
						max={3600}
						required
					/>
				</div>
				<DialogFooter>
					{!isEditing && onOpenBulkAdd && (
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenBulkAdd(selectedSystemIds)}
							disabled={loading}
							className="me-auto"
						>
							<ListIcon className="size-4 me-2" />
							<Trans>Bulk Add</Trans>
						</Button>
					)}
					<Button
						type="submit"
						disabled={
							loading || dnsTargetIsIp || (!systemId && (isEditing ? !selectedSystemId : !selectedSystemIds.size))
						}
					>
						{isEditing ? <Trans>Save {{ foo: t`Monitor` }}</Trans> : <Trans>Add {{ foo: t`Monitor` }}</Trans>}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
