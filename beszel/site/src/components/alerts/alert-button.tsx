import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { memo, useMemo, useState } from "react"
import { useStore } from "@nanostores/react"
import { $alerts, $systems, pb } from "@/lib/stores"
import {
	// Dialog,
	// DialogTrigger,
	// DialogContent,
	// DialogDescription,
	// DialogHeader,
	// DialogTitle,
	Sheet,
	SheetTrigger,
	SheetContent,
	SheetHeader,
	SheetTitle,
	SheetDescription,
} from "@/components/ui/sheet"
import { BellIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { SystemRecord } from "@/types"
import { $router, Link } from "../router"
import { getPagePath } from "@nanostores/router"
import MultiSystemAlertSheetContent from './alerts-multi-sheet'

export default memo(function AlertsButton({ system }: { system: SystemRecord }) {
  const alerts = useStore($alerts)
  const systems = useStore($systems)
  const [opened, setOpened] = useState(false)
  const hasAlert = alerts.some((alert) => alert.system === system.id)
  return useMemo(
    () => (
      <Sheet open={opened} onOpenChange={setOpened}>
        <SheetTrigger asChild>
          <Button variant="ghost" size="icon" aria-label={t`Alerts`} data-nolink onClick={() => setOpened(true)}>
            <BellIcon
              className={cn("h-[1.2em] w-[1.2em] pointer-events-none", {
                "fill-primary": hasAlert,
              })}
            />
          </Button>
        </SheetTrigger>
        <SheetContent side="right" className="max-h-full overflow-auto w-[55em] p-4 sm:p-5">
          <SheetHeader>
            <SheetTitle className="text-xl">
              <Trans>Alerts</Trans>
            </SheetTitle>
            <SheetDescription>
              <Trans>
                See <Link href={getPagePath($router, "settings", { name: "notifications" })} className="link">notification settings</Link> to configure how you receive alerts.
              </Trans>
            </SheetDescription>
          </SheetHeader>
          <MultiSystemAlertSheetContent
            systems={systems}
            alerts={alerts}
            initialSystems={[system.id]}
            onClose={() => setOpened(false)}
            hideSystemSelector
          />
        </SheetContent>
      </Sheet>
    ),
    [opened, hasAlert, systems, alerts, system.id]
  )
})
