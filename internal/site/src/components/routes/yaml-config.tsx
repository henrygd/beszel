import { lazy, Suspense, useEffect } from "react"

const YamlConfigComponent = lazy(() => import("./settings/config-yaml.tsx"))

export default function YamlConfigPage() {
  useEffect(() => {
    document.title = "YAML Config / Beszel"
  }, [])

  return (
    <Suspense>
      <YamlConfigComponent />
    </Suspense>
  )
}