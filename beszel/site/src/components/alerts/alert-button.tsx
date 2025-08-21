import { t } from "@lingui/core/macro"
import { memo, useMemo, useState } from "react"
import { useStore } from "@nanostores/react"
import { $alerts } from "@/lib/stores"
import { Dialog, DialogTrigger, DialogContent } from "@/components/ui/dialog"
import { BellIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { SystemRecord } from "@/types"
import { AlertDialogContent } from "./alerts-dialog"

export default memo(function AlertsButton({ system }: { system: SystemRecord }) {
	const [opened, setOpened] = useState(false)
	const alerts = useStore($alerts)

	const hasSystemAlert = alerts[system.id]?.size > 0

	return useMemo(
		() => (
			<Dialog>
				<DialogTrigger asChild>
					<Button variant="ghost" size="icon" aria-label={t`Alerts`} onClick={() => setOpened(true)}>
						<BellIcon
							className={cn("h-[1.2em] w-[1.2em]", {
								"fill-primary": hasSystemAlert,
							})}
						/>
					</Button>
				</DialogTrigger>
				<DialogContent className="max-h-full sm:max-h-[95svh] overflow-auto max-w-[37rem]">
					{opened && <AlertDialogContent system={system} />}
				</DialogContent>
			</Dialog>
		),
		[opened, hasSystemAlert]
	)

	// return useMemo(
	// 	() => (
	// 		<Sheet>
	// 			<SheetTrigger asChild>
	// 				<Button variant="ghost" size="icon" aria-label={t`Alerts`} data-nolink onClick={() => setOpened(true)}>
	// 					<BellIcon
	// 						className={cn("h-[1.2em] w-[1.2em] pointer-events-none", {
	// 							"fill-primary": hasAlert,
	// 						})}
	// 					/>
	// 				</Button>
	// 			</SheetTrigger>
	// 			<SheetContent className="max-h-full overflow-auto w-[35em] p-4 sm:p-5">
	// 				{opened && <AlertDialogContent system={system} />}
	// 			</SheetContent>
	// 		</Sheet>
	// 	),
	// 	[opened, hasAlert]
	// )
})
