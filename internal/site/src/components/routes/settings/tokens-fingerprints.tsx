import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { redirectPage } from "@nanostores/router"
import {
	CopyIcon,
	FingerprintIcon,
	KeyIcon,
	MoreHorizontalIcon,
	RotateCwIcon,
	ServerIcon,
	Trash2Icon,
	ExternalLinkIcon,
} from "lucide-react"
import { memo, useEffect, useMemo, useState } from "react"
import {
	copyDockerCompose,
	copyDockerRun,
	copyLinuxCommand,
	copyWindowsCommand,
	type DropdownItem,
	InstallDropdown,
} from "@/components/install-dropdowns"
import { $router } from "@/components/router"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { AppleIcon, DockerIcon, FreeBsdIcon, TuxIcon, WindowsIcon } from "@/components/ui/icons"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"
import { isReadOnlyUser, pb } from "@/lib/api"
import { $publicKey } from "@/lib/stores"
import { cn, copyToClipboard, generateToken, getHubURL, tokenMap } from "@/lib/utils"
import type { FingerprintRecord } from "@/types"

const pbFingerprintOptions = {
	expand: "system",
	fields: "id,fingerprint,token,system,expand.system.name",
}

function sortFingerprints(fingerprints: FingerprintRecord[]) {
	return fingerprints.sort((a, b) => a.expand.system.name.localeCompare(b.expand.system.name))
}

const SettingsFingerprintsPage = memo(() => {
	if (isReadOnlyUser()) {
		redirectPage($router, "settings", { name: "general" })
	}
	const [fingerprints, setFingerprints] = useState<FingerprintRecord[]>([])

	// Get fingerprint records on mount
	useEffect(() => {
		pb.collection("fingerprints")
			.getFullList<FingerprintRecord>(pbFingerprintOptions)
			.then((prints) => {
				setFingerprints(sortFingerprints(prints))
			})
	}, [])

	// Subscribe to fingerprint updates
	useEffect(() => {
		let unsubscribe: (() => void) | undefined
		;(async () => {
			// subscribe to fingerprint updates
			unsubscribe = await pb.collection("fingerprints").subscribe(
				"*",
				(res) => {
					setFingerprints((currentFingerprints) => {
						if (res.action === "create") {
							return sortFingerprints([...currentFingerprints, res.record as FingerprintRecord])
						}
						if (res.action === "update") {
							return currentFingerprints.map((fingerprint) => {
								if (fingerprint.id === res.record.id) {
									return { ...fingerprint, ...res.record } as FingerprintRecord
								}
								return fingerprint
							})
						}
						if (res.action === "delete") {
							return currentFingerprints.filter((fingerprint) => fingerprint.id !== res.record.id)
						}
						return currentFingerprints
					})
				},
				pbFingerprintOptions
			)
		})()
		// unsubscribe on unmount
		return () => unsubscribe?.()
	}, [])

	// Update token map whenever fingerprints change
	useEffect(() => {
		for (const fingerprint of fingerprints) {
			tokenMap.set(fingerprint.system, fingerprint.token)
		}
	}, [fingerprints])

	return (
		<>
			<SectionIntro />
			<Separator className="my-4" />
			<SectionUniversalToken />
			<Separator className="my-4" />
			<SectionTable fingerprints={fingerprints} />
		</>
	)
})

const SectionIntro = memo(() => {
	return (
		<div>
			<h3 className="text-xl font-medium mb-2">
				<Trans>Tokens & Fingerprints</Trans>
			</h3>
			<p className="text-sm text-muted-foreground leading-relaxed">
				<Trans>Tokens and fingerprints are used to authenticate WebSocket connections to the hub.</Trans>
			</p>
			<p className="text-sm text-muted-foreground leading-relaxed mt-1.5">
				<Trans>
					Tokens allow agents to connect and register. Fingerprints are stable identifiers unique to each system, set on
					first connection.
				</Trans>
			</p>
		</div>
	)
})

const SectionUniversalToken = memo(() => {
	const [token, setToken] = useState("")
	const [isLoading, setIsLoading] = useState(true)
	const [checked, setChecked] = useState(false)

	async function updateToken(enable: number = -1) {
		// enable: 0 for disable, 1 for enable, -1 (unset) for get current state
		const data = await pb.send(`/api/beszel/universal-token`, {
			query: {
				token,
				enable,
			},
		})
		setToken(data.token)
		setChecked(data.active)
		setIsLoading(false)
	}

	// biome-ignore lint/correctness/useExhaustiveDependencies: only on mount
	useEffect(() => {
		updateToken()
	}, [])

	return (
		<div>
			<h3 className="text-lg font-medium mb-2">
				<Trans>Universal token</Trans>
			</h3>
			<p className="text-sm text-muted-foreground leading-relaxed">
				<Trans>
					When enabled, this token allows agents to self-register without prior system creation. Expires after one hour
					or on hub restart.
				</Trans>
			</p>
			<div className="min-h-16 overflow-auto max-w-full inline-flex items-center gap-5 mt-3 border py-2 ps-5 pe-4 rounded-md">
				{!isLoading && (
					<>
						<Switch
							defaultChecked={checked}
							onCheckedChange={(checked) => {
								updateToken(checked ? 1 : 0)
							}}
						/>
						<span
							className={cn(
								"text-sm text-primary opacity-60 transition-opacity",
								checked ? "opacity-100" : "select-none"
							)}
						>
							{token}
						</span>
						<ActionsButtonUniversalToken token={token} checked={checked} />
					</>
				)}
			</div>
		</div>
	)
})

