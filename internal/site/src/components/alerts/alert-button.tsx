import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { BellIcon } from "lucide-react"
import { memo, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { $alerts } from "@/lib/stores"
import { cn } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { AlertDialogContent, alertsSheetClassName } from "./alerts-sheet"

export default memo(function AlertsButton({ system }: { system: SystemRecord }) {
	const [opened, setOpened] = useState(false)
	const alerts = useStore($alerts)

	const hasSystemAlert = alerts[system.id]?.size > 0
	return useMemo(
		() => (
			<Sheet>
				<SheetTrigger asChild>
					<Button variant="ghost" size="icon" aria-label={t`Alerts`} data-nolink onClick={() => setOpened(true)}>
						<BellIcon
							className={cn("size-[1.2em] pointer-events-none", {
								"fill-primary": hasSystemAlert,
							})}
						/>
					</Button>
				</SheetTrigger>
				<SheetContent className={alertsSheetClassName}>{opened && <AlertDialogContent system={system} />}</SheetContent>
			</Sheet>
		),
		[opened, hasSystemAlert]
	)
})
