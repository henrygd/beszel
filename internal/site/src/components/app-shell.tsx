import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { getPagePath } from "@nanostores/router"
import { useStore } from "@nanostores/react"
import {
	ContainerIcon,
	DatabaseBackupIcon,
	ExternalLinkIcon,
	HardDriveIcon,
	LogOutIcon,
	LogsIcon,
	MenuIcon,
	PanelLeftCloseIcon,
	PanelLeftOpenIcon,
	PlusIcon,
	SearchIcon,
	ServerIcon,
	SettingsIcon,
	UsersIcon,
} from "lucide-react"
import { lazy, Suspense, useState, type ElementType, type ReactNode } from "react"
import { $direction } from "@/lib/stores"
import { isAdmin, isReadOnlyUser, logOut, pb } from "@/lib/api"
import { cn, runOnce } from "@/lib/utils"
import { AddSystemDialog } from "./add-system"
import { Logo, LogoMark } from "./logo"
import { ModeToggle } from "./mode-toggle"
import { $router, basePath, Link, navigate, prependBasePath } from "./router"
import { Button } from "./ui/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "./ui/dropdown-menu"
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "./ui/sheet"
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip"

const CommandPalette = lazy(() => import("./command-palette"))

const preloadHome = runOnce(() => import("@/components/routes/home"))
const preloadSettings = runOnce(() => import("@/components/routes/settings/general"))

const SIDEBAR_COLLAPSED_KEY = "sidebar-collapsed"

function readCollapsedPreference(): boolean {
	try {
		return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1"
	} catch {
		return false
	}
}

/** Primary destinations, shared by the sidebar and the mobile drawer. */
function useNavItems() {
	const page = useStore($router)
	const route = page?.route
	return [
		{
			href: basePath,
			label: t`Fleet`,
			icon: ServerIcon,
			active: route === "home",
			preload: preloadHome,
			external: false,
		},
		{
			href: getPagePath($router, "containers"),
			label: t`All Containers`,
			icon: ContainerIcon,
			active: route === "containers",
			preload: () => import("@/components/routes/containers"),
			external: false,
		},
		{
			href: getPagePath($router, "smart"),
			label: "S.M.A.R.T.",
			icon: HardDriveIcon,
			active: route === "smart",
			preload: () => import("@/components/routes/smart"),
			external: false,
		},
	]
}

function NavLink({
	href,
	label,
	icon: Icon,
	active,
	external,
	preload,
	onNavigate,
	collapsed,
}: {
	href: string
	label: string
	icon: ElementType
	active: boolean
	external?: boolean
	preload?: () => Promise<unknown>
	onNavigate?: () => void
	collapsed?: boolean
}) {
	const className = cn(
		"relative flex items-center rounded-md text-sm font-medium transition-colors",
		collapsed ? "h-9 justify-center px-0" : "gap-3 px-3 py-2",
		active
			? "bg-accent text-accent-foreground"
			: "text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground"
	)

	const link = external ? (
		<a
			href={href}
			target="_blank"
			rel="noreferrer"
			onClick={onNavigate}
			className={className}
			aria-label={collapsed ? label : undefined}
		>
			<Icon className="size-4 shrink-0" strokeWidth={1.75} aria-hidden="true" />
			{!collapsed && <span className="truncate">{label}</span>}
			{!collapsed && <ExternalLinkIcon className="ms-auto size-3.5 opacity-50" aria-hidden="true" />}
		</a>
	) : (
		<Link
			href={href}
			onClick={onNavigate}
			onMouseEnter={collapsed ? undefined : preload}
			aria-current={active ? "page" : undefined}
			aria-label={collapsed ? label : undefined}
			className={className}
		>
			{active && !collapsed && (
				<span
					aria-hidden="true"
					className="absolute start-0 top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-full bg-primary"
				/>
			)}
			<Icon className="size-4 shrink-0" strokeWidth={1.75} aria-hidden="true" />
			{!collapsed && <span className="truncate">{label}</span>}
		</Link>
	)

	if (!collapsed) {
		return link
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>{link}</TooltipTrigger>
			<TooltipContent side="right">{label}</TooltipContent>
		</Tooltip>
	)
}

