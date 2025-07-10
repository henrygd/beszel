import { Trans } from "@lingui/react/macro"
import { t } from "@lingui/core/macro"
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
import { cn, copyToClipboard, isReadOnlyUser, useLocalStorage } from "@/lib/utils"
import { i18n } from "@lingui/core"
import { useStore } from "@nanostores/react"
import { ChevronDownIcon, Copy, ExternalLinkIcon, PlusIcon } from "lucide-react"
import { memo, useRef, useState } from "react"
import { basePath, navigate } from "./router"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "./ui/dropdown-menu"
import { SystemRecord } from "@/types"
import { AppleIcon, DockerIcon, TuxIcon, WindowsIcon } from "./ui/icons"

export function AddSystemButton({ className }: { className?: string }) {
	const [open, setOpen] = useState(false)
	let opened = useRef(false)
	if (open) {
		opened.current = true
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
			{opened.current && <SystemDialog setOpen={setOpen} />}
		</Dialog>
	)
}

function copyDockerCompose(port = "45876", publicKey: string) {
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
      LISTEN: ${port}
      KEY: "${publicKey}"`)
}

function copyDockerRun(port = "45876", publicKey: string) {
	copyToClipboard(
		`docker run -d --name beszel-agent --network host --restart unless-stopped -v /var/run/docker.sock:/var/run/docker.sock:ro -e KEY="${publicKey}" -e LISTEN=${port} henrygd/beszel-agent:latest`
	)
}

function copyLinuxCommand(port = "45876", publicKey: string, brew = false) {
	let cmd = `curl -sL https://get.beszel.dev${
		brew ? "/brew" : ""
	} -o /tmp/install-agent.sh && chmod +x /tmp/install-agent.sh && /tmp/install-agent.sh -p ${port} -k "${publicKey}"`
	// brew script does not support --china-mirrors
	if (!brew && (i18n.locale + navigator.language).includes("zh-CN")) {
		cmd += ` --china-mirrors`
	}
	copyToClipboard(cmd)
}

function copyWindowsCommand(port = "45876", publicKey: string) {
	copyToClipboard(
		`& iwr -useb https://get.beszel.dev -OutFile "$env:TEMP\\install-agent.ps1"; & Powershell -ExecutionPolicy Bypass -File "$env:TEMP\\install-agent.ps1" -Key "${publicKey}" -Port ${port}`
	)
}

/**
 * SystemDialog component for adding or editing a system.
 * @param {Object} props - The component props.
 * @param {function} props.setOpen - Function to set the open state of the dialog.
 * @param {SystemRecord} [props.system] - Optional system record for editing an existing system.
 */
