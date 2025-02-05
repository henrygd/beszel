import { useState, lazy, Suspense } from "react"
import { Button, buttonVariants } from "@/components/ui/button"
import {
	DatabaseBackupIcon,
	LogOutIcon,
	LogsIcon,
	SearchIcon,
	ServerIcon,
	SettingsIcon,
	UserIcon,
	UsersIcon,
} from "lucide-react"
import { $router, basePath, Link, prependBasePath } from "./router"
import { LangToggle } from "./lang-toggle"
import { ModeToggle } from "./mode-toggle"
import { Logo } from "./logo"
import { pb } from "@/lib/stores"
import { cn, isReadOnlyUser, isAdmin } from "@/lib/utils"
import {
	DropdownMenu,
	DropdownMenuTrigger,
	DropdownMenuContent,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuGroup,
	DropdownMenuItem,
} from "@/components/ui/dropdown-menu"
import { AddSystemButton } from "./add-system"
import { Trans } from "@lingui/macro"
import { getPagePath } from "@nanostores/router"

const CommandPalette = lazy(() => import("./command-palette"))

const isMac = navigator.platform.toUpperCase().indexOf("MAC") >= 0

export default function Navbar() {
	return (
		<div className="flex items-center h-14 md:h-16 bg-card px-4 pe-3 sm:px-6 border border-border/60 bt-0 rounded-md my-4">
			<Link href={basePath} aria-label="Home" className="p-2 ps-0 me-3">
				<Logo className="h-[1.1rem] md:h-5 fill-foreground" />
			</Link>
			<SearchButton />

			<div className="flex items-center ms-auto">
				<LangToggle />
				<ModeToggle />
				<Link
					href={getPagePath($router, "settings", { name: "general" })}
					aria-label="Settings"
					className={cn("", buttonVariants({ variant: "ghost", size: "icon" }))}
				>
					<SettingsIcon className="h-[1.2rem] w-[1.2rem]" />
				</Link>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<button aria-label="User Actions" className={cn("", buttonVariants({ variant: "ghost", size: "icon" }))}>
							<UserIcon className="h-[1.2rem] w-[1.2rem]" />
						</button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align={isReadOnlyUser() ? "end" : "center"} className="min-w-44">
						<DropdownMenuLabel>{pb.authStore.record?.email}</DropdownMenuLabel>
						<DropdownMenuSeparator />
						<DropdownMenuGroup>
							{isAdmin() && (
								<>
									<DropdownMenuItem asChild>
										<a href={prependBasePath("/_/")} target="_blank">
											<UsersIcon className="me-2.5 h-4 w-4" />
											<span>
												<Trans>Users</Trans>
											</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuItem asChild>
										<a href={prependBasePath("/_/#/collections?collection=systems")} target="_blank">
											<ServerIcon className="me-2.5 h-4 w-4" />
											<span>
												<Trans>Systems</Trans>
											</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuItem asChild>
										<a href={prependBasePath("/_/#/logs")} target="_blank">
											<LogsIcon className="me-2.5 h-4 w-4" />
											<span>
												<Trans>Logs</Trans>
											</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuItem asChild>
										<a href={prependBasePath("/_/#/settings/backups")} target="_blank">
											<DatabaseBackupIcon className="me-2.5 h-4 w-4" />
											<span>
												<Trans>Backups</Trans>
											</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuSeparator />
								</>
							)}
						</DropdownMenuGroup>
						<DropdownMenuItem onSelect={() => pb.authStore.clear()}>
							<LogOutIcon className="me-2.5 h-4 w-4" />
							<span>
								<Trans>Log Out</Trans>
							</span>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				<AddSystemButton className="ms-2" />
			</div>
		</div>
	)
}

function SearchButton() {
	const [open, setOpen] = useState(false)

	const Kbd = ({ children }: { children: React.ReactNode }) => (
		<kbd className="pointer-events-none inline-flex h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground opacity-100">
			{children}
		</kbd>
	)

	return (
		<>
			<Button
				variant="outline"
				className="hidden md:block text-sm text-muted-foreground px-4"
				onClick={() => setOpen(true)}
			>
				<span className="flex items-center">
					<SearchIcon className="me-1.5 h-4 w-4" />
					<Trans>Search</Trans>
					<span className="flex items-center ms-3.5">
						<Kbd>{isMac ? "⌘" : "Ctrl"}</Kbd>
						<Kbd>K</Kbd>
					</span>
				</span>
			</Button>
			<Suspense>
				<CommandPalette open={open} setOpen={setOpen} />
			</Suspense>
		</>
	)
}
