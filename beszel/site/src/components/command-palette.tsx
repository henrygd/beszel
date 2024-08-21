import {
	DatabaseBackupIcon,
	Github,
	LayoutDashboard,
	LockKeyholeIcon,
	LogsIcon,
	MailIcon,
	Server,
	UsersIcon,
} from 'lucide-react'

import {
	CommandDialog,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
	CommandSeparator,
	CommandShortcut,
} from '@/components/ui/command'
import { useEffect, useState } from 'react'
import { useStore } from '@nanostores/react'
import { $systems } from '@/lib/stores'
import { isAdmin } from '@/lib/utils'
import { navigate } from './router'

export default function CommandPalette() {
	const [open, setOpen] = useState(false)
	const systems = useStore($systems)

	useEffect(() => {
		const down = (e: KeyboardEvent) => {
			if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
				e.preventDefault()
				setOpen((open) => !open)
			}
		}

		document.addEventListener('keydown', down)
		return () => document.removeEventListener('keydown', down)
	}, [])

	return (
		<CommandDialog open={open} onOpenChange={setOpen}>
			<CommandInput placeholder="Search for systems or settings..." />
			<CommandList>
				<CommandEmpty>No results found.</CommandEmpty>
				<CommandGroup heading="Suggestions">
					<CommandItem
						keywords={['home']}
						onSelect={() => {
							navigate('/')
							setOpen((open) => !open)
						}}
					>
						<LayoutDashboard className="mr-2 h-4 w-4" />
						<span>Dashboard</span>
						<CommandShortcut>Page</CommandShortcut>
					</CommandItem>
					<CommandItem
						keywords={['github']}
						onSelect={() => {
							window.location.href = 'https://github.com/henrygd/beszel/blob/main/readme.md'
						}}
					>
						<Github className="mr-2 h-4 w-4" />
						<span>Documentation</span>
						<CommandShortcut>GitHub</CommandShortcut>
					</CommandItem>
				</CommandGroup>
				{systems.length > 0 && (
					<>
						<CommandSeparator />
						<CommandGroup heading="Systems">
							{systems.map((system) => (
								<CommandItem
									key={system.id}
									onSelect={() => {
										navigate(`/system/${encodeURIComponent(system.name)}`)
										setOpen(false)
									}}
								>
									<Server className="mr-2 h-4 w-4" />
									<span>{system.name}</span>
									<CommandShortcut>{system.host}</CommandShortcut>
								</CommandItem>
							))}
						</CommandGroup>
					</>
				)}
				{isAdmin() && (
					<>
						<CommandSeparator />
						<CommandGroup heading="Admin">
							<CommandItem
								keywords={['pocketbase']}
								onSelect={() => {
									setOpen(false)
									window.open('/_/', '_blank')
								}}
							>
								<UsersIcon className="mr-2 h-4 w-4" />
								<span>Users</span>
								<CommandShortcut>Admin</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/logs', '_blank')
								}}
							>
								<LogsIcon className="mr-2 h-4 w-4" />
								<span>Logs</span>
								<CommandShortcut>Admin</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/settings/backups', '_blank')
								}}
							>
								<DatabaseBackupIcon className="mr-2 h-4 w-4" />
								<span>Database backups</span>
								<CommandShortcut>Admin</CommandShortcut>
							</CommandItem>
							<CommandItem
								keywords={['oauth', 'oicd']}
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/settings/auth-providers', '_blank')
								}}
							>
								<LockKeyholeIcon className="mr-2 h-4 w-4" />
								<span>Auth Providers</span>
								<CommandShortcut>Admin</CommandShortcut>
							</CommandItem>
							<CommandItem
								keywords={['email']}
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/settings/mail', '_blank')
								}}
							>
								<MailIcon className="mr-2 h-4 w-4" />
								<span>SMTP settings</span>
								<CommandShortcut>Admin</CommandShortcut>
							</CommandItem>
						</CommandGroup>
					</>
				)}
			</CommandList>
		</CommandDialog>
	)
}
