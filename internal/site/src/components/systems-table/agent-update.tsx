import { plural, t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { DownloadIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import { $systems } from "@/lib/stores"
import type { SystemRecord } from "@/types"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "../ui/alert-dialog"
import { Button } from "../ui/button"

/** Whether the current user can trigger an update for this system's agent. */
export function canUpdateAgent(system: SystemRecord) {
	return isAdmin() && !!system.info?.ru && system.status === SystemStatus.Up
}

/** Agents that can be updated and don't match the hub version. */
function isOutdatedUpdatableAgent(system: SystemRecord) {
	return canUpdateAgent(system) && !!system.info?.v && system.info.v !== globalThis.BESZEL.HUB_VERSION
}

async function requestAgentUpdates(systems: SystemRecord[]) {
	const results = await Promise.allSettled(
		systems.map((system) =>
			pb.send("/api/beszel/agent-update", {
				method: "POST",
				query: { system: system.id },
			})
		)
	)
	const failed = systems.filter((_, i) => results[i].status === "rejected")
	const succeeded = systems.length - failed.length
	if (succeeded > 0) {
		toast({
			title: t`Update requested`,
			description: plural(succeeded, {
				one: "# agent will update and reconnect shortly.",
				other: "# agents will update and reconnect shortly.",
			}),
		})
	}
	if (failed.length > 0) {
		toast({
			title: t`Error`,
			description: t`Failed to request update for ${failed.map((s) => s.name).join(", ")}`,
			variant: "destructive",
		})
	}
}

/** Confirmation dialog for updating one or more agents. */
export function AgentUpdateDialog({
	systems,
	open,
	setOpen,
}: {
	systems: SystemRecord[]
	open: boolean
	setOpen: (open: boolean) => void
}) {
	const count = systems.length
	const names = systems.map((s) => s.name).join(", ")
	return (
		<AlertDialog open={open} onOpenChange={setOpen}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>{plural(count, { one: "Update # agent?", other: "Update # agents?" })}</AlertDialogTitle>
					<AlertDialogDescription>
						<Trans>
							The agent will download the latest release from GitHub and restart. Systems may briefly appear down.
						</Trans>
					</AlertDialogDescription>
				</AlertDialogHeader>
				<p className="text-sm text-muted-foreground break-words">{names}</p>
				<AlertDialogFooter>
					<AlertDialogCancel>
						<Trans>Cancel</Trans>
					</AlertDialogCancel>
					<AlertDialogAction onClick={() => requestAgentUpdates(systems)}>
						<Trans>Update</Trans>
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	)
}

/** Toolbar button to update all outdated agents that have remote update enabled. */
export function UpdateOutdatedAgentsButton() {
	const systems = useStore($systems)
	const [open, setOpen] = useState(false)
	const outdated = useMemo(() => systems.filter(isOutdatedUpdatableAgent), [systems])

	if (outdated.length === 0) {
		return null
	}
	return (
		<>
			<Button variant="outline" onClick={() => setOpen(true)} title={t`Update outdated agents`}>
				<DownloadIcon className="me-1.5 size-4 opacity-80" />
				<Trans>Update agents ({outdated.length})</Trans>
			</Button>
			<AgentUpdateDialog systems={outdated} open={open} setOpen={setOpen} />
		</>
	)
}
