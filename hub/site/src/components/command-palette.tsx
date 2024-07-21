'use client'

import {
	Database,
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
	const servers = useStore($systems)

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
			<CommandInput placeholder="Type a command or search..." />
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
						onSelect={() => {
							window.location.href = 'https://github.com/henrygd'
						}}
					>
						<Github className="mr-2 h-4 w-4" />
						<span>Documentation</span>
						<CommandShortcut>GitHub</CommandShortcut>
					</CommandItem>
				</CommandGroup>
				<CommandSeparator />
				<CommandGroup heading="Servers">
					{servers.map((server) => (
						<CommandItem
							key={server.id}
							onSelect={() => {
								navigate(`/server/${server.name}`)
								setOpen((open) => !open)
							}}
						>
							<Server className="mr-2 h-4 w-4" />
							<span>{server.name}</span>
							<CommandShortcut>{server.host}</CommandShortcut>
						</CommandItem>
					))}
				</CommandGroup>
				{isAdmin() && (
					<>
						<CommandSeparator />
						<CommandGroup heading="Admin">
							<CommandItem
								keywords={['pocketbase']}
								onSelect={() => {
									window.open('/_/', '_blank')
								}}
							>
								<UsersIcon className="mr-2 h-4 w-4" />
								<span>Users</span>
								<CommandShortcut>Admin</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
									window.open('/_/#/logs', '_blank')
								}}
							>
								<LogsIcon className="mr-2 h-4 w-4" />
								<span>Logs</span>
								<CommandShortcut>Admin</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
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
