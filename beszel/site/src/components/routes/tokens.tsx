import { lazy, Suspense, useEffect } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card.tsx"
import { Trans } from "@lingui/react/macro"

const TokensSettingsComponent = lazy(() => import("./settings/tokens-fingerprints.tsx"))

export default function TokensPage() {
  useEffect(() => {
    document.title = "Tokens & Fingerprints / Beszel"
  }, [])

  return (
    <Card className="pt-5 px-4 pb-8 min-h-96 mb-14 sm:pt-6 sm:px-7">
      <CardContent className="p-0">
        <Suspense>
          <TokensSettingsComponent />
        </Suspense>
      </CardContent>
    </Card>
  )
}