import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { BellIcon } from "lucide-react"
import { memo, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
import { $containerAlerts } from "@/lib/stores"
import { cn } from "@/lib/utils"
import type { ContainerRecord } from "@/types"
import { ContainerAlertDialogContent } from "./container-alerts-sheet"

export default memo(function ContainerAlertButton({
    systemId,
    container,
}: {
    systemId: string
    container: ContainerRecord
}) {
    const [opened, setOpened] = useState(false)
    const alerts = useStore($containerAlerts)

    const containerAlerts = alerts[systemId]?.get(container.id)
    const hasContainerAlert = containerAlerts && containerAlerts.size > 0

    return useMemo(
        () => (
            <Sheet>
                <SheetTrigger asChild>
                    <Button variant="ghost" size="icon" aria-label={t`Alerts`} data-nolink onClick={() => setOpened(true)}>
                        <BellIcon
                            className={cn("h-[1.2em] w-[1.2em] pointer-events-none", {
                                "fill-primary": hasContainerAlert,
                            })}
                        />
                    </Button>
                </SheetTrigger>
                <SheetContent className="max-h-full overflow-auto w-150 !max-w-full p-4 sm:p-6">
                    {opened && <ContainerAlertDialogContent systemId={systemId} container={container} />}
                </SheetContent>
            </Sheet>
        ),
        [opened, hasContainerAlert]
    )
})