function NavSection({ label, collapsed, children }: { label: string; collapsed?: boolean; children: ReactNode }) {
	return (
		<div className="mt-5 first:mt-0">
			{collapsed ? (
				<div aria-hidden="true" className="mx-auto mb-2 h-px w-6 bg-border" />
			) : (
				<p className="eyebrow px-3 pb-1.5">{label}</p>
			)}
			<nav className="grid gap-0.5">{children}</nav>
		</div>
	)
}

function AdminLinks({ onNavigate, collapsed }: { onNavigate?: () => void; collapsed?: boolean }) {
	const items = [
		{ href: prependBasePath("/_/"), label: t`Users`, icon: UsersIcon },
		{ href: prependBasePath("/_/#/collections?collection=systems"), label: t`Systems`, icon: ServerIcon },
		{ href: prependBasePath("/_/#/logs"), label: t`Logs`, icon: LogsIcon },
		{ href: prependBasePath("/_/#/settings/backups"), label: t`Backups`, icon: DatabaseBackupIcon },
	]
	return (
		<div className="grid gap-0.5">
			{items.map(({ href, label, icon: Icon }) => (
				<NavLink
					key={label}
					href={href}
					label={label}
					icon={Icon}
					active={false}
					external
					onNavigate={onNavigate}
					collapsed={collapsed}
				/>
			))}
		</div>
	)
}

function SearchButton({ collapsed, onOpen }: { collapsed?: boolean; onOpen: () => void }) {
	if (collapsed) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<button
						type="button"
						onClick={onOpen}
						aria-label={t`Search`}
						className="grid h-9 w-full place-items-center rounded-md border bg-card text-muted-foreground shadow-xs transition-colors hover:border-input hover:text-foreground"
					>
						<SearchIcon className="size-4" aria-hidden="true" />
					</button>
				</TooltipTrigger>
				<TooltipContent side="right">
					<Trans>Search</Trans>
				</TooltipContent>
			</Tooltip>
		)
	}
	return (
		<button
			type="button"
			onClick={onOpen}
			className={cn(
				"flex h-9 w-full items-center gap-2.5 rounded-md border bg-card px-3 text-sm text-muted-foreground shadow-xs transition-colors hover:border-input hover:text-foreground"
			)}
		>
			<SearchIcon className="size-4 shrink-0" aria-hidden="true" />
			<span className="truncate">
				<Trans>Search</Trans>
			</span>
			<kbd className="ms-auto hidden rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground xl:inline-flex">
				⌘K
			</kbd>
		</button>
	)
}

