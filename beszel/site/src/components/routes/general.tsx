import { lazy, Suspense, useEffect } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card.tsx"
import { useStore } from "@nanostores/react"
import { $userSettings } from "@/lib/stores.ts"
import { Trans } from "@lingui/react/macro"

const GeneralSettingsComponent = lazy(() => import("./settings/general.tsx"))

export default function GeneralPage() {
  const userSettings = useStore($userSettings)

  useEffect(() => {
    document.title = "General Settings / Beszel"
  }, [])

  return (
    <Card className="pt-5 px-4 pb-8 min-h-96 mb-14 sm:pt-6 sm:px-7">
      <CardContent className="p-0">
        <Suspense>
          <GeneralSettingsComponent userSettings={userSettings} />
        </Suspense>
      </CardContent>
    </Card>
  )
}