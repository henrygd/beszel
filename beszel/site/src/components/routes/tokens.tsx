import { lazy, Suspense, useEffect } from "react"

const TokensSettingsComponent = lazy(() => import("./settings/tokens-fingerprints.tsx"))

export default function TokensPage() {
  useEffect(() => {
    document.title = "Tokens & Fingerprints / Beszel"
  }, [])

  return (
    <Suspense>
      <TokensSettingsComponent />
    </Suspense>
  )
}