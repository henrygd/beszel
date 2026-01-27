import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { ChevronDownIcon, ExternalLinkIcon, PlusIcon, XIcon } from "lucide-react"
import { memo, useEffect, useRef, useState } from "react"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { isReadOnlyUser, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import { $publicKey } from "@/lib/stores"
import { cn, generateToken, tokenMap, useBrowserStorage } from "@/lib/utils"
import type { SystemRecord, TagRecord } from "@/types"
import { Badge } from "./ui/badge"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "./ui/dropdown-menu"
import {
	copyDockerCompose,
	copyDockerRun,
	copyLinuxCommand,
	copyWindowsCommand,
	type DropdownItem,
	InstallDropdown,
} from "./install-dropdowns"
import { $router, basePath, Link, navigate } from "./router"
import { AppleIcon, DockerIcon, FreeBsdIcon, TuxIcon, WindowsIcon } from "./ui/icons"
import { InputCopy } from "./ui/input-copy"
import { toast } from "./ui/use-toast"

// Generate a random vibrant color for new tags
function getRandomColor(): string {
	const colors = [
		"#ef4444", "#f97316", "#f59e0b", "#eab308", "#84cc16", "#22c55e",
		"#10b981", "#14b8a6", "#06b6d4", "#0ea5e9", "#3b82f6", "#6366f1",
		"#8b5cf6", "#a855f7", "#d946ef", "#ec4899", "#f43f5e",
	]
	return colors[Math.floor(Math.random() * colors.length)]
}

export function AddSystemButton({ className }: { className?: string }) {
		if (isReadOnlyUser()) {
		return null
	}
	const [open, setOpen] = useState(false)
	const opened = useRef(false)
	if (open) {
		opened.current = true
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button variant="outline" className={cn("flex gap-1 max-xs:h-[2.4rem]", className)}>
					<PlusIcon className="h-4 w-4 450:-ms-1" />
					<span className="hidden 450:inline">
						<Trans>
							Add <span className="hidden sm:inline">System</span>
						</Trans>
					</span>
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
	const [availableTags, setAvailableTags] = useState<TagRecord[]>([])
	const [selectedTags, setSelectedTags] = useState<string[]>(system?.tags ?? [])
	const [tagSearchQuery, setTagSearchQuery] = useState("")

	useEffect(() => {
		;(async () => {
			// Load available tags
			try {
				const tags = await pb.collection("tags").getFullList<TagRecord>({
					sort: "name",
				})
				setAvailableTags(tags)
			} catch (e) {
				console.error("Failed to load tags", e)
			}

			// if no system, generate a new token
			if (!system) {
				nextSystemToken ||= generateToken()
				setToken(nextSystemToken)
				return
			}
			// if system exists,get the token from the fingerprint record
			if (tokenMap.has(system.id)) {
				setToken(tokenMap.get(system.id)!)
				return
			}
			try {
				const { token } = await pb.collection("fingerprints").getFirstListItem(`system = "${system.id}"`, {
					fields: "token",
				})
				tokenMap.set(system.id, token)
				setToken(token)
			} catch (e) {
				console.error("Failed to load fingerprint", e)
			}
		})()
	}, [system?.id, nextSystemToken])

	async function createTagFromSearch() {
		const name = tagSearchQuery.trim()
		if (!name) return
		// Check if tag with this name already exists
		if (availableTags.some((tag) => tag.name.toLowerCase() === name.toLowerCase())) return
		try {
			const record = await pb.collection("tags").create<TagRecord>({
				name,
				color: getRandomColor(),
			})
			setAvailableTags((prev) => [...prev, record].sort((a, b) => a.name.localeCompare(b.name)))
			setSelectedTags((prev) => [...prev, record.id])
			setTagSearchQuery("")
		} catch (e: any) {
			console.error("Failed to create tag", e)
			toast({
				title: t`Failed to create tag`,
				description: e.message || t`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault()
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Record<string, any>
		data.users = pb.authStore.record!.id
		data.tags = selectedTags
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
	
	const systemTranslation = t`System`
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
						{system ? (
							<Trans>Edit {{ foo: systemTranslation }}</Trans>
						) : (
							<Trans>Add {{ foo: systemTranslation }}</Trans>
						)}
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
						<Label htmlFor="tags" className="xs:text-end self-start pt-2">
							<Trans>Tags</Trans>
						</Label>
						<div className="flex flex-col gap-2">
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" className="justify-between font-normal">
										{selectedTags.length > 0 ? (
											<span className="truncate">
												{selectedTags.length === 1
													? availableTags.find((t) => t.id === selectedTags[0])?.name
													: t`${selectedTags.length} tags selected`}
											</span>
										) : (
											<span className="text-muted-foreground">
												<Trans>Select tags...</Trans>
											</span>
										)}
										<ChevronDownIcon className="ml-2 h-4 w-4 shrink-0 opacity-50" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent className="w-80" align="start">
									<div className="px-2 py-1.5">
										<Input
											placeholder={t`Search or create tag...`}
											value={tagSearchQuery}
											onChange={(e) => setTagSearchQuery(e.target.value)}
											className="h-8"
											onKeyDown={(e) => {
												if (e.key === "Enter") {
													e.preventDefault()
													createTagFromSearch()
												}
											}}
										/>
									</div>
									<DropdownMenuSeparator />
									<div className="max-h-60 overflow-y-auto">
										{availableTags
											.filter((tag) =>
												tag.name.toLowerCase().includes(tagSearchQuery.toLowerCase())
											)
											.map((tag) => {
												const isSelected = selectedTags.includes(tag.id)
												return (
													<DropdownMenuCheckboxItem
														key={tag.id}
														checked={isSelected}
														onCheckedChange={(checked) => {
															setSelectedTags((prev) =>
																checked ? [...prev, tag.id] : prev.filter((id) => id !== tag.id)
															)
														}}
														onSelect={(e) => e.preventDefault()}
													>
														<Badge
															style={{ backgroundColor: tag.color || "#3b82f6" }}
															className="text-white text-xs"
														>
															{tag.name}
														</Badge>
													</DropdownMenuCheckboxItem>
												)
											})}
										{tagSearchQuery &&
											!availableTags.some(
												(tag) => tag.name.toLowerCase() === tagSearchQuery.toLowerCase()
											) && (
												<div className="py-3 px-2 text-center text-sm text-muted-foreground">
													<Trans>
														Press Enter to create "{tagSearchQuery}"
													</Trans>
												</div>
											)}
										{availableTags.length === 0 && !tagSearchQuery && (
											<div className="py-4 text-center text-sm text-muted-foreground">
												<Trans>Type a name and press Enter to create a tag.</Trans>
											</div>
										)}
										{availableTags.length > 0 &&
											!tagSearchQuery &&
											availableTags.filter((tag) =>
												tag.name.toLowerCase().includes(tagSearchQuery.toLowerCase())
											).length === 0 && (
												<div className="py-4 text-center text-sm text-muted-foreground">
													<Trans>No tags found.</Trans>
												</div>
											)}
									</div>
								</DropdownMenuContent>
							</DropdownMenu>
							{selectedTags.length > 0 && (
								<div className="flex flex-wrap gap-1.5">
									{selectedTags.map((tagId) => {
										const tag = availableTags.find((t) => t.id === tagId)
										if (!tag) return null
										return (
											<Badge
												key={tag.id}
												style={{ backgroundColor: tag.color || "#3b82f6" }}
												className="text-white text-xs"
											>
												{tag.name}
												<button
													type="button"
													className="ml-1 hover:bg-white/20 rounded-full"
													onClick={(e) => {
														e.stopPropagation()
														setSelectedTags((prev) => prev.filter((id) => id !== tagId))
													}}
												>
													<XIcon className="h-3 w-3" />
												</button>
											</Badge>
										)
									})}
								</div>
							)}
						</div>
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
