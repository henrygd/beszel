import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "@/components/ui/dialog"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { $publicKey, pb } from "@/lib/stores"
import { cn, copyToClipboard, isReadOnlyUser } from "@/lib/utils"
import { i18n } from "@lingui/core"
import { Trans } from "@lingui/macro"
import { useStore } from "@nanostores/react"
import { ChevronDownIcon, Copy, PlusIcon } from "lucide-react"
import { MutableRefObject, useRef, useState } from "react"
import { basePath, navigate } from "./router"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "./ui/dropdown-menu"

export function AddSystemButton({ className }: { className?: string }) {
	const [open, setOpen] = useState(false)
	const port = useRef() as MutableRefObject<HTMLInputElement>
	const publicKey = useStore($publicKey)

	function copyDockerCompose(port: string) {
		copyToClipboard(`services:
  beszel-agent:
    image: "henrygd/beszel-agent"
    container_name: "beszel-agent"
    restart: unless-stopped
    network_mode: host
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      # monitor other disks / partitions by mounting a folder in /extra-filesystems
      # - /mnt/disk/.beszel:/extra-filesystems/sda1:ro
    environment:
      PORT: ${port}
      KEY: "${publicKey}"`)
	}
	function copyDockerRun(port: string) {
		copyToClipboard(
			`docker run -d --name beszel-agent --network host --restart unless-stopped -v /var/run/docker.sock:/var/run/docker.sock:ro -e KEY="${publicKey}" -e PORT=${port} henrygd/beszel-agent:latest`
		)
	}

	function copyInstallCommand(port: string) {
		let cmd = `curl -sL https://raw.githubusercontent.com/henrygd/beszel/main/supplemental/scripts/install-agent.sh -o install-agent.sh && chmod +x install-agent.sh && ./install-agent.sh -p ${port} -k "${publicKey}"`
		// add china mirrors flag if zh-CN
		if ((i18n.locale + navigator.language).includes("zh-CN")) {
			cmd += ` --china-mirrors`
		}
		copyToClipboard(cmd)
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault()
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Record<string, any>
		data.users = pb.authStore.record!.id
		try {
			setOpen(false)
			await pb.collection("systems").create(data)
			navigate(basePath)
			// console.log(record)
		} catch (e) {
			console.log(e)
		}
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button
					variant="outline"
					className={cn("flex gap-1 max-xs:h-[2.4rem]", className, isReadOnlyUser() && "hidden")}
				>
					<PlusIcon className="h-4 w-4 -ms-1" />
					<Trans>
						Add <span className="hidden sm:inline">System</span>
					</Trans>
				</Button>
			</DialogTrigger>
			<DialogContent className="w-[90%] sm:w-auto sm:ns-dialog max-w-full rounded-lg">
				<Tabs defaultValue="docker">
					<DialogHeader>
						<DialogTitle className="mb-2">
							<Trans>Add New System</Trans>
						</DialogTitle>
						<TabsList className="grid w-full grid-cols-2">
							<TabsTrigger value="docker">Docker</TabsTrigger>
							<TabsTrigger value="binary">
								<Trans>Binary</Trans>
							</TabsTrigger>
						</TabsList>
					</DialogHeader>
					{/* Docker */}
					<TabsContent value="docker">
						<DialogDescription className="mb-4 leading-normal w-0 min-w-full">
							<Trans>
								The agent must be running on the system to connect. Copy the
								<code className="bg-muted px-1 rounded-sm leading-3">docker-compose.yml</code> for the agent below.
							</Trans>
						</DialogDescription>
					</TabsContent>
					{/* Binary */}
					<TabsContent value="binary">
						<DialogDescription className="mb-4 leading-normal w-0 min-w-full">
							<Trans>
								The agent must be running on the system to connect. Copy the installation command for the agent below.
							</Trans>
						</DialogDescription>
					</TabsContent>
					<form onSubmit={handleSubmit as any}>
						<div className="grid xs:grid-cols-[auto_1fr] gap-y-3 gap-x-4 items-center mt-1 mb-4">
							<Label htmlFor="name" className="xs:text-end">
								<Trans>Name</Trans>
							</Label>
							<Input id="name" name="name" className="" required />
							<Label htmlFor="host" className="xs:text-end">
								<Trans>Host / IP</Trans>
							</Label>
							<Input id="host" name="host" className="" required />
							<Label htmlFor="port" className="xs:text-end">
								<Trans>Port</Trans>
							</Label>
							<Input ref={port} name="port" id="port" defaultValue="45876" className="" required />
							<Label htmlFor="pkey" className="xs:text-end whitespace-pre">
								<Trans comment="Use 'Key' if your language requires many more characters">Public Key</Trans>
							</Label>
							<div className="relative">
								<Input readOnly id="pkey" value={publicKey} className="" required></Input>
								<div
									className={
										"h-6 w-24 bg-gradient-to-r rtl:bg-gradient-to-l from-transparent to-background to-65% absolute top-2 end-1 pointer-events-none"
									}
								></div>
								<TooltipProvider delayDuration={100}>
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												type="button"
												variant={"link"}
												className="absolute end-0 top-0"
												onClick={() => copyToClipboard(publicKey)}
											>
												<Copy className="h-4 w-4 " />
											</Button>
										</TooltipTrigger>
										<TooltipContent>
											<p>
												<Trans>Click to copy</Trans>
											</p>
										</TooltipContent>
									</Tooltip>
								</TooltipProvider>
							</div>
						</div>
						{/* Docker */}
						<TabsContent value="docker">
							<DialogFooter className="flex justify-end gap-x-2 gap-y-3 flex-col">
								<div className="flex gap-0 rounded-lg">
									<Button
										type="button"
										variant="outline"
										onClick={() => copyDockerCompose(port.current.value)}
										className="rounded-e-none dark:border-e-0 grow"
									>
										<Trans>Copy</Trans> docker compose
									</Button>
									<div className="w-px h-full bg-muted"></div>
									<DropdownMenu>
										<DropdownMenuTrigger asChild>
											<Button variant="outline" className={"px-2 rounded-s-none border-s-0"}>
												<ChevronDownIcon />
											</Button>
										</DropdownMenuTrigger>
										<DropdownMenuContent align="end">
											<DropdownMenuItem onClick={() => copyDockerRun(port.current.value)}>
												<Trans>Copy</Trans> docker run
											</DropdownMenuItem>
										</DropdownMenuContent>
									</DropdownMenu>
								</div>
								<Button>
									<Trans>Add system</Trans>
								</Button>
							</DialogFooter>
						</TabsContent>
						{/* Binary */}
						<TabsContent value="binary">
							<DialogFooter className="flex justify-end gap-x-2 gap-y-3 flex-col">
								<Button type="button" variant="outline" onClick={() => copyInstallCommand(port.current.value)}>
									<Trans>Copy Linux command</Trans>
								</Button>
								<Button>
									<Trans>Add system</Trans>
								</Button>
							</DialogFooter>
						</TabsContent>
					</form>
				</Tabs>
			</DialogContent>
		</Dialog>
	)
}