export const SystemDialog = memo(({ setOpen, system }: { setOpen: (open: boolean) => void; system?: SystemRecord }) => {
	const publicKey = useStore($publicKey)
	const port = useRef<HTMLInputElement>(null)
	const [hostValue, setHostValue] = useState(system?.host ?? "")
	const isUnixSocket = hostValue.startsWith("/")
	const [tab, setTab] = useLocalStorage("as-tab", "docker")

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault()
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Record<string, any>
		data.users = pb.authStore.record!.id
		try {
			setOpen(false)
			if (system) {
				await pb.collection("systems").update(system.id, { ...data, status: "pending" })
			} else {
				await pb.collection("systems").create(data)
			}
			navigate(basePath)
			// console.log(record)
		} catch (e) {
			console.log(e)
		}
	}

	return (
		<DialogContent
			className="w-[90%] sm:w-auto sm:ns-dialog max-w-full rounded-lg"
			onCloseAutoFocus={() => {
				setHostValue(system?.host ?? "")
			}}
		>
			<Tabs defaultValue={tab} onValueChange={setTab}>
				<DialogHeader>
					<DialogTitle className="mb-2">
						{system ? `${t`Edit`} ${system?.name}` : <Trans>Add New System</Trans>}
					</DialogTitle>
					<TabsList className="grid w-full grid-cols-2">
						<TabsTrigger value="docker">Docker</TabsTrigger>
						<TabsTrigger value="binary">
							<Trans>Binary</Trans>
						</TabsTrigger>
					</TabsList>
				</DialogHeader>
				{/* Docker (set tab index to prevent auto focusing content in edit system dialog) */}
				<TabsContent value="docker" tabIndex={-1}>
					<DialogDescription className="mb-4 leading-normal w-0 min-w-full">
						<Trans>
							The agent must be running on the system to connect. Copy the
							<code className="bg-muted px-1 rounded-sm leading-3">docker-compose.yml</code> for the agent below.
						</Trans>
					</DialogDescription>
				</TabsContent>
				{/* Binary */}
				<TabsContent value="binary" tabIndex={-1}>
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
						<Input id="name" name="name" defaultValue={system?.name} required />
						<Label htmlFor="host" className="xs:text-end">
							<Trans>Host / IP</Trans>
						</Label>
						<Input
							id="host"
							name="host"
							value={hostValue}
							required
							onChange={(e) => {
								setHostValue(e.target.value)
							}}
						/>
						<Label htmlFor="port" className={cn("xs:text-end", isUnixSocket && "hidden")}>
							<Trans>Port</Trans>
						</Label>
						<Input
							ref={port}
							name="port"
							id="port"
							defaultValue={system?.port || "45876"}
							required={!isUnixSocket}
							className={cn(isUnixSocket && "hidden")}
						/>
						<Label htmlFor="pkey" className="xs:text-end whitespace-pre">
							<Trans comment="Use 'Key' if your language requires many more characters">Public Key</Trans>
						</Label>
						<div className="relative">
							<Input readOnly id="pkey" value={publicKey} required></Input>
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
											<Copy className="size-4" />
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
					<DialogFooter className="flex justify-end gap-x-2 gap-y-3 flex-col mt-5">
						{/* Docker */}
						<TabsContent value="docker" className="contents">
							<CopyButton
								text={t({ message: "Copy docker compose", context: "Button to copy docker compose file content" })}
								onClick={() => copyDockerCompose(isUnixSocket ? hostValue : port.current?.value, publicKey)}
								icon={<DockerIcon className="size-4 -me-0.5" />}
								dropdownItems={[
									{
										text: t({ message: "Copy docker run", context: "Button to copy docker run command" }),
										onClick: () => copyDockerRun(isUnixSocket ? hostValue : port.current?.value, publicKey),
										icons: [<DockerIcon className="size-4" />],
									},
								]}
							/>
						</TabsContent>
						{/* Binary */}
						<TabsContent value="binary" className="contents">
							<CopyButton
								text={t`Copy Linux command`}
								icon={<TuxIcon className="size-4" />}
								onClick={() => copyLinuxCommand(isUnixSocket ? hostValue : port.current?.value, publicKey)}
								dropdownItems={[
									{
										text: t({ message: "Homebrew command", context: "Button to copy install command" }),
										onClick: () => copyLinuxCommand(isUnixSocket ? hostValue : port.current?.value, publicKey, true),
										icons: [<AppleIcon className="size-4" />, <TuxIcon className="w-4 h-4" />],
									},
									{
										text: t({ message: "Windows command", context: "Button to copy install command" }),
										onClick: () => copyWindowsCommand(isUnixSocket ? hostValue : port.current?.value, publicKey),
										icons: [<WindowsIcon className="size-4" />],
									},
									{
										text: t`Manual setup instructions`,
										url: "https://beszel.dev/guide/agent-installation#binary",
										icons: [<ExternalLinkIcon className="size-4" />],
									},
								]}
							/>
						</TabsContent>
						{/* Save */}
						<Button>{system ? <Trans>Save system</Trans> : <Trans>Add system</Trans>}</Button>
					</DialogFooter>
				</form>
			</Tabs>
		</DialogContent>
	)
})

interface DropdownItem {
	text: string
	onClick?: () => void
	url?: string
	icons?: React.ReactNode[]
}

interface CopyButtonProps {
	text: string
	onClick: () => void
	dropdownItems: DropdownItem[]
	icon?: React.ReactNode
}

const CopyButton = memo((props: CopyButtonProps) => {
	return (
		<div className="flex gap-0 rounded-lg">
			<Button
				type="button"
				variant="outline"
				onClick={props.onClick}
				className="rounded-e-none dark:border-e-0 grow flex items-center gap-2"
			>
				{props.text} {props.icon}
			</Button>
			<div className="w-px h-full bg-muted"></div>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="outline" className={"px-2 rounded-s-none border-s-0"}>
						<ChevronDownIcon />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					{props.dropdownItems.map((item, index) => {
						const className = "cursor-pointer flex items-center gap-1.5"
						return item.url ? (
							<DropdownMenuItem key={index} asChild>
								<a href={item.url} className={className} target="_blank" rel="noopener noreferrer">
									{item.text} {item.icons?.map((icon) => icon)}
								</a>
							</DropdownMenuItem>
						) : (
							<DropdownMenuItem key={index} onClick={item.onClick} className={className}>
								{item.text} {item.icons?.map((icon) => icon)}
							</DropdownMenuItem>
						)
					})}
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	)
})
