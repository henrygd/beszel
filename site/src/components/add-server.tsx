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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { pb } from '@/lib/stores'
import { Plus } from 'lucide-react'
import { MutableRef, useRef, useState } from 'preact/hooks'

function copyDockerCompose(port: string) {
	console.log('copying docker compose')
	navigator.clipboard.writeText(`services:
  agent:
    image: 'henrygd/monitor-agent'
    container_name: 'monitor-agent'
    restart: unless-stopped
    ports:
      - '${port}:45876'
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock`)
}

export function AddServerButton() {
	const [open, setOpen] = useState(false)
	const port = useRef() as MutableRef<HTMLInputElement>

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault()
		const formData = new FormData(e.target as HTMLFormElement)
		const stats = {
			cpu: 0,
			mem: 0,
			memUsed: 0,
			memPct: 0,
			disk: 0,
			diskUsed: 0,
			diskPct: 0,
		}
		const data = { stats } as Record<string, any>
		for (const [key, value] of formData) {
			data[key.slice(2)] = value
		}
		console.log(data)

		try {
			const record = await pb.collection('systems').create(data)
			console.log(record)
			setOpen(false)
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
						<code class="bg-muted px-1 rounded-sm">docker-compose.yml</code> for the agent below.
					</DialogDescription>
				</DialogHeader>
				<form name="testing" action="/" onSubmit={handleSubmit}>
					<div className="grid gap-4 py-4">
						<div className="grid grid-cols-4 items-center gap-4">
							<Label for="s-name" className="text-right">
								Name
							</Label>
							<Input id="s-name" name="s-name" className="col-span-3" required />
						</div>
						<div className="grid grid-cols-4 items-center gap-4">
							<Label for="s-ip" className="text-right">
								IP Address
							</Label>
							<Input id="s-ip" name="s-ip" className="col-span-3" required />
						</div>
						<div className="grid grid-cols-4 items-center gap-4">
							<Label for="s-port" className="text-right">
								Port
							</Label>
							<Input
								ref={port}
								name="s-port"
								id="s-port"
								defaultValue="45876"
								className="col-span-3"
								required
							/>
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
