import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { BellIcon } from "lucide-react"
import { memo, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { $alerts } from "@/lib/stores"
import { cn } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { AlertDialogContent } from "./alerts-sheet"

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
							className={cn("h-[1.2em] w-[1.2em] pointer-events-none", {
								"fill-primary": hasSystemAlert,
							})}
						/>
					</Button>
				</SheetTrigger>
				<SheetContent className="max-h-full overflow-auto w-145 !max-w-full p-4 sm:p-6">
					{opened && <AlertDialogContent system={system} />}
				</SheetContent>
			</Sheet>
		),
		[opened, hasSystemAlert]
	)
})
