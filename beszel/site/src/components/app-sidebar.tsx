import * as React from "react"
import { Trans } from "@lingui/react/macro"
import { useState, lazy, Suspense } from "react"
import {
  AlertOctagonIcon,
  BellIcon,
  DatabaseBackupIcon,
  FileSlidersIcon,
  FingerprintIcon,
  LogOutIcon,
  LogsIcon,
  MoonStarIcon,
  SearchIcon,
  ServerIcon,
  SettingsIcon,
  SunIcon,
  UserIcon,
  UsersIcon,
} from "lucide-react"
import { $router, basePath, Link, prependBasePath } from "./router"
import { useTheme } from "./theme-provider"
import { Logo } from "./logo"
import { runOnce } from "@/lib/utils"
import { isReadOnlyUser, isAdmin, logOut, pb } from "@/lib/api"
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu"
import { AddSystemButton } from "./add-system"
import { getPagePath } from "@nanostores/router"
import { Button } from "@/components/ui/button"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar"

const CommandPalette = lazy(() => import("./command-palette"))

const isMac = navigator.platform.toUpperCase().indexOf("MAC") >= 0

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const [open, setOpen] = useState(false)
  const { theme, setTheme } = useTheme()

  const Kbd = ({ children }: { children: React.ReactNode }) => (
    <kbd className="pointer-events-none inline-flex h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground opacity-100">
      {children}
    </kbd>
  )

  return (
    <Sidebar {...props}>
      <SidebarHeader className="border-b px-4 py-3">
        <Link
          href={basePath}
          aria-label="Home"
          className="flex items-center justify-center"
          onMouseEnter={runOnce(() => import("@/components/routes/home"))}
        >
          <Logo className="h-6 w-40 fill-foreground" />
        </Link>
      </SidebarHeader>
      
      <SidebarContent className="px-2">
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <Button
                  variant="outline"
                  className="w-full justify-start text-sm text-muted-foreground"
                  onClick={() => setOpen(true)}
                >
                  <SearchIcon className="mr-2 h-4 w-4" />
                  <Trans>Search</Trans>
                  <span className="ml-auto flex items-center gap-1">
                    <Kbd>{isMac ? "⌘" : "Ctrl"}</Kbd>
                    <Kbd>K</Kbd>
                  </span>
                </Button>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton asChild>
                  <Link href={basePath} className="flex items-center gap-2">
                    <ServerIcon className="h-4 w-4" />
                    <span>
                      <Trans>Systems</Trans>
                    </span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupLabel>
            <Trans>Settings</Trans>
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton asChild>
                  <Link
                    href={getPagePath($router, "settings", { name: "general" })}
                    className="flex items-center gap-2"
                  >
                    <SettingsIcon className="h-4 w-4" />
                    <span>
                      <Trans>General</Trans>
                    </span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton asChild>
                  <Link
                    href={getPagePath($router, "settings", { name: "notifications" })}
                    className="flex items-center gap-2"
                  >
                    <BellIcon className="h-4 w-4" />
                    <span>
                      <Trans>Notifications</Trans>
                    </span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
              {!isReadOnlyUser() && (
                <SidebarMenuItem>
                  <SidebarMenuButton asChild>
                    <Link
                      href={getPagePath($router, "settings", { name: "tokens" })}
                      className="flex items-center gap-2"
                    >
                      <FingerprintIcon className="h-4 w-4" />
                      <span>
                        <Trans>Tokens & Fingerprints</Trans>
                      </span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
              <SidebarMenuItem>
                <SidebarMenuButton asChild>
                  <Link
                    href={getPagePath($router, "settings", { name: "alert-history" })}
                    className="flex items-center gap-2"
                  >
                    <AlertOctagonIcon className="h-4 w-4" />
                    <span>
                      <Trans>Alert History</Trans>
                    </span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
              {isAdmin() && (
                <SidebarMenuItem>
                  <SidebarMenuButton asChild>
                    <Link
                      href={getPagePath($router, "settings", { name: "config" })}
                      className="flex items-center gap-2"
                    >
                      <FileSlidersIcon className="h-4 w-4" />
                      <span>
                        <Trans>YAML Config</Trans>
                      </span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {isAdmin() && (
          <SidebarGroup>
            <SidebarGroupLabel>
              <Trans>Application</Trans>
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton asChild>
                    <a href={prependBasePath("/_/")} target="_blank" className="flex items-center gap-2">
                      <UsersIcon className="h-4 w-4" />
                      <span>
                        <Trans>Users</Trans>
                      </span>
                    </a>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton asChild>
                    <a href={prependBasePath("/_/#/collections?collection=systems")} target="_blank" className="flex items-center gap-2">
                      <ServerIcon className="h-4 w-4" />
                      <span>
                        <Trans>Manage Systems</Trans>
                      </span>
                    </a>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton asChild>
                    <a href={prependBasePath("/_/#/logs")} target="_blank" className="flex items-center gap-2">
                      <LogsIcon className="h-4 w-4" />
                      <span>
                        <Trans>Logs</Trans>
                      </span>
                    </a>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton asChild>
                    <a href={prependBasePath("/_/#/settings/backups")} target="_blank" className="flex items-center gap-2">
                      <DatabaseBackupIcon className="h-4 w-4" />
                      <span>
                        <Trans>Backups</Trans>
                      </span>
                    </a>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        )}
      </SidebarContent>

      <SidebarFooter className="border-t p-4">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="w-full justify-start gap-2 p-2">
              <UserIcon className="h-4 w-4" />
              <span className="truncate text-sm">{pb.authStore.record?.email}</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-44">
            <DropdownMenuLabel>{pb.authStore.record?.email}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setTheme(theme === "dark" ? "light" : "dark")}>
              {theme === "dark" ? (
                <SunIcon className="mr-2 h-4 w-4" />
              ) : (
                <MoonStarIcon className="mr-2 h-4 w-4" />
              )}
              <span>
                <Trans>{theme === "dark" ? "Light Mode" : "Dark Mode"}</Trans>
              </span>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={logOut}>
              <LogOutIcon className="mr-2 h-4 w-4" />
              <span>
                <Trans>Log Out</Trans>
              </span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <AddSystemButton className="w-full" />
      </SidebarFooter>
      
      <SidebarRail />
      
      <Suspense>
        <CommandPalette open={open} setOpen={setOpen} />
      </Suspense>
    </Sidebar>
  )
}
