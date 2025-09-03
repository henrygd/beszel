import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { $router, basePath, Link, getPagePath } from "./router"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { HomeIcon, ChevronDownIcon } from "lucide-react"
import { isAdmin, isReadOnlyUser } from "@/lib/api"

interface BreadcrumbSegment {
  label: string
  href?: string
  isLast?: boolean
  hasDropdown?: boolean
  dropdownItems?: Array<{ label: string; href: string }>
}

export function Breadcrumbs() {
  const page = useStore($router)
  
  if (!page) return null

  const segments: BreadcrumbSegment[] = []

  // Always start with Home
  segments.push({
    label: "Home",
    href: basePath || "/",
  })

  // Add route-specific breadcrumbs
  if (page.route === "system") {
    segments.push({
      label: page.params.name,
      isLast: true,
    })
  } else if (page.route === "settings") {
    const settingsName = page.params.name
    
    // Build settings dropdown items based on permissions
    const settingsDropdownItems = []
    
    settingsDropdownItems.push(
      { label: "General", href: getPagePath($router, "settings", { name: "general" }) },
      { label: "Notifications", href: getPagePath($router, "settings", { name: "notifications" }) }
    )
    
    if (!isReadOnlyUser()) {
      settingsDropdownItems.push(
        { label: "Tokens & Fingerprints", href: getPagePath($router, "settings", { name: "tokens" }) }
      )
    }
    
    settingsDropdownItems.push(
      { label: "Alert History", href: getPagePath($router, "settings", { name: "alert-history" }) }
    )
    
    if (isAdmin()) {
      settingsDropdownItems.push(
        { label: "YAML Config", href: getPagePath($router, "settings", { name: "config" }) }
      )
    }
    
    // Add Settings section with dropdown
    segments.push({
      label: "Settings",
      hasDropdown: true,
      dropdownItems: settingsDropdownItems,
    })
    
    // Add specific settings page if exists
    if (settingsName) {
      const settingsLabels: Record<string, string> = {
        general: "General",
        notifications: "Notifications", 
        tokens: "Tokens & Fingerprints",
        "alert-history": "Alert History",
        config: "YAML Config",
      }
      
      segments.push({
        label: settingsLabels[settingsName] || settingsName,
        isLast: true,
      })
    } else {
      // Mark Settings as last if no specific page
      segments[segments.length - 1].isLast = true
    }
  }

  return (
    <>
      {segments.map((segment, index) => (
        <div key={index} className="flex items-center">
          {index > 0 && <BreadcrumbSeparator />}
          <BreadcrumbItem>
            {segment.hasDropdown ? (
              <DropdownMenu>
                <DropdownMenuTrigger className="flex items-center gap-1 hover:text-foreground">
                  <Trans>{segment.label}</Trans>
                  <ChevronDownIcon className="h-4 w-4" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  {segment.dropdownItems?.map((item, itemIndex) => (
                    <DropdownMenuItem key={itemIndex} asChild>
                      <Link href={item.href}>
                        <Trans>{item.label}</Trans>
                      </Link>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : segment.isLast || !segment.href ? (
              <BreadcrumbPage>
                {index === 0 ? (
                  <div className="flex items-center gap-1.5">
                    <HomeIcon className="h-4 w-4" />
                    <Trans>{segment.label}</Trans>
                  </div>
                ) : (
                  <Trans>{segment.label}</Trans>
                )}
              </BreadcrumbPage>
            ) : (
              <BreadcrumbLink asChild>
                <Link href={segment.href}>
                  {index === 0 ? (
                    <div className="flex items-center gap-1.5">
                      <HomeIcon className="h-4 w-4" />
                      <Trans>{segment.label}</Trans>
                    </div>
                  ) : (
                    <Trans>{segment.label}</Trans>
                  )}
                </Link>
              </BreadcrumbLink>
            )}
          </BreadcrumbItem>
        </div>
      ))}
    </>
  )
}