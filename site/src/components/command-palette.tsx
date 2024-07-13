'use client'

import {
	Database,
	DatabaseBackupIcon,
	Github,
	LayoutDashboard,
	MailIcon,
	Server,
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
import { $servers, navigate } from '@/lib/stores'

export default function CommandPalette() {
	const [open, setOpen] = useState(false)
	const servers = useStore($servers)

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
				<CommandSeparator />
				<CommandGroup heading="Admin">
					<CommandItem
						onSelect={() => {
							window.location.href = '/_/#/collections?collectionId=2hz5ncl8tizk5nx'
						}}
					>
						<Database className="mr-2 h-4 w-4" />
						<span>PocketBase</span>
						<CommandShortcut>Admin</CommandShortcut>
					</CommandItem>
					<CommandItem
						keywords={['email']}
						onSelect={() => {
							window.location.href = '/_/#/settings/backups'
						}}
					>
						<DatabaseBackupIcon className="mr-2 h-4 w-4" />
						<span>Database backups</span>
						<CommandShortcut>Admin</CommandShortcut>
					</CommandItem>
					<CommandItem
						keywords={['email']}
						onSelect={() => {
							window.location.href = '/_/#/settings/mail'
						}}
					>
						<MailIcon className="mr-2 h-4 w-4" />
						<span>SMTP settings</span>
						<CommandShortcut>Admin</CommandShortcut>
					</CommandItem>
				</CommandGroup>
			</CommandList>
		</CommandDialog>
	)
}
