import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { $router, basePath, Link } from "./router"
import { getPagePath } from "@nanostores/router"
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
import { ChevronDownIcon } from "lucide-react"
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
  } else if (page.route === "application") {
    const applicationName = page.params.name
    
    // Add Application section  
    segments.push({
      label: "Application",
      href: getPagePath($router, "application", {}),
    })
    
    // Add specific application page if exists
    if (applicationName) {
      const applicationLabels: Record<string, string> = {
        config: "YAML Config",
      }
      
      segments.push({
        label: applicationLabels[applicationName] || applicationName,
        isLast: true,
      })
    } else {
      // Mark Application as last if no specific page
      segments[segments.length - 1].isLast = true
    }
  }

  return (
    <Breadcrumb>
      <BreadcrumbList>
        {segments.map((segment, index) => (
          <>
            {index > 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem key={index}>
              {segment.hasDropdown ? (
                <DropdownMenu>
                  <DropdownMenuTrigger>
                    <Trans>{segment.label}</Trans>
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
                  <Trans>{segment.label}</Trans>
                </BreadcrumbPage>
              ) : (
                <BreadcrumbLink asChild>
                  <Link href={segment.href}>
                    <Trans>{segment.label}</Trans>
                  </Link>
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
          </>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  )
}