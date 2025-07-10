import { Trans, useLingui } from "@lingui/react/macro"
import { t } from "@lingui/core/macro"
import { $publicKey, pb } from "@/lib/stores"
import { memo, useEffect, useMemo, useState } from "react"
import { Table, TableCell, TableHead, TableBody, TableRow, TableHeader } from "@/components/ui/table"
import { FingerprintRecord } from "@/types"
import {
	CopyIcon,
	FingerprintIcon,
	KeyIcon,
	MoreHorizontalIcon,
	RotateCwIcon,
	ServerIcon,
	Trash2Icon,
} from "lucide-react"
import { toast } from "@/components/ui/use-toast"
import { cn, copyToClipboard, generateToken, getHubURL, isReadOnlyUser, tokenMap } from "@/lib/utils"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import {
	copyDockerCompose,
	copyDockerRun,
	copyLinuxCommand,
	copyWindowsCommand,
	DropdownItem,
	InstallDropdown,
} from "@/components/install-dropdowns"
import { AppleIcon, DockerIcon, TuxIcon, WindowsIcon } from "@/components/ui/icons"

const pbFingerprintOptions = {
	expand: "system",
	fields: "id,fingerprint,token,system,expand.system.name",
}

const SettingsFingerprintsPage = memo(() => {
	const [fingerprints, setFingerprints] = useState<FingerprintRecord[]>([])

	// Get fingerprint records on mount
	useEffect(() => {
		pb.collection("fingerprints")
			.getFullList(pbFingerprintOptions)
			// @ts-ignore
			.then(setFingerprints)
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
							return [...currentFingerprints, res.record as FingerprintRecord]
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
			<div className="min-h-16 overflow-auto max-w-full inline-flex items-center gap-5 mt-3 border py-2 pl-5 pr-4 rounded-md">
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
					<TableRow>
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
					</TableRow>
				</TableHeader>
				<TableBody className="whitespace-pre">
					{fingerprints.map((fingerprint, i) => (
						<TableRow key={i}>
							<TableCell className="font-medium ps-5 py-2.5">{fingerprint.expand.system.name}</TableCell>
							<TableCell className="font-mono text-[0.95em] py-2.5">{fingerprint.token}</TableCell>
							<TableCell className="font-mono text-[0.95em] py-2.5">{fingerprint.fingerprint}</TableCell>
							{!isReadOnly && (
								<TableCell className="py-2.5 px-4 xl:px-2">
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
	} catch (error: any) {
		toast({
			title: t`Error`,
			description: error.message,
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
