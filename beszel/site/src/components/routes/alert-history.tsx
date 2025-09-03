import { lazy, Suspense, useEffect } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card.tsx"
import { Trans } from "@lingui/react/macro"

const AlertHistoryComponent = lazy(() => import("./settings/alerts-history-data-table.tsx"))

export default function AlertHistoryPage() {
  useEffect(() => {
    document.title = "Alert History / Beszel"
  }, [])

  return (
    <Card className="pt-5 px-4 pb-8 min-h-96 mb-14 sm:pt-6 sm:px-7">
      <CardContent className="p-0">
        <Suspense>
          <AlertHistoryComponent />
        </Suspense>
      </CardContent>
    </Card>
  )
}