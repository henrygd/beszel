import { lazy, Suspense, useEffect } from "react"
import { useStore } from "@nanostores/react"
import { $userSettings } from "@/lib/stores.ts"

const NotificationsSettingsComponent = lazy(() => import("./settings/notifications.tsx"))

export default function NotificationsPage() {
  const userSettings = useStore($userSettings)

  useEffect(() => {
    document.title = "Notifications / Beszel"
  }, [])

  return (
    <Suspense>
      <NotificationsSettingsComponent userSettings={userSettings} />
    </Suspense>
  )
}