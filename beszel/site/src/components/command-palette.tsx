import {
	BookIcon,
	DatabaseBackupIcon,
	LayoutDashboard,
	LogsIcon,
	MailIcon,
	Server,
	SettingsIcon,
	UsersIcon,
} from "lucide-react"

import {
	CommandDialog,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
	CommandSeparator,
	CommandShortcut,
} from "@/components/ui/command"
import { useEffect } from "react"
import { useStore } from "@nanostores/react"
import { $systems } from "@/lib/stores"
import { isAdmin } from "@/lib/utils"
import { $router, basePath, navigate } from "./router"
import { Trans, t } from "@lingui/macro"
import { getPagePath } from "@nanostores/router"

export default function CommandPalette({ open, setOpen }: { open: boolean; setOpen: (open: boolean) => void }) {
	const systems = useStore($systems)

	useEffect(() => {
		const down = (e: KeyboardEvent) => {
			if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
				e.preventDefault()
				setOpen(!open)
			}
		}

		document.addEventListener("keydown", down)
		return () => document.removeEventListener("keydown", down)
	}, [open, setOpen])

	return (
		<CommandDialog open={open} onOpenChange={setOpen}>
			<CommandInput placeholder={t`Search for systems or settings...`} />
			<CommandList>
				<CommandEmpty>
					<Trans>No results found.</Trans>
				</CommandEmpty>
				{systems.length > 0 && (
					<>
						<CommandGroup>
							{systems.map((system) => (
								<CommandItem
									key={system.id}
									onSelect={() => {
										navigate(getPagePath($router, "system", { name: system.name }))
										setOpen(false)
									}}
								>
									<Server className="me-2 h-4 w-4" />
									<span>{system.name}</span>
									<CommandShortcut>{system.host}</CommandShortcut>
								</CommandItem>
							))}
						</CommandGroup>
						<CommandSeparator className="mb-1.5" />
					</>
				)}
				<CommandGroup heading={t`Pages / Settings`}>
					<CommandItem
						keywords={["home"]}
						onSelect={() => {
							navigate(basePath)
							setOpen(false)
						}}
					>
						<LayoutDashboard className="me-2 h-4 w-4" />
						<span>
							<Trans>Dashboard</Trans>
						</span>
						<CommandShortcut>
							<Trans>Page</Trans>
						</CommandShortcut>
					</CommandItem>
					<CommandItem
						onSelect={() => {
							navigate(getPagePath($router, "settings", { name: "general" }))
							setOpen(false)
						}}
					>
						<SettingsIcon className="me-2 h-4 w-4" />
						<span>
							<Trans>Settings</Trans>
						</span>
						<CommandShortcut>
							<Trans>Settings</Trans>
						</CommandShortcut>
					</CommandItem>
					<CommandItem
						keywords={["alerts"]}
						onSelect={() => {
							navigate(getPagePath($router, "settings", { name: "notifications" }))
							setOpen(false)
						}}
					>
						<MailIcon className="me-2 h-4 w-4" />
						<span>
							<Trans>Notifications</Trans>
						</span>
						<CommandShortcut>
							<Trans>Settings</Trans>
						</CommandShortcut>
					</CommandItem>
					<CommandItem
						keywords={["help", "oauth", "oidc"]}
						onSelect={() => {
							window.location.href = "https://beszel.dev/guide/what-is-beszel"
						}}
					>
						<BookIcon className="me-2 h-4 w-4" />
						<span>
							<Trans>Documentation</Trans>
						</span>
						<CommandShortcut>beszel.dev</CommandShortcut>
					</CommandItem>
				</CommandGroup>
				{isAdmin() && (
					<>
						<CommandSeparator className="mb-1.5" />
						<CommandGroup heading={t`Admin`}>
							<CommandItem
								keywords={["pocketbase"]}
								onSelect={() => {
									setOpen(false)
									window.open("/_/", "_blank")
								}}
							>
								<UsersIcon className="me-2 h-4 w-4" />
								<span>
									<Trans>Users</Trans>
								</span>
								<CommandShortcut>
									<Trans>Admin</Trans>
								</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
									setOpen(false)
									window.open("/_/#/logs", "_blank")
								}}
							>
								<LogsIcon className="me-2 h-4 w-4" />
								<span>
									<Trans>Logs</Trans>
								</span>
								<CommandShortcut>
									<Trans>Admin</Trans>
								</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
									setOpen(false)
									window.open("/_/#/settings/backups", "_blank")
								}}
							>
								<DatabaseBackupIcon className="me-2 h-4 w-4" />
								<span>
									<Trans>Backups</Trans>
								</span>
								<CommandShortcut>
									<Trans>Admin</Trans>
								</CommandShortcut>
							</CommandItem>
							<CommandItem
								keywords={["email"]}
								onSelect={() => {
									setOpen(false)
									window.open("/_/#/settings/mail", "_blank")
								}}
							>
								<MailIcon className="me-2 h-4 w-4" />
								<span>
									<Trans>SMTP settings</Trans>
								</span>
								<CommandShortcut>
									<Trans>Admin</Trans>
								</CommandShortcut>
							</CommandItem>
						</CommandGroup>
					</>
				)}
			</CommandList>
		</CommandDialog>
	)
}
