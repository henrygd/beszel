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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { $publicKey } from "@/lib/stores"
import { cn, generateToken, tokenMap, useBrowserStorage } from "@/lib/utils"
import { pb, isReadOnlyUser } from "@/lib/api"
import { useStore } from "@nanostores/react"
import { ChevronDownIcon, ExternalLinkIcon, PlusIcon } from "lucide-react"
import { memo, useEffect, useRef, useState } from "react"
import { $router, basePath, Link, navigate } from "./router"
import { SystemRecord } from "@/types"
import { SystemStatus } from "@/lib/enums"
import { AppleIcon, DockerIcon, FreeBsdIcon, TuxIcon, WindowsIcon } from "./ui/icons"
import { InputCopy } from "./ui/input-copy"
import { getPagePath } from "@nanostores/router"
import {
	copyDockerCompose,
	copyDockerRun,
	copyLinuxCommand,
	copyWindowsCommand,
	DropdownItem,
	InstallDropdown,
} from "./install-dropdowns"
import { DropdownMenu, DropdownMenuTrigger } from "./ui/dropdown-menu"

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

/**
 * Token to be used for the next system.
 * Prevents token changing if user copies config, then closes dialog and opens again.
 */
let nextSystemToken: string | null = null

/**
 * SystemDialog component for adding or editing a system.
 * @param {Object} props - The component props.
 * @param {function} props.setOpen - Function to set the open state of the dialog.
 * @param {SystemRecord} [props.system] - Optional system record for editing an existing system.
 */
export const SystemDialog = ({ setOpen, system }: { setOpen: (open: boolean) => void; system?: SystemRecord }) => {
	const publicKey = useStore($publicKey)
	const port = useRef<HTMLInputElement>(null)
	const [hostValue, setHostValue] = useState(system?.host ?? "")
	const isUnixSocket = hostValue.startsWith("/")
	const [tab, setTab] = useBrowserStorage("as-tab", "docker")
	const [token, setToken] = useState(system?.token ?? "")

	useEffect(() => {
		;(async () => {
			// if no system, generate a new token
			if (!system) {
				nextSystemToken ||= generateToken()
				return setToken(nextSystemToken)
			}
			// if system exists,get the token from the fingerprint record
			if (tokenMap.has(system.id)) {
				return setToken(tokenMap.get(system.id)!)
			}
			const { token } = await pb.collection("fingerprints").getFirstListItem(`system = "${system.id}"`, {
				fields: "token",
			})
			tokenMap.set(system.id, token)
			setToken(token)
		})()
	}, [system?.id, nextSystemToken])

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault()
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Record<string, any>
		data.users = pb.authStore.record!.id
		try {
			setOpen(false)
			if (system) {
				await pb.collection("systems").update(system.id, { ...data, status: SystemStatus.Pending })
			} else {
				const createdSystem = await pb.collection("systems").create(data)
				await pb.collection("fingerprints").create({
					system: createdSystem.id,
					token,
				})
				// Reset the current token after successful system
				// creation so next system gets a new token
				nextSystemToken = null
			}
			navigate(basePath)
		} catch (e) {
			console.error(e)
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
					<DialogTitle className="mb-1 pb-1 max-w-100 truncate pr-8">
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
					<DialogDescription className="mb-3 leading-relaxed w-0 min-w-full">
						<Trans>
							Copy the
							<code className="bg-muted px-1 rounded-sm leading-3">docker-compose.yml</code> content for the agent
							below, or register agents automatically with a{" "}
							<Link
								onClick={() => setOpen(false)}
								href={getPagePath($router, "settings", { name: "tokens" })}
								className="link"
							>
								universal token
							</Link>
							.
						</Trans>
					</DialogDescription>
				</TabsContent>
				{/* Binary */}
				<TabsContent value="binary" tabIndex={-1}>
					<DialogDescription className="mb-3 leading-relaxed w-0 min-w-full">
						<Trans>
							Copy the installation command for the agent below, or register agents automatically with a{" "}
							<Link
								onClick={() => setOpen(false)}
								href={getPagePath($router, "settings", { name: "tokens" })}
								className="link"
							>
								universal token
							</Link>
							.
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
						<InputCopy value={publicKey} id="pkey" name="pkey" />
						<Label htmlFor="tkn" className="xs:text-end whitespace-pre">
							<Trans>Token</Trans>
						</Label>
						<InputCopy value={token} id="tkn" name="tkn" />
					</div>
					<DialogFooter className="flex justify-end gap-x-2 gap-y-3 flex-col mt-5">
						{/* Docker */}
						<TabsContent value="docker" className="contents">
							<CopyButton
								text={t({ message: "Copy docker compose", context: "Button to copy docker compose file content" })}
								onClick={async () =>
									copyDockerCompose(isUnixSocket ? hostValue : port.current?.value, publicKey, token)
								}
								icon={<DockerIcon className="size-4 -me-0.5" />}
								dropdownItems={[
									{
										text: t({ message: "Copy docker run", context: "Button to copy docker run command" }),
										onClick: async () =>
											copyDockerRun(isUnixSocket ? hostValue : port.current?.value, publicKey, token),
										icons: [DockerIcon],
									},
								]}
							/>
						</TabsContent>
						{/* Binary */}
						<TabsContent value="binary" className="contents">
							<CopyButton
								text={t`Copy Linux command`}
								icon={<TuxIcon className="size-4" />}
								onClick={async () => copyLinuxCommand(isUnixSocket ? hostValue : port.current?.value, publicKey, token)}
								dropdownItems={[
									{
										text: t({ message: "Homebrew command", context: "Button to copy install command" }),
										onClick: async () =>
											copyLinuxCommand(isUnixSocket ? hostValue : port.current?.value, publicKey, token, true),
										icons: [AppleIcon, TuxIcon],
									},
									{
										text: t({ message: "Windows command", context: "Button to copy install command" }),
										onClick: async () =>
											copyWindowsCommand(isUnixSocket ? hostValue : port.current?.value, publicKey, token),
										icons: [WindowsIcon],
									},
									{
										text: t({ message: "FreeBSD command", context: "Button to copy install command" }),
										onClick: async () =>
											copyLinuxCommand(isUnixSocket ? hostValue : port.current?.value, publicKey, token),
										icons: [FreeBsdIcon],
									},
									{
										text: t`Manual setup instructions`,
										url: "https://beszel.dev/guide/agent-installation#binary",
										icons: [ExternalLinkIcon],
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
}

interface CopyButtonProps {
	text: string
	onClick: () => void
	dropdownItems: DropdownItem[]
	icon?: React.ReactElement<any>
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
				<InstallDropdown items={props.dropdownItems} />
			</DropdownMenu>
		</div>
	)
})
