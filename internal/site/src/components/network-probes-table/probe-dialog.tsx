import { useState } from "react"
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
import { ChevronDownIcon, ListIcon } from "lucide-react"
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

const Schema = v.object({
	system: v.string(),
	target: v.string(),
	protocol: v.picklist(["icmp", "tcp", "http"]),
	port: v.number(),
	interval: v.pipe(v.string(), v.toNumber(), v.minValue(1), v.maxValue(3600)),
	enabled: v.boolean(),
	name: v.optional(v.string()),
})

function buildProbePayload(values: ProbeValues) {
	const normalizedPort = (values.protocol === "tcp" || values.protocol === "http") && !values.port ? 443 : values.port
	const payload = v.parse(Schema, {
		system: values.system,
		target: values.target,
		protocol: values.protocol,
		port: normalizedPort,
		interval: values.interval,
		enabled: true,
	})
	const trimmedName = values.name?.trim()
	const targetName = values.target.replace(/^https?:\/\//i, "")
	if (trimmedName) {
		payload.name = trimmedName
	} else if (targetName !== values.target) {
		payload.name = targetName
	} else {
		payload.name = ""
	}
	return payload
}

function parseBulkProbeLine(line: string, lineNumber: number, system: string) {
	const [rawTarget = "", rawProtocol = "", rawPort = "", rawInterval = "", ...rawName] = line.split(",")
	const target = rawTarget.trim()
	if (!target) {
		throw new Error(`Line ${lineNumber}: target is required`)
	}

	const inferredProtocol: ProbeProtocol = /^https?:\/\//i.test(target) ? "http" : "icmp"
	const protocolValue = rawProtocol.trim().toLowerCase() || inferredProtocol
	if (protocolValue !== "icmp" && protocolValue !== "tcp" && protocolValue !== "http") {
		throw new Error(`Line ${lineNumber}: protocol must be icmp, tcp, or http`)
	}

	const portValue = rawPort.trim()
	if (protocolValue === "tcp") {
		const port = portValue ? Number(portValue) : 443
		if (!Number.isInteger(port) || port < 1 || port > 65535) {
			throw new Error(`Line ${lineNumber}: TCP entries require a port between 1 and 65535`)
		}
		return buildProbePayload({
			system,
			target,
			protocol: "tcp",
			port,
			interval: rawInterval.trim() || "30",
			name: rawName.join(",").trim() || undefined,
		})
	}

	return buildProbePayload({
		system,
		target,
		protocol: protocolValue,
		port: 0,
		interval: rawInterval.trim() || "30",
		name: rawName.join(",").trim() || undefined,
	})
}

export function AddProbeDialog({ systemId }: { systemId?: string }) {
	const [open, setOpen] = useState(false)
	const [bulkOpen, setBulkOpen] = useState(false)
	const [bulkInput, setBulkInput] = useState("")
	const [bulkLoading, setBulkLoading] = useState(false)
	const [bulkSelectedSystemId, setBulkSelectedSystemId] = useState("")
	const { toast } = useToast()
	const { t } = useLingui()
	const systems = useStore($systems)

	const resetBulkForm = () => {
		setBulkInput("")
		setBulkSelectedSystemId("")
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
			const rawLines = bulkInput.split(/\r?\n/).filter((line) => line.trim())
			if (!rawLines.length) {
				throw new Error("Enter at least one probe.")
			}

			const payloads = rawLines.map((line, index) => parseBulkProbeLine(line, index + 1, system))
			setBulkOpen(false)
			closedForSubmit = true
			let batch = pb.createBatch()
			let inBatch = 0
			for (const payload of payloads) {
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
			toast({ title: t`Probes created`, description: `${payloads.length} probe(s) added.` })
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
				{open && <ProbeDialogContent setOpen={setOpen} systemId={systemId} onOpenBulkAdd={openBulkAdd} />}
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
						<SheetDescription>
							<Trans>
								Paste one probe per line. See{" "}
								<a href={"#bulk-add-probes-docs"} className="underline underline-offset-2">
									the documentation
								</a>
								.
							</Trans>
						</SheetDescription>
					</SheetHeader>
					<form onSubmit={handleBulkSubmit} className="flex h-full flex-col overflow-hidden">
						<div className="flex-1 space-y-4 overflow-auto p-4">
							{!systemId && (
								<div className="grid gap-2">
									<Label>
										<Trans>System</Trans>
									</Label>
									<Select value={bulkSelectedSystemId} onValueChange={setBulkSelectedSystemId} required>
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
								<Label htmlFor="bulk-probes">
									<Trans>Entries</Trans>
								</Label>
								<Textarea
									id="bulk-probes"
									value={bulkInput}
									onChange={(e) => setBulkInput(e.target.value)}
									onKeyDown={(e) => {
										if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
											e.preventDefault()
											handleBulkSubmit(e)
										}
									}}
									className="h-120 font-mono text-sm bg-muted/40"
									style={{ maxHeight: `calc(100vh - 20rem)` }}
									placeholder={["1.1.1.1", "example.com,tcp", "https://example.com,http,,60,Homepage"].join("\n")}
									required
								/>
								<p className="text-xs text-muted-foreground">
									target[,protocol[,port[,interval[,name]]]] • TCP and HTTP default to port 443.
								</p>
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
	if (!probe) {
		return null
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			{open && <ProbeDialogContent setOpen={setOpen} systemId={systemId} probe={probe} />}
		</Dialog>
	)
}

function ProbeDialogContent({
	setOpen,
	systemId,
	probe,
	onOpenBulkAdd,
}: {
	setOpen: (open: boolean) => void
	systemId?: string
	probe?: NetworkProbeRecord
	onOpenBulkAdd?: (selectedSystemId?: string) => void
}) {
	const [protocol, setProtocol] = useState<ProbeProtocol>(probe?.protocol ?? "icmp")
	const [target, setTarget] = useState(probe?.target ?? "")
	const [port, setPort] = useState(probe?.protocol === "tcp" && probe.port ? String(probe.port) : "")
	const [probeInterval, setProbeInterval] = useState(String(probe?.interval ?? 30))
	const [name, setName] = useState(probe?.name ?? "")
	const [loading, setLoading] = useState(false)
	const [selectedSystemId, setSelectedSystemId] = useState(probe?.system ?? "")
	const systems = useStore($systems)
	const { toast } = useToast()
	const { t } = useLingui()
	const isEditing = !!probe
	const targetName = target.replace(/^https?:\/\//, "")

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault()
		setLoading(true)

		try {
			const payload = buildProbePayload({
				system: systemId ?? selectedSystemId,
				target,
				protocol,
				port: protocol === "tcp" ? Number(port) : 0,
				interval: probeInterval,
				name,
			})
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
							required
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
