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
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { PlusIcon } from "lucide-react"
import { useToast } from "@/components/ui/use-toast"
import { SystemMultiSelect } from "@/components/network-monitors-table/monitor-dialog"
import { AUTOMATIC_SERVER, type SpeedtestServer, SpeedtestServerSelect } from "./speedtest-server-select"
import { $systems } from "@/lib/stores"
import { supportsSpeedtests } from "@/lib/utils"
import { DEFAULT_SPEEDTEST_INTERVAL, MAX_SPEEDTEST_INTERVAL, MIN_SPEEDTEST_INTERVAL } from "@/lib/speedtest-utils"
import type { SpeedtestRecord } from "@/types"

export function AddSpeedtestDialog({ systemId }: { systemId?: string }) {
	const [open, setOpen] = useState(false)
	const { t } = useLingui()
	const systems = useStore($systems)
	const hasEligibleSystems = systemId ? true : systems.some(supportsSpeedtests)

	return (
		<>
			<Button variant="outline" onClick={() => setOpen(true)} disabled={!hasEligibleSystems}>
				<PlusIcon className="size-4 me-1" />
				<span className="sm:hidden">
					<Trans>Add</Trans>
				</span>
				<span className="hidden sm:inline">
					<Trans>Add {{ foo: t`Speedtest` }}</Trans>
				</span>
			</Button>
			<Dialog open={open} onOpenChange={setOpen}>
				<SpeedtestDialogContent open={open} setOpen={setOpen} systemId={systemId} />
			</Dialog>
		</>
	)
}

export function EditSpeedtestDialog({
	open,
	setOpen,
	systemId,
	speedtest,
}: {
	open: boolean
	setOpen: (open: boolean) => void
	systemId?: string
	speedtest?: SpeedtestRecord
}) {
	const hasOpened = useRef(false)
	if (!speedtest && !hasOpened.current) {
		return null
	}
	hasOpened.current = true
	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<SpeedtestDialogContent open={open} setOpen={setOpen} systemId={systemId} speedtest={speedtest} />
		</Dialog>
	)
}

function SpeedtestDialogContent({
	open,
	setOpen,
	systemId,
	speedtest,
}: {
	open: boolean
	setOpen: (open: boolean) => void
	systemId?: string
	speedtest?: SpeedtestRecord
}) {
	const [server, setServer] = useState<SpeedtestServer>(AUTOMATIC_SERVER)
	const [interval, setInterval] = useState(String(DEFAULT_SPEEDTEST_INTERVAL))
	const [loading, setLoading] = useState(false)
	const [selectedSystemId, setSelectedSystemId] = useState("")
	const [selectedSystemIds, setSelectedSystemIds] = useState<Set<string>>(new Set())
	const systems = useStore($systems)
	const { toast } = useToast()
	const { t } = useLingui()
	const isEditing = !!speedtest

	// Initialize form fields with speedtest values (if editing) or defaults (if adding).
	useEffect(() => {
		if (!open) {
			return
		}
		setServer(
			speedtest?.server_id
				? { id: speedtest.server_id, name: speedtest.server_name, location: speedtest.server_location }
				: AUTOMATIC_SERVER
		)
		setInterval(String(speedtest?.interval ?? DEFAULT_SPEEDTEST_INTERVAL))
		setSelectedSystemId(speedtest?.system ?? "")
		setSelectedSystemIds(new Set())
		setLoading(false)
	}, [open, speedtest])

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault()
		setLoading(true)

		const targetSystems = systemId ? [systemId] : speedtest ? [selectedSystemId] : Array.from(selectedSystemIds)
		const remainingSystemIds = new Set(targetSystems)
		try {
			if (!targetSystems.length || !targetSystems[0]) throw new Error("Select at least one system.")
			const payload = {
				server_id: server.id,
				server_name: server.name,
				server_location: server.location,
				interval: Number(interval),
			}
			if (speedtest) {
				await pb.collection("speedtests").update(speedtest.id, { ...payload, system: targetSystems[0] })
			} else {
				for (const system of targetSystems) {
					await pb.collection("speedtests").create({ ...payload, system, enabled: true })
					remainingSystemIds.delete(system)
				}
			}
			setOpen(false)
		} catch (err: unknown) {
			if (!speedtest && !systemId) {
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
					{isEditing ? <Trans>Edit {{ foo: t`Speedtest` }}</Trans> : <Trans>Add {{ foo: t`Speedtest` }}</Trans>}
				</DialogTitle>
				<DialogDescription>
					<Trans>Run scheduled speedtests from this agent with the Ookla speedtest CLI.</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="grid gap-4 tabular-nums">
				{!systemId && !isEditing && (
					<div className="grid gap-2">
						<Label htmlFor="speedtest-systems">
							<Trans>Systems</Trans>
						</Label>
						<SystemMultiSelect
							id="speedtest-systems"
							selectedSystemIds={selectedSystemIds}
							onChange={setSelectedSystemIds}
							disabled={loading}
							isEligible={supportsSpeedtests}
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
									.filter((sys) => sys.id === speedtest?.system || supportsSpeedtests(sys))
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
					<Label htmlFor="speedtest-server">
						<Trans>Server</Trans>
					</Label>
					<SpeedtestServerSelect id="speedtest-server" value={server} onChange={setServer} disabled={loading} />
				</div>
				<div className="grid gap-2">
					<Label htmlFor="speedtest-interval">
						<Trans>Interval (minutes)</Trans>
					</Label>
					<Input
						id="speedtest-interval"
						type="number"
						value={interval}
						onChange={(e) => setInterval(e.target.value)}
						min={MIN_SPEEDTEST_INTERVAL}
						max={MAX_SPEEDTEST_INTERVAL}
						step={1}
						required
					/>
					<p className="text-xs text-muted-foreground">
						<Trans>
							Minimum {MIN_SPEEDTEST_INTERVAL} minutes. Each system tests at its own fixed time within the interval,
							so systems on the same network don't overlap. Each run uses your full bandwidth for about 30 seconds.
						</Trans>
					</p>
				</div>
				<DialogFooter>
					<Button
						type="submit"
						disabled={loading || (!systemId && (isEditing ? !selectedSystemId : !selectedSystemIds.size))}
					>
						{isEditing ? <Trans>Save {{ foo: t`Speedtest` }}</Trans> : <Trans>Add {{ foo: t`Speedtest` }}</Trans>}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
