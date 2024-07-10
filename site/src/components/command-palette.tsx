'use client'

import { Database, Github, Home, Server } from 'lucide-react'

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
import { navigate } from 'wouter/use-browser-location'
import { useStore } from '@nanostores/react'
import { $servers } from '@/lib/stores'

export function CommandPalette() {
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
						onSelect={() => {
							navigate('/')
							setOpen((open) => !open)
						}}
					>
						<Home className="mr-2 h-4 w-4" />
						<span>Home</span>
						<CommandShortcut>Page</CommandShortcut>
					</CommandItem>
					<CommandItem
						onSelect={() => {
							window.location.href = '/_/#/collections?collectionId=2hz5ncl8tizk5nx'
						}}
					>
						<Database className="mr-2 h-4 w-4" />
						<span>Admin UI</span>
						<CommandShortcut>PocketBase</CommandShortcut>
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
						</CommandItem>
					))}
				</CommandGroup>
			</CommandList>
		</CommandDialog>
	)
}