const ActionsButtonUniversalToken = memo(({ token, checked }: { token: string; checked: boolean }) => {
	const { t } = useLingui()
	const publicKey = $publicKey.get()
	const port = "45876"

	const dropdownItems: DropdownItem[] = [
		{
			text: t({ message: "Copy docker compose", context: "Button to copy docker compose file content" }),
			onClick: () => copyDockerCompose(port, publicKey, token),
			icons: [DockerIcon],
		},
		{
			text: t({ message: "Copy docker run", context: "Button to copy docker run command" }),
			onClick: () => copyDockerRun(port, publicKey, token),
			icons: [DockerIcon],
		},
		{
			text: t`Copy Linux command`,
			onClick: () => copyLinuxCommand(port, publicKey, token),
			icons: [TuxIcon],
		},
		{
			text: t({ message: "Homebrew command", context: "Button to copy install command" }),
			onClick: () => copyLinuxCommand(port, publicKey, token, true),
			icons: [TuxIcon, AppleIcon],
		},
		{
			text: t({ message: "Windows command", context: "Button to copy install command" }),
			onClick: () => copyWindowsCommand(port, publicKey, token),
			icons: [WindowsIcon],
		},
		{
			text: t({ message: "FreeBSD command", context: "Button to copy install command" }),
			onClick: () => copyLinuxCommand(port, publicKey, token),
			icons: [FreeBsdIcon],
		},
		{
			text: t`Manual setup instructions`,
			url: "https://beszel.dev/guide/agent-installation#binary",
			icons: [ExternalLinkIcon],
		},
	]
	return (
		<div className="flex items-center gap-2">
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						variant="ghost"
						size="icon"
						disabled={!checked}
						className={cn("transition-opacity", !checked && "opacity-50")}
					>
						<span className="sr-only">
							<Trans>Open menu</Trans>
						</span>
						<MoreHorizontalIcon className="w-5" />
					</Button>
				</DropdownMenuTrigger>
				<InstallDropdown items={dropdownItems} />
			</DropdownMenu>
		</div>
	)
})

const SectionTable = memo(({ fingerprints = [] }: { fingerprints: FingerprintRecord[] }) => {
	const { t } = useLingui()
	const isReadOnly = isReadOnlyUser()

	const headerCols = useMemo(
		() => [
			{
				label: t`System`,
				Icon: ServerIcon,
				w: "11em",
			},
			{
				label: t`Token`,
				Icon: KeyIcon,
				w: "20em",
			},
			{
				label: t`Fingerprint`,
				Icon: FingerprintIcon,
				w: "20em",
			},
		],
		[t]
	)
	return (
		<div className="rounded-md border overflow-hidden w-full mt-4">
			<Table>
				<TableHeader>
					<tr className="border-border/50">
						{headerCols.map((col) => (
							<TableHead key={col.label} style={{ minWidth: col.w }}>
								<span className="flex items-center gap-2">
									<col.Icon className="size-4" />
									{col.label}
								</span>
							</TableHead>
						))}
						{!isReadOnly && (
							<TableHead className="w-0">
								<span className="sr-only">
									<Trans>Actions</Trans>
								</span>
							</TableHead>
						)}
					</tr>
				</TableHeader>
				<TableBody className="whitespace-pre">
					{fingerprints.map((fingerprint) => (
						<TableRow key={fingerprint.id}>
							<TableCell className="font-medium ps-5 py-2 max-w-60 truncate">
								{fingerprint.expand.system.name}
							</TableCell>
							<TableCell className="font-mono text-[0.95em] py-2">{fingerprint.token}</TableCell>
							<TableCell className="font-mono text-[0.95em] py-2">{fingerprint.fingerprint}</TableCell>
							{!isReadOnly && (
								<TableCell className="py-2 px-4 xl:px-2">
									<ActionsButtonTable fingerprint={fingerprint} />
								</TableCell>
							)}
						</TableRow>
					))}
				</TableBody>
			</Table>
		</div>
	)
})

async function updateFingerprint(fingerprint: FingerprintRecord, rotateToken = false) {
	try {
		await pb.collection("fingerprints").update(fingerprint.id, {
			fingerprint: "",
			token: rotateToken ? generateToken() : fingerprint.token,
		})
	} catch (error: unknown) {
		toast({
			title: t`Error`,
			description: (error as Error).message,
		})
	}
}

const ActionsButtonTable = memo(({ fingerprint }: { fingerprint: FingerprintRecord }) => {
	const envVar = `HUB_URL=${getHubURL()}\nTOKEN=${fingerprint.token}`
	const copyEnv = () => copyToClipboard(envVar)
	const copyYaml = () => copyToClipboard(envVar.replaceAll("=", ": "))

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="ghost" size={"icon"} data-nolink>
					<span className="sr-only">
						<Trans>Open menu</Trans>
					</span>
					<MoreHorizontalIcon className="w-5" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem onClick={copyYaml}>
					<CopyIcon className="me-2.5 size-4" />
					<Trans>Copy YAML</Trans>
				</DropdownMenuItem>
				<DropdownMenuItem onClick={copyEnv}>
					<CopyIcon className="me-2.5 size-4" />
					<Trans context="Environment variables">Copy env</Trans>
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				<DropdownMenuItem onSelect={() => updateFingerprint(fingerprint, true)}>
					<RotateCwIcon className="me-2.5 size-4" />
					<Trans>Rotate token</Trans>
				</DropdownMenuItem>
				{fingerprint.fingerprint && (
					<DropdownMenuItem onSelect={() => updateFingerprint(fingerprint)}>
						<Trash2Icon className="me-2.5 size-4" />
						<Trans>Delete fingerprint</Trans>
					</DropdownMenuItem>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	)
})

export default SettingsFingerprintsPage
