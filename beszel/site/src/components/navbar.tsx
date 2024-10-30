import { useState, lazy, Suspense } from "react"
import { Button, buttonVariants } from "@/components/ui/button"
import {
	DatabaseBackupIcon,
	LockKeyholeIcon,
	LogOutIcon,
	LogsIcon,
	SearchIcon,
	ServerIcon,
	SettingsIcon,
	UserIcon,
	UsersIcon,
} from "lucide-react"
import { Link } from "./router"
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
import { useTranslation } from "react-i18next"
import { TFunction } from "i18next"

const CommandPalette = lazy(() => import("./command-palette"))

const isMac = navigator.platform.toUpperCase().indexOf("MAC") >= 0

export default function Navbar() {
	const { t } = useTranslation()
	return (
		<div className="flex items-center h-14 md:h-16 bg-card px-4 pr-3 sm:px-6 border bt-0 rounded-md my-4">
			<Link href="/" aria-label="Home" className={"p-2 pl-0"}>
				<Logo className="h-[1.15rem] md:h-[1.25em] fill-foreground" />
			</Link>

			<SearchButton t={t} />

			<div className={"flex ml-auto items-center"}>
				<LangToggle />
				<ModeToggle />
				<Link
					href="/settings/general"
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
						<DropdownMenuLabel>{pb.authStore.model?.email}</DropdownMenuLabel>
						<DropdownMenuSeparator />
						<DropdownMenuGroup>
							{isAdmin() && (
								<>
									<DropdownMenuItem asChild>
										<a href="/_/" target="_blank">
											<UsersIcon className="mr-2.5 h-4 w-4" />
											<span>{t("user_dm.users")}</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuItem asChild>
										<a href="/_/#/collections?collectionId=2hz5ncl8tizk5nx" target="_blank">
											<ServerIcon className="mr-2.5 h-4 w-4" />
											<span>{t("systems")}</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuItem asChild>
										<a href="/_/#/logs" target="_blank">
											<LogsIcon className="mr-2.5 h-4 w-4" />
											<span>{t("user_dm.logs")}</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuItem asChild>
										<a href="/_/#/settings/backups" target="_blank">
											<DatabaseBackupIcon className="mr-2.5 h-4 w-4" />
											<span>{t("user_dm.backups")}</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuItem asChild>
										<a href="/_/#/settings/auth-providers" target="_blank">
											<LockKeyholeIcon className="mr-2.5 h-4 w-4" />
											<span>{t("user_dm.auth_providers")}</span>
										</a>
									</DropdownMenuItem>
									<DropdownMenuSeparator />
								</>
							)}
						</DropdownMenuGroup>
						<DropdownMenuItem onSelect={() => pb.authStore.clear()}>
							<LogOutIcon className="mr-2.5 h-4 w-4" />
							<span>{t("user_dm.log_out")}</span>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				<AddSystemButton className="ml-2" />
			</div>
		</div>
	)
}

function SearchButton({ t }: { t: TFunction<"translation", undefined> }) {
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
				className="hidden md:block text-sm text-muted-foreground px-4 mx-3"
				onClick={() => setOpen(true)}
			>
				<span className="flex items-center">
					<SearchIcon className="mr-1.5 h-4 w-4" />
					{t("search")}
					<span className="sr-only">{t("search")}</span>
					<span className="flex items-center ml-3.5">
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
