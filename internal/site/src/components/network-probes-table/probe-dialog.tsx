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
import { ChevronDownIcon, ListIcon, ServerIcon } from "lucide-react"
import { useToast } from "@/components/ui/use-toast"
import { $systems } from "@/lib/stores"
import type { NetworkProbeRecord } from "@/types"
import * as v from "valibot"

type ProbeProtocol = "icmp" | "tcp" | "http"

type ProbeValues = {
	system: string
	target: string
	protocol: ProbeProtocol
	port: number
	interval: string
	name?: string
}

type NormalizedProbeValues = Omit<ProbeValues, "system" | "interval"> & {
	interval: number
}

type BulkProbeLineSource = Pick<NetworkProbeRecord, "target" | "protocol" | "port" | "interval" | "name">

const defaultInterval = 30

const ProbeProtocolSchema = v.picklist(["icmp", "tcp", "http"])

const ProbeIntervalSchema = v.pipe(v.string(), v.toNumber(), v.minValue(1), v.maxValue(3600))

// Both the single-probe form and the bulk importer flow through this schema so
// defaults and HTTP target normalization stay in one place.
const NormalizedProbeValuesSchema = v.pipe(
	v.object({
		target: v.pipe(v.string(), v.trim(), v.nonEmpty("target is required")),
		protocol: ProbeProtocolSchema,
		port: v.number(),
		interval: ProbeIntervalSchema,
		name: v.optional(v.pipe(v.string(), v.trim())),
	}),
	v.transform((input): NormalizedProbeValues => {
		let { protocol, port } = input
		if (protocol === "icmp") {
			port = 0
		} else if ((protocol === "tcp" || protocol === "http") && !port) {
			port = 443
		}
		return {
			// HTTP probes may be entered as bare hostnames, so normalize them to a
			// scheme-bearing URL before the payload is sent to PocketBase.
			target: protocol === "http" ? normalizeHttpTarget(input.target, port) : input.target,
			protocol,
			port,
			interval: input.interval,
			name: input.name || undefined,
		}
	}),
	v.forward(
		v.check((input) => {
			if (input.protocol === "icmp") {
				return input.port === 0
			}

			return Number.isInteger(input.port) && input.port >= 1 && input.port <= 65535
		}, "Port must be between 1 and 65535"),
		["port"]
	)
)

// Bulk parsing only trims raw CSV fields. Inference, defaults, and protocol-
// specific validation still go through the shared normalization schema above.
const BulkProbeSchema = v.object({
	target: v.pipe(v.string(), v.trim(), v.nonEmpty("target is required")),
	protocol: v.optional(v.pipe(v.string(), v.trim())),
	port: v.optional(v.pipe(v.string(), v.trim())),
	interval: v.optional(v.pipe(v.string(), v.trim())),
	name: v.optional(v.pipe(v.string(), v.trim())),
})

function normalizeHttpTarget(target: string, port: number) {
	if (/^https?:\/\//i.test(target)) {
		return target
	}

	return `${port === 443 ? "https" : "http"}://${target}`
}

function trimTrailingEmptyFields(fields: string[]) {
	let lastValueIndex = fields.length - 1
	while (lastValueIndex > 0 && !fields[lastValueIndex]) {
		lastValueIndex--
	}
	return fields.slice(0, lastValueIndex + 1)
}

