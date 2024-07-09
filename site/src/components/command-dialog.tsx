'use client'

import { Database, Home, Server } from 'lucide-react'

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
import { useEffect, useState } from 'preact/hooks'
import { navigate } from 'wouter-preact/use-browser-location'

export function CommandPalette() {
	const [open, setOpen] = useState(false)

	useEffect(() => {
		const down = (e: KeyboardEvent) => {
			if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
				console.log('open')
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
					</CommandItem>
					<CommandItem
						onSelect={() => {
							window.location.href = '/_/'
						}}
					>
						<Database className="mr-2 h-4 w-4" />
						<span>PocketBase</span>
						<CommandShortcut>⌘P</CommandShortcut>
					</CommandItem>
				</CommandGroup>
				<CommandSeparator />
				<CommandGroup heading="Systems">
					<CommandItem
						onSelect={() => {
							navigate('/server/kagemusha')
							setOpen((open) => !open)
						}}
					>
						<Server className="mr-2 h-4 w-4" />
						<span>Kagemusha</span>
					</CommandItem>
					<CommandItem onSelect={() => navigate('/server/rashomon')}>
						<Server className="mr-2 h-4 w-4" />
						<span>Rashomon</span>
					</CommandItem>
					<CommandItem onSelect={() => navigate('/server/ikiru')}>
						<Server className="mr-2 h-4 w-4" />
						<span>Ikiru</span>
					</CommandItem>
				</CommandGroup>
			</CommandList>
		</CommandDialog>
	)
}
