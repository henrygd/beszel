import { lazy, Suspense, useEffect } from "react"

const AlertHistoryComponent = lazy(() => import("./settings/alerts-history-data-table.tsx"))

export default function AlertHistoryPage() {
  useEffect(() => {
    document.title = "Alert History / Beszel"
  }, [])

  return (
    <Suspense>
      <AlertHistoryComponent />
    </Suspense>
  )
}