import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { BellIcon } from "lucide-react"
import { memo, useState } from "react"
import { AlertDialogContent, isNetworkMonitorAlert } from "@/components/alerts/alerts-sheet"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { $alerts, $allSystemsById } from "@/lib/stores"
import { cn } from "@/lib/utils"

/** Bell button that opens the network monitor alerts of a monitor's system. Filled when any are enabled. */
export const MonitorAlertsButton = memo(function MonitorAlertsButton({ systemId }: { systemId: string }) {
	const [opened, setOpened] = useState(false)
	const system = useStore($allSystemsById)[systemId]
	const systemAlerts = useStore($alerts)[systemId]
	const hasMonitorAlert = !!systemAlerts && Array.from(systemAlerts.keys()).some(isNetworkMonitorAlert)
	if (!system) {
		return null
	}
	return (
		// Keep clicks, including those inside the portaled sheet, from opening the row's monitor sheet.
		<div className="contents" onClick={(event) => event.stopPropagation()}>
			<Sheet>
				<SheetTrigger asChild>
					<Button variant="ghost" size="icon" aria-label={t`Alerts`} onClick={() => setOpened(true)}>
						<BellIcon className={cn("size-[1.2em] pointer-events-none", { "fill-primary": hasMonitorAlert })} />
					</Button>
				</SheetTrigger>
				<SheetContent className="max-h-full overflow-auto w-160 !max-w-full p-4 sm:p-6">
					{opened && <AlertDialogContent system={system} monitorAlertsOnly />}
				</SheetContent>
			</Sheet>
		</div>
	)
})
