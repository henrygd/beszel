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
	DialogTrigger,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { PlusIcon } from "lucide-react"
import { useToast } from "@/components/ui/use-toast"
import { $systems } from "@/lib/stores"

export function AddProbeDialog({ systemId }: { systemId?: string }) {
	const [open, setOpen] = useState(false)
	const [protocol, setProtocol] = useState<string>("icmp")
	const [target, setTarget] = useState("")
	const [port, setPort] = useState("")
	const [probeInterval, setProbeInterval] = useState("60")
	const [name, setName] = useState("")
	const [loading, setLoading] = useState(false)
	const [selectedSystemId, setSelectedSystemId] = useState("")
	const systems = useStore($systems)
	const { toast } = useToast()
	const { t } = useLingui()

	const resetForm = () => {
		setProtocol("icmp")
		setTarget("")
		setPort("")
		setProbeInterval("60")
		setName("")
		setSelectedSystemId("")
	}

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault()
		setLoading(true)
		try {
			await pb.collection("network_probes").create({
				system: systemId ?? selectedSystemId,
				name,
				target,
				protocol,
				port: protocol === "tcp" ? Number(port) : 0,
				interval: Number(probeInterval),
				enabled: true,
			})
			resetForm()
			setOpen(false)
		} catch (err: unknown) {
			toast({ variant: "destructive", title: t`Error`, description: (err as Error)?.message })
		} finally {
			setLoading(false)
		}
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button variant="outline">
					<PlusIcon className="size-4 me-1" />
					<Trans>Add {{ foo: t`Probe` }}</Trans>
				</Button>
			</DialogTrigger>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						<Trans>Add {{ foo: t`Network Probe` }}</Trans>
					</DialogTitle>
					<DialogDescription>
						<Trans>Configure ICMP, TCP, or HTTP latency monitoring from this agent.</Trans>
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
						<Select value={protocol} onValueChange={setProtocol}>
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
							placeholder={target || t`e.g. Cloudflare DNS`}
						/>
					</div>
					<DialogFooter>
					<Button type="submit" disabled={loading || (!systemId && !selectedSystemId)}>
							{loading ? <Trans>Creating...</Trans> : <Trans>Add {{ foo: t`Probe` }}</Trans>}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