function buildProbePayload(values: ProbeValues, enabled = true) {
	const normalizedValues = v.safeParse(NormalizedProbeValuesSchema, values)
	if (!normalizedValues.success) {
		throw new Error(normalizedValues.issues[0]?.message || "Invalid probe")
	}

	const payload = {
		system: values.system,
		enabled,
		...normalizedValues.output,
	}

	const trimmedName = normalizedValues.output.name?.trim()
	const targetName = normalizedValues.output.target.replace(/^https?:\/\//i, "")
	if (trimmedName) {
		payload.name = trimmedName
	} else if (targetName !== normalizedValues.output.target) {
		payload.name = targetName
	} else {
		payload.name = ""
	}
	return payload
}

type ProbeIdentity = Pick<ProbeValues, "system" | "target" | "protocol" | "port">
function getProbeIdentityKey({ system, target, protocol, port }: ProbeIdentity) {
	return `${system}${target}${protocol}${port}`
}

function parseBulkProbeLine(line: string, lineNumber: number, system: string) {
	const [rawTarget = "", rawProtocol = "", rawPort = "", rawInterval = "", ...rawName] = line.split(",")
	const parsed = v.safeParse(BulkProbeSchema, {
		target: rawTarget,
		protocol: rawProtocol,
		port: rawPort,
		interval: rawInterval,
		name: rawName.join(","),
	})
	if (!parsed.success) {
		throw new Error(`Line ${lineNumber}: ${parsed.issues[0]?.message || "invalid probe entry"}`)
	}

	return buildProbePayload({
		system,
		target: parsed.output.target,
		protocol: (parsed.output.protocol?.toLowerCase() ||
			(/^https?:\/\//i.test(parsed.output.target) ? "http" : "icmp")) as ProbeProtocol,
		port: parsed.output.port ? Number(parsed.output.port) : 0,
		interval: parsed.output.interval || `${defaultInterval}`,
		name: parsed.output.name || undefined,
	})
}

export function formatBulkProbeLine(probe: BulkProbeLineSource) {
	const port = probe.protocol === "icmp" || probe.port === 443 ? "" : `${probe.port}`
	const interval = probe.interval === defaultInterval ? "" : `${probe.interval}`
	return trimTrailingEmptyFields([probe.target, probe.protocol, port, interval, probe.name?.trim() || ""]).join(",")
}

export function AddProbeDialog({ systemId, probes }: { systemId?: string; probes: NetworkProbeRecord[] }) {
	const [open, setOpen] = useState(false)
	const [bulkOpen, setBulkOpen] = useState(false)
	const [bulkInput, setBulkInput] = useState("")
	const [bulkLoading, setBulkLoading] = useState(false)
	const [bulkSelectedSystemId, setBulkSelectedSystemId] = useState("")
	const bulkFormRef = useRef<HTMLFormElement>(null)
	const { toast } = useToast()
	const { t } = useLingui()
	const systems = useStore($systems)

	const resetBulkForm = () => {
		setBulkInput("")
		// setBulkSelectedSystemId("")
	}

	const openBulkAdd = (selectedSystemId?: string) => {
		if (!systemId && selectedSystemId) {
			setBulkSelectedSystemId(selectedSystemId)
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
			const system = systemId ?? bulkSelectedSystemId
			if (!system) {
				throw new Error("Select a system.")
			}
			const rawLines = bulkInput.split(/\r?\n/).filter((line) => line.trim())
			if (!rawLines.length) {
				throw new Error("Enter at least one probe.")
			}

			const payloads = rawLines.map((line, index) => parseBulkProbeLine(line, index + 1, system))
			const existingProbeKeys = new Set(
				probes.filter((probe) => probe.system === system).map((probe) => getProbeIdentityKey(probe))
			)
			const newPayloads = [] as typeof payloads

			for (const payload of payloads) {
				const probeKey = getProbeIdentityKey(payload)
				if (existingProbeKeys.has(probeKey)) {
					continue
				}

				existingProbeKeys.add(probeKey)
				newPayloads.push(payload)
			}

			if (!newPayloads.length) {
				throw new Error("No new probes. All entries exist.")
			}

			closedForSubmit = true
			let batch = pb.createBatch()
			let inBatch = 0
			for (const payload of newPayloads) {
				batch.collection("network_probes").create(payload)
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

			resetBulkForm()
			toast({ title: t`Probes created`, description: `${newPayloads.length} probe(s) added.` })
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
				<Button variant="outline" onClick={openAdd} className="rounded-e-none grow">
					{/* <PlusIcon className="size-4 me-1" /> */}
					<Trans>Add {{ foo: t`Probe` }}</Trans>
				</Button>
				<div className="w-px h-full bg-muted"></div>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="outline" className="px-2 rounded-s-none border-s-0" aria-label={t`More probe actions`}>
							<ChevronDownIcon className="size-4" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem onClick={() => openBulkAdd(systemId)}>
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
				<ProbeDialogContent open={open} setOpen={setOpen} systemId={systemId} onOpenBulkAdd={openBulkAdd} />
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
							<Trans>Bulk Add {{ foo: t`Network Probes` }}</Trans>
						</SheetTitle>
						<SheetDescription>target[,protocol[,port[,interval[,name]]]]</SheetDescription>
					</SheetHeader>
					<form ref={bulkFormRef} onSubmit={handleBulkSubmit} className="flex h-full flex-col overflow-hidden">
						<div className="flex-1 flex flex-col space-y-4 overflow-auto p-4">
							{!systemId && (
								<div className="grid gap-2">
									<Label className="sr-only">
										<Trans>System</Trans>
									</Label>
									<Select value={bulkSelectedSystemId} onValueChange={setBulkSelectedSystemId} required>
										<SelectTrigger className="relative ps-10 pe-5 bg-card">
											<ServerIcon className="size-3.5 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
											<SelectValue placeholder={t`Select a system`} />
										</SelectTrigger>
										<SelectContent>
											{systems.map((sys) => (
												<SelectItem key={sys.id} value={sys.id}>
													{sys.name}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</div>
							)}
							<div className="grow flex flex-col gap-2">
								<Label htmlFor="bulk-probes" className="sr-only">
									Entries
								</Label>
								<Textarea
									id="bulk-probes"
									value={bulkInput}
									onChange={(e) => setBulkInput(e.target.value)}
									onKeyDown={(e) => {
										if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
											e.preventDefault()
											bulkFormRef.current?.requestSubmit()
										}
									}}
									className="font-mono grow text-sm bg-card"
									placeholder={["1.1.1.1", "example.com,tcp", "https://example.com,http,,60,Example"].join("\n")}
									required
								/>
								<p className="text-xs text-muted-foreground">target[,protocol[,port[,interval[,name]]]]</p>
							</div>
						</div>
						<SheetFooter className="border-t">
							<Button type="submit" disabled={bulkLoading || (!systemId && !bulkSelectedSystemId)}>
								<Trans>Add {{ foo: t`Network Probes` }}</Trans>
							</Button>
						</SheetFooter>
					</form>
				</SheetContent>
			</Sheet>
		</>
	)
}

export function EditProbeDialog({
	open,
	setOpen,
	systemId,
	probe,
}: {
	open: boolean
	setOpen: (open: boolean) => void
	systemId?: string
	probe?: NetworkProbeRecord
}) {
	const hasOpened = useRef(false)
	if (!probe && !hasOpened.current) {
		return null
	}
	hasOpened.current = true
	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<ProbeDialogContent open={open} setOpen={setOpen} systemId={systemId} probe={probe} />
		</Dialog>
	)
}

function ProbeDialogContent({
	open,
	setOpen,
	systemId,
	probe,
	onOpenBulkAdd,
}: {
	open: boolean
	setOpen: (open: boolean) => void
	systemId?: string
	probe?: NetworkProbeRecord
	onOpenBulkAdd?: (selectedSystemId?: string) => void
}) {
	const [protocol, setProtocol] = useState<ProbeProtocol>(probe?.protocol ?? "icmp")
	const [target, setTarget] = useState(probe?.target ?? "")
	const [port, setPort] = useState(
		(probe?.protocol === "tcp" || probe?.protocol === "http") && probe.port ? String(probe.port) : ""
	)
	const [probeInterval, setProbeInterval] = useState(String(probe?.interval ?? defaultInterval))
	const [name, setName] = useState(probe?.name ?? "")
	const [loading, setLoading] = useState(false)
	const [selectedSystemId, setSelectedSystemId] = useState(probe?.system ?? "")
	const systems = useStore($systems)
	const { toast } = useToast()
	const { t } = useLingui()
	const isEditing = !!probe
	const targetName = target.replace(/^https?:\/\//, "")

	// When the dialog is opened, initialize form fields with probe values (if editing) or defaults (if adding).
	useEffect(() => {
		if (!open) {
			return
		}

		setProtocol(probe?.protocol ?? "icmp")
		setTarget(probe?.target ?? "")
		setPort((probe?.protocol === "tcp" || probe?.protocol === "http") && probe.port ? String(probe.port) : "")
		setProbeInterval(String(probe?.interval ?? defaultInterval))
		setName(probe?.name ?? "")
		setSelectedSystemId(probe?.system ?? "")
		setLoading(false)
	}, [open, probe])

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault()
		setLoading(true)

		try {
			const selectedSystem = systemId ?? selectedSystemId
			if (!selectedSystem) {
				throw new Error("Select a system.")
			}
			const payload = buildProbePayload(
				{
					system: selectedSystem,
					target,
					protocol,
					port: protocol === "tcp" || protocol === "http" ? Number(port) : 0,
					interval: probeInterval,
					name,
				},
				probe ? probe.enabled : true
			)
			if (probe) {
				await pb.collection("network_probes").update(probe.id, payload)
			} else {
				await pb.collection("network_probes").create(payload)
			}
			setOpen(false)
		} catch (err: unknown) {
			toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
		} finally {
			setLoading(false)
		}
	}

	return (
		<DialogContent className="max-w-md">
			<DialogHeader>
				<DialogTitle>
					{isEditing ? <Trans>Edit {{ foo: t`Network Probe` }}</Trans> : <Trans>Add {{ foo: t`Network Probe` }}</Trans>}
				</DialogTitle>
				<DialogDescription>
					<Trans>Configure response monitoring from this agent.</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="grid gap-4 tabular-nums">
				{!systemId && (
					<div className="grid gap-2">
						<Label>
							<Trans>System</Trans>
						</Label>
						<Select value={selectedSystemId} onValueChange={setSelectedSystemId} required>
							<SelectTrigger>
								<SelectValue placeholder={t`Select a system`} />
							</SelectTrigger>
							<SelectContent>
								{systems.map((sys) => (
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
						placeholder={protocol === "http" ? "https://example.com" : "1.1.1.1"}
						required
					/>
				</div>
				<div className="grid gap-2">
					<Label>
						<Trans>Protocol</Trans>
					</Label>

					<Select value={protocol} onValueChange={(value) => setProtocol(value as ProbeProtocol)}>
						<SelectTrigger>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="icmp">ICMP</SelectItem>
							<SelectItem value="tcp">TCP</SelectItem>
							<SelectItem value="http">HTTP</SelectItem>
						</SelectContent>
					</Select>
				</div>
				{(protocol === "tcp" || protocol === "http") && (
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
				<div className="grid gap-2">
					<Label>
						<Trans>Interval (seconds)</Trans>
					</Label>
					<Input
						type="number"
						value={probeInterval}
						onChange={(e) => setProbeInterval(e.target.value)}
						min={1}
						max={3600}
						required
					/>
				</div>
				<div className="grid gap-2">
					<Label>
						<Trans>Name (optional)</Trans>
					</Label>
					<Input
						value={name}
						onChange={(e) => setName(e.target.value)}
						placeholder={targetName || t`e.g. Cloudflare DNS`}
					/>
				</div>
				<DialogFooter>
					{!isEditing && onOpenBulkAdd && (
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenBulkAdd(selectedSystemId)}
							disabled={loading}
							className="me-auto"
						>
							<ListIcon className="size-4 me-2" />
							<Trans>Bulk Add</Trans>
						</Button>
					)}
					<Button type="submit" disabled={loading || (!systemId && !selectedSystemId)}>
						{loading ? (
							isEditing ? (
								<Trans>Saving...</Trans>
							) : (
								<Trans>Creating...</Trans>
							)
						) : isEditing ? (
							<Trans>Save {{ foo: t`Probe` }}</Trans>
						) : (
							<Trans>Add {{ foo: t`Probe` }}</Trans>
						)}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
