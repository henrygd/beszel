import { Button } from '@/components/ui/button'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from '@/components/ui/dialog'
import { TooltipProvider, Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { $publicKey, pb } from '@/lib/stores'
import { Copy, Plus } from 'lucide-react'
import { useState, useRef, MutableRefObject, useEffect } from 'react'
import { useStore } from '@nanostores/react'
import { copyToClipboard } from '@/lib/utils'
import { SystemStats } from '@/types'

export function AddServerButton() {
	const [open, setOpen] = useState(false)
	const port = useRef() as MutableRefObject<HTMLInputElement>
	const publicKey = useStore($publicKey)

	function copyDockerCompose(port: string) {
		copyToClipboard(`services:
  agent:
    image: 'henrygd/quoma-agent'
    container_name: 'quoma-agent'
    restart: unless-stopped
    ports:
      - '${port}:45876'
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - KEY="${publicKey}"`)
	}

	useEffect(() => {
		if (publicKey || !open) {
			return
		}
		// get public key
		pb.send('/getkey', {}).then(({ key }) => {
			console.log('key', key)
			$publicKey.set(key)
		})
	}, [open])

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault()
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Record<string, any>
		data.stats = {
			c: 0,
			d: 0,
			dp: 0,
			du: 0,
			m: 0,
			mp: 0,
			mu: 0,
		} as SystemStats
		try {
			setOpen(false)
			await pb.collection('systems').create(data)
			// console.log(record)
		} catch (e) {
			console.log(e)
		}
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button variant="outline" className="flex gap-1">
					<Plus className="h-4 w-4 mr-auto" />
					Add Server
				</Button>
			</DialogTrigger>
			<DialogContent className="sm:max-w-[425px]">
				<DialogHeader>
					<DialogTitle>Add New Server</DialogTitle>
					<DialogDescription>
						The agent must be running on the server to connect. Copy the{' '}
						<code className="bg-muted px-1 rounded-sm">docker-compose.yml</code> for the agent
						below.
					</DialogDescription>
				</DialogHeader>
				<form name="testing" action="/" onSubmit={handleSubmit as any}>
					<div className="grid gap-4 py-4">
						<div className="grid grid-cols-4 items-center gap-4">
							<Label htmlFor="name" className="text-right">
								Name
							</Label>
							<Input id="name" name="name" className="col-span-3" required />
						</div>
						<div className="grid grid-cols-4 items-center gap-4">
							<Label htmlFor="ip" className="text-right">
								Host / IP
							</Label>
							<Input id="ip" name="ip" className="col-span-3" required />
						</div>
						<div className="grid grid-cols-4 items-center gap-4">
							<Label htmlFor="port" className="text-right">
								Port
							</Label>
							<Input
								ref={port}
								name="port"
								id="port"
								defaultValue="45876"
								className="col-span-3"
								required
							/>
						</div>
						<div className="grid grid-cols-4 items-center gap-4 relative">
							<Label htmlFor="pkey" className="text-right">
								Public Key
							</Label>
							<Input readOnly id="pkey" value={publicKey} className="col-span-3" required></Input>
							<div
								className={
									'h-6 w-24 bg-gradient-to-r from-transparent to-background to-65% absolute right-1 pointer-events-none'
								}
							></div>
							<TooltipProvider delayDuration={100}>
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											type="button"
											variant={'link'}
											className="absolute right-0"
											onClick={() => copyToClipboard(publicKey)}
										>
											<Copy className="h-4 w-4 " />
										</Button>
									</TooltipTrigger>
									<TooltipContent>
										<p>Click to copy</p>
									</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						</div>
					</div>
					<DialogFooter>
						<Button
							type="button"
							variant={'ghost'}
							onClick={() => copyDockerCompose(port.current.value)}
						>
							Copy docker compose
						</Button>
						<Button>Add server</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
