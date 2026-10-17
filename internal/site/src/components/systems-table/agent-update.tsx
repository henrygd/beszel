import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
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

/** Whether the current user can trigger an update for this system's agent.
 * Only offered when the agent version differs from the hub version. */
export function canUpdateAgent(system: SystemRecord) {
	const { info, status } = system
	return isAdmin() && !!info?.ru && status === SystemStatus.Up && !!info.v && info.v !== globalThis.BESZEL.HUB_VERSION
}

async function requestAgentUpdate(system: SystemRecord) {
	try {
		await pb.send("/api/beszel/agent-update", {
			method: "POST",
			query: { system: system.id },
		})
		toast({
			title: t`Update requested`,
			description: t`${system.name} will update and reconnect shortly.`,
		})
	} catch {
		toast({
			title: t`Error`,
			description: t`Failed to request update for ${system.name}`,
			variant: "destructive",
		})
	}
}

/** Confirmation dialog for updating a system's agent. */
export function AgentUpdateDialog({
	system,
	open,
	setOpen,
}: {
	system: SystemRecord
	open: boolean
	setOpen: (open: boolean) => void
}) {
	const { name } = system
	return (
		<AlertDialog open={open} onOpenChange={setOpen}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>
						<Trans>Update agent on {name}?</Trans>
					</AlertDialogTitle>
					<AlertDialogDescription>
						<Trans>The agent will download the latest release and restart. The system may briefly appear down.</Trans>
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>
						<Trans>Cancel</Trans>
					</AlertDialogCancel>
					<AlertDialogAction onClick={() => requestAgentUpdate(system)}>
						<Trans>Update</Trans>
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	)
}
