import { useState } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
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

export function AddProbeDialog({
	systemId,
	onCreated,
}: {
	systemId: string
	onCreated: () => void
}) {
	const [open, setOpen] = useState(false)
	const [protocol, setProtocol] = useState<string>("icmp")
	const [target, setTarget] = useState("")
	const [port, setPort] = useState("")
	const [interval, setInterval] = useState("10")
	const [name, setName] = useState("")
	const [loading, setLoading] = useState(false)
	const { toast } = useToast()
	const { t } = useLingui()

	const resetForm = () => {
		setProtocol("icmp")
		setTarget("")
		setPort("")
		setInterval("10")
		setName("")
	}

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault()
		setLoading(true)
		try {
			await pb.send("/api/beszel/network-probes", {
				method: "POST",
				body: {
					system: systemId,
					name,
					target,
					protocol,
					port: protocol === "tcp" ? Number(port) : 0,
					interval: Number(interval),
				},
			})
			resetForm()
			setOpen(false)
			onCreated()
		} catch (err: any) {
			toast({ variant: "destructive", title: t`Error`, description: err?.message })
		} finally {
			setLoading(false)
		}
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button variant="outline" size="sm">
					<PlusIcon className="size-4 me-1" />
					<Trans>Add Probe</Trans>
				</Button>
			</DialogTrigger>
			<DialogContent className="max-w-md">
				<DialogHeader>
					<DialogTitle>
						<Trans>Add Network Probe</Trans>
					</DialogTitle>
					<DialogDescription>
						<Trans>Configure ICMP, TCP, or HTTP latency monitoring from this agent.</Trans>
					</DialogDescription>
				</DialogHeader>
				<form onSubmit={handleSubmit} className="grid gap-4">
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
							value={interval}
							onChange={(e) => setInterval(e.target.value)}
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
							placeholder={t`e.g. Cloudflare DNS`}
						/>
					</div>
					<DialogFooter>
						<Button type="submit" disabled={loading}>
							{loading ? <Trans>Creating...</Trans> : <Trans>Create</Trans>}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
