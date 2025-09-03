import { lazy, Suspense, useEffect } from "react"
import { useStore } from "@nanostores/react"
import { $userSettings } from "@/lib/stores.ts"

const GeneralSettingsComponent = lazy(() => import("./settings/general.tsx"))

export default function GeneralPage() {
  const userSettings = useStore($userSettings)

  useEffect(() => {
    document.title = "General Settings / Beszel"
  }, [])

  return (
    <Suspense>
      <GeneralSettingsComponent userSettings={userSettings} />
    </Suspense>
  )
}