import { memo, useEffect, useMemo } from "react"
import { Trans } from "@lingui/react/macro"
import { t } from "@lingui/core/macro"
import { useQuery } from "@tanstack/react-query"
import { DialogDescription } from "@radix-ui/react-dialog"
import {
	Activity,
	AlertOctagonIcon,
	AlertTriangle,
	BookIcon,
	Calendar,
	ContainerIcon,
	DatabaseBackupIcon,
	FingerprintIcon,
	GlobeIcon,
	HardDriveIcon,
	LogsIcon,
	MailIcon,
	NetworkIcon,
	Server,
	ServerIcon,
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
import { isAdmin } from "@/lib/api"
import { $systems } from "@/lib/stores"
import { listMonitors } from "@/lib/monitors"
import { getDomains } from "@/lib/domains"
import { getPagePath } from "@nanostores/router"
import { $router, basePath, navigate, prependBasePath } from "./router"
import { getHostDisplayValue, listen } from "@/lib/utils"

export default memo(function CommandPalette({ open, setOpen }: { open: boolean; setOpen: (open: boolean) => void }) {
	useEffect(() => {
		const down = (e: KeyboardEvent) => {
			if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
				e.preventDefault()
				setOpen(!open)
			}
		}
		return listen(document, "keydown", down)
	}, [open, setOpen])

	const { data: monitors = [] } = useQuery({
		queryKey: ["monitors"],
		queryFn: listMonitors,
		enabled: open,
	})

	const { data: domains = [] } = useQuery({
		queryKey: ["domains"],
		queryFn: getDomains,
		enabled: open,
	})

	return useMemo(() => {
		const systems = $systems.get()
		const SettingsShortcut = (
			<CommandShortcut>
				<Trans>Settings</Trans>
			</CommandShortcut>
		)
		const AdminShortcut = (
			<CommandShortcut>
				<Trans>Admin</Trans>
			</CommandShortcut>
		)
		return (
			<CommandDialog open={open} onOpenChange={setOpen}>
				<DialogDescription className="sr-only">Command palette</DialogDescription>
				<CommandInput placeholder={t`Search for systems, monitors, domains or settings...`} />
				<CommandList>
					{systems.length > 0 && (
						<>
							<CommandGroup>
								{systems.map((system) => (
									<CommandItem
										key={system.id}
										onSelect={() => {
											navigate(getPagePath($router, "system", { id: system.id }))
											setOpen(false)
										}}
									>
										<Server className="me-2 size-4" />
										<span className="max-w-60 truncate">{system.name}</span>
										<CommandShortcut>{getHostDisplayValue(system)}</CommandShortcut>
									</CommandItem>
								))}
							</CommandGroup>
							<CommandSeparator className="mb-1.5" />
						</>
					)}
					{monitors.length > 0 && (
						<>
							<CommandGroup heading={t`Monitors`}>
								{monitors.map((monitor) => (
									<CommandItem
										key={monitor.id}
										onSelect={() => {
											setOpen(false)
											$router.open(getPagePath($router, "monitor", { id: monitor.id }))
										}}
									>
										<Activity className="me-2 size-4" />
										<span className="max-w-60 truncate">{monitor.name}</span>
									</CommandItem>
								))}
							</CommandGroup>
							<CommandSeparator className="mb-1.5" />
						</>
					)}
					{domains.length > 0 && (
						<>
							<CommandGroup heading={t`Domains`}>
								{domains.map((domain) => (
									<CommandItem
										key={domain.id}
										onSelect={() => {
											setOpen(false)
											$router.open(getPagePath($router, "domain", { id: domain.id }))
										}}
									>
										<GlobeIcon className="me-2 size-4" />
										<span className="max-w-60 truncate">{domain.domain_name}</span>
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
							<ServerIcon className="me-2 size-4" />
							<span>
								<Trans>All Systems</Trans>
							</span>
							<CommandShortcut>
								<Trans>Page</Trans>
							</CommandShortcut>
						</CommandItem>
						<CommandItem
							keywords={["containers", "docker", "podman"]}
							onSelect={() => {
								navigate(getPagePath($router, "containers"))
								setOpen(false)
							}}
						>
							<ContainerIcon className="me-2 size-4" />
							<span>
								<Trans>All Containers</Trans>
							</span>
							<CommandShortcut>
								<Trans>Page</Trans>
							</CommandShortcut>
						</CommandItem>
						<CommandItem
							onSelect={() => {
								navigate(getPagePath($router, "smart"))
								setOpen(false)
							}}
						>
							<HardDriveIcon className="me-2 size-4" />
							<span>S.M.A.R.T.</span>
							<CommandShortcut>
								<Trans>Page</Trans>
							</CommandShortcut>
						</CommandItem>
						<CommandItem
							keywords={["monitoring", "monitors", "domains", "websites"]}
							onSelect={() => {
								navigate(getPagePath($router, "monitoring"))
								setOpen(false)
							}}
						>
							<Activity className="me-2 size-4" />
							<span>
								<Trans>Monitoring</Trans>
							</span>
							<CommandShortcut>
								<Trans>Page</Trans>
							</CommandShortcut>
						</CommandItem>
						<CommandItem
							keywords={["status", "public"]}
							onSelect={() => {
								navigate(getPagePath($router, "status_pages"))
								setOpen(false)
							}}
						>
							<GlobeIcon className="me-2 size-4" />
							<span>
								<Trans>Status Pages</Trans>
							</span>
							<CommandShortcut>
								<Trans>Page</Trans>
							</CommandShortcut>
						</CommandItem>
						<CommandItem
							keywords={["incidents", "problems"]}
							onSelect={() => {
								navigate(getPagePath($router, "incidents"))
								setOpen(false)
							}}
						>
							<AlertTriangle className="me-2 size-4" />
							<span>
								<Trans>Incidents</Trans>
							</span>
							<CommandShortcut>
								<Trans>Page</Trans>
							</CommandShortcut>
						</CommandItem>
						<CommandItem
							keywords={["calendar", "expiry", "ssl"]}
							onSelect={() => {
								navigate(getPagePath($router, "calendar"))
								setOpen(false)
							}}
						>
							<Calendar className="me-2 size-4" />
							<span>
								<Trans>Calendar</Trans>
							</span>
							<CommandShortcut>
								<Trans>Page</Trans>
							</CommandShortcut>
						</CommandItem>
						<CommandItem
							onSelect={() => {
								navigate(getPagePath($router, "monitors"))
								setOpen(false)
							}}
						>
							<NetworkIcon className="me-2 size-4" />
							<span>
								<Trans>Network Monitors</Trans>
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
							<SettingsIcon className="me-2 size-4" />
							<span>
								<Trans>Settings</Trans>
							</span>
							{SettingsShortcut}
						</CommandItem>
						<CommandItem
							keywords={["alerts"]}
							onSelect={() => {
								navigate(getPagePath($router, "settings", { name: "notifications" }))
								setOpen(false)
							}}
						>
							<MailIcon className="me-2 size-4" />
							<span>
								<Trans>Notifications</Trans>
							</span>
							{SettingsShortcut}
						</CommandItem>
						<CommandItem
							keywords={[t`Universal token`]}
							onSelect={() => {
								navigate(getPagePath($router, "settings", { name: "tokens" }))
								setOpen(false)
							}}
						>
							<FingerprintIcon className="me-2 size-4" />
							<span>
								<Trans>Tokens & Fingerprints</Trans>
							</span>
							{SettingsShortcut}
						</CommandItem>
						<CommandItem
							onSelect={() => {
								navigate(getPagePath($router, "settings", { name: "alert-history" }))
								setOpen(false)
							}}
						>
							<AlertOctagonIcon className="me-2 size-4" />
							<span>
								<Trans>Alert History</Trans>
							</span>
							{SettingsShortcut}
						</CommandItem>
						<CommandItem
							keywords={["help", "oauth", "oidc"]}
							onSelect={() => {
								window.location.href = "https://beszel.dev/guide/what-is-beszel"
							}}
						>
							<BookIcon className="me-2 size-4" />
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
										window.open(prependBasePath("/_/"), "_blank")
									}}
								>
									<UsersIcon className="me-2 size-4" />
									<span>
										<Trans>Users</Trans>
									</span>
									{AdminShortcut}
								</CommandItem>
								<CommandItem
									onSelect={() => {
										setOpen(false)
										window.open(prependBasePath("/_/#/logs"), "_blank")
									}}
								>
									<LogsIcon className="me-2 size-4" />
									<span>
										<Trans>Logs</Trans>
									</span>
									{AdminShortcut}
								</CommandItem>
								<CommandItem
									onSelect={() => {
										setOpen(false)
										window.open(prependBasePath("/_/#/settings/backups"), "_blank")
									}}
								>
									<DatabaseBackupIcon className="me-2 size-4" />
									<span>
										<Trans>Backups</Trans>
									</span>
									{AdminShortcut}
								</CommandItem>
								<CommandItem
									keywords={["email"]}
									onSelect={() => {
										setOpen(false)
										window.open(prependBasePath("/_/#/settings/mail"), "_blank")
									}}
								>
									<MailIcon className="me-2 size-4" />
									<span>
										<Trans>SMTP settings</Trans>
									</span>
									{AdminShortcut}
								</CommandItem>
							</CommandGroup>
						</>
					)}
					<CommandEmpty>
						<Trans>No results found.</Trans>
					</CommandEmpty>
				</CommandList>
			</CommandDialog>
		)
	}, [open])
})