function UserMenu({ collapsed, side = "top" }: { collapsed?: boolean; side?: "top" | "bottom" }) {
	const email = pb.authStore.record?.email
	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				className={cn(
					"flex min-w-0 flex-1 items-center gap-2.5 rounded-md px-1.5 py-1.5 text-start transition-colors hover:bg-accent/60",
					collapsed && "justify-center px-0 py-2"
				)}
				aria-label={t`User actions`}
			>
				<span className="grid size-8 shrink-0 place-items-center rounded-full bg-primary/12 font-display text-sm font-semibold text-primary">
					{(email?.[0] ?? "?").toUpperCase()}
				</span>
				{!collapsed && (
					<span className="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground">{email}</span>
				)}
			</DropdownMenuTrigger>
			<DropdownMenuContent side={side} align="start" className="min-w-52">
				<DropdownMenuLabel className="truncate">{email}</DropdownMenuLabel>
				<DropdownMenuSeparator />
				<DropdownMenuItem
					onClick={() => navigate(getPagePath($router, "settings", { name: "general" }))}
					onMouseEnter={preloadSettings}
				>
					<SettingsIcon className="me-2.5 h-4 w-4" />
					<span>
						<Trans>Settings</Trans>
					</span>
				</DropdownMenuItem>
				{isAdmin() && (
					<>
						<DropdownMenuSeparator />
						<DropdownMenuGroup>
							<AdminLinks />
						</DropdownMenuGroup>
					</>
				)}
				<DropdownMenuSeparator />
				<DropdownMenuItem onSelect={logOut}>
					<LogOutIcon className="me-2.5 h-4 w-4" />
					<span>
						<Trans>Log Out</Trans>
					</span>
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

function SidebarNav({ onNavigate, collapsed }: { onNavigate?: () => void; collapsed?: boolean }) {
	const items = useNavItems()
	return (
		<>
			<NavSection label={t`Monitor`} collapsed={collapsed}>
				{items.map((item) => (
					<NavLink key={item.href} {...item} onNavigate={onNavigate} collapsed={collapsed} />
				))}
			</NavSection>
			{isAdmin() && (
				<NavSection label={t`Manage`} collapsed={collapsed}>
					<AdminLinks onNavigate={onNavigate} collapsed={collapsed} />
				</NavSection>
			)}
		</>
	)
}

function Sidebar({
	collapsed,
	onToggleCollapsed,
	onSearch,
	onAddSystem,
}: {
	collapsed: boolean
	onToggleCollapsed: () => void
	onSearch: () => void
	onAddSystem: () => void
}) {
	return (
		<aside
			className={cn(
				"fixed inset-y-0 start-0 z-40 hidden flex-col border-e bg-card transition-[width] duration-200 ease-in-out lg:flex",
				collapsed ? "w-[3.75rem]" : "w-60"
			)}
		>
			<div
				className={cn(
					"flex h-16 shrink-0 items-center border-b",
					collapsed ? "flex-col justify-center gap-1 px-0" : "px-4"
				)}
			>
				<Link href={basePath} aria-label={t`Home`} onMouseEnter={preloadHome} className="flex shrink-0 items-center">
					{collapsed ? (
						<LogoMark className="h-[1.15rem] fill-foreground" />
					) : (
						<Logo className="h-[1.35rem] fill-foreground" />
					)}
				</Link>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon-sm"
							onClick={onToggleCollapsed}
							aria-label={collapsed ? t`Expand sidebar` : t`Collapse sidebar`}
							className={cn("shrink-0 text-muted-foreground", !collapsed && "ms-auto")}
						>
							{collapsed ? (
								<PanelLeftOpenIcon className="size-4" aria-hidden="true" />
							) : (
								<PanelLeftCloseIcon className="size-4" aria-hidden="true" />
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent side={collapsed ? "right" : "bottom"}>
						{collapsed ? t`Expand sidebar` : t`Collapse sidebar`}
					</TooltipContent>
				</Tooltip>
			</div>
			<div className={cn("flex-1 overflow-y-auto py-4", collapsed ? "px-2" : "px-3")}>
				<SearchButton collapsed={collapsed} onOpen={onSearch} />
				{!isReadOnlyUser() &&
					(collapsed ? (
						<Tooltip>
							<TooltipTrigger asChild>
								<Button className="mt-3 grid w-full px-0" onClick={onAddSystem} aria-label={t`Add System`}>
									<PlusIcon className="size-4" />
								</Button>
							</TooltipTrigger>
							<TooltipContent side="right">
								<Trans>Add System</Trans>
							</TooltipContent>
						</Tooltip>
					) : (
						<Button className="mt-3 w-full" onClick={onAddSystem}>
							<PlusIcon className="size-4" />
							<Trans>Add System</Trans>
						</Button>
					))}
				<div className="mt-5">
					<SidebarNav collapsed={collapsed} />
				</div>
			</div>
			<div className={cn("flex shrink-0 items-center gap-1 border-t p-2", collapsed && "flex-col px-0")}>
				<UserMenu collapsed={collapsed} />
				<ModeToggle />
			</div>
		</aside>
	)
}

function MobileTopBar({ onSearch, onAddSystem }: { onSearch: () => void; onAddSystem: () => void }) {
	const direction = useStore($direction)
	const [menuOpen, setMenuOpen] = useState(false)

	return (
		<header className="sticky top-0 z-40 border-b bg-background/85 backdrop-blur-lg lg:hidden">
			<div className="container flex h-14 items-center gap-1">
				<Sheet open={menuOpen} onOpenChange={setMenuOpen}>
					<SheetTrigger asChild>
						<Button variant="ghost" size="icon" aria-label={t`Open menu`}>
							<MenuIcon className="size-5" />
						</Button>
					</SheetTrigger>
					<SheetContent side={direction === "rtl" ? "right" : "left"} className="w-72 gap-0 p-0">
						<div className="flex h-14 items-center border-b px-5">
							<SheetTitle asChild>
								<Link href={basePath} onClick={() => setMenuOpen(false)} className="flex items-center">
									<Logo className="h-[1.35rem] fill-foreground" />
								</Link>
							</SheetTitle>
						</div>
						<div className="flex-1 overflow-y-auto px-3 py-4">
							{!isReadOnlyUser() && (
								<Button
									className="mb-5 w-full"
									onClick={() => {
										setMenuOpen(false)
										onAddSystem()
									}}
								>
									<PlusIcon className="size-4" />
									<Trans>Add System</Trans>
								</Button>
							)}
							<SidebarNav onNavigate={() => setMenuOpen(false)} />
						</div>
						<div className="flex items-center gap-1 border-t p-2">
							<UserMenu />
							<ModeToggle />
						</div>
					</SheetContent>
				</Sheet>
				<Link href={basePath} aria-label={t`Home`} className="ms-1 flex items-center">
					<Logo className="h-[1.25rem] fill-foreground" />
				</Link>
				<div className="ms-auto flex items-center gap-0.5">
					<Button variant="ghost" size="icon" aria-label={t`Search`} onClick={onSearch}>
						<SearchIcon className="size-[1.1rem]" />
					</Button>
					{!isReadOnlyUser() && (
						<Button variant="ghost" size="icon" aria-label={t`Add System`} onClick={onAddSystem}>
							<PlusIcon className="size-[1.1rem]" />
						</Button>
					)}
					<ModeToggle />
				</div>
			</div>
		</header>
	)
}

/** Application chrome: desktop sidebar + mobile top bar with drawer navigation. */
export function AppShell({ children }: { children: React.ReactNode }) {
	const [commandPaletteOpen, setCommandPaletteOpen] = useState(false)
	const [addSystemOpen, setAddSystemOpen] = useState(false)
	const [collapsed, setCollapsed] = useState(readCollapsedPreference)

	const toggleCollapsed = () =>
		setCollapsed((prev) => {
			const next = !prev
			try {
				localStorage.setItem(SIDEBAR_COLLAPSED_KEY, next ? "1" : "0")
			} catch {
				// preference just won't persist
			}
			return next
		})

	return (
		<div className="min-h-svh">
			<Suspense>
				<CommandPalette open={commandPaletteOpen} setOpen={setCommandPaletteOpen} />
			</Suspense>
			<AddSystemDialog open={addSystemOpen} setOpen={setAddSystemOpen} />
			<Sidebar
				collapsed={collapsed}
				onToggleCollapsed={toggleCollapsed}
				onSearch={() => setCommandPaletteOpen(true)}
				onAddSystem={() => setAddSystemOpen(true)}
			/>
			<div
				className={cn(
					"flex min-h-svh flex-col transition-[padding] duration-200 ease-in-out",
					collapsed ? "lg:ps-[3.75rem]" : "lg:ps-60"
				)}
			>
				<MobileTopBar onSearch={() => setCommandPaletteOpen(true)} onAddSystem={() => setAddSystemOpen(true)} />
				{children}
			</div>
		</div>
	)
}
