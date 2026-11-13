import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { KeyRoundIcon, LoaderCircleIcon, ShieldXIcon } from "lucide-react"
import { useState } from "react"
import { TokenValue } from "@/components/token-value"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button, buttonVariants } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { toast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import { cn, getHubURL } from "@/lib/utils"

const DAY = 24 * 60 * 60
const NEVER = "never"

function showError(error: unknown) {
	toast({
		title: t`Error`,
		description: (error as Error).message,
		variant: "destructive",
	})
}

export default function ApiKeysSettings() {
	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>API Keys</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						API keys let scripts and integrations use the Beszel API as your user. A key has the same access as your
						account and works until it expires or is revoked.
					</Trans>
				</p>
			</div>
			<Separator className="my-4" />
			<SectionCreate />
			<Separator className="my-5" />
			<SectionRevoke />
		</div>
	)
}

function SectionCreate() {
	const { t } = useLingui()
	const [duration, setDuration] = useState(String(30 * DAY))
	const [isLoading, setIsLoading] = useState(false)
	const [created, setCreated] = useState<{ token: string; expires: string } | null>(null)

	const durations = [
		{ value: DAY, label: t`1 day` },
		{ value: 7 * DAY, label: t`7 days` },
		{ value: 30 * DAY, label: t`30 days` },
		{ value: 90 * DAY, label: t`90 days` },
		{ value: 365 * DAY, label: t`1 year` },
		{ value: NEVER, label: t`Never` },
	]

	async function createKey() {
		setIsLoading(true)
		try {
			const data = await pb.send<{ token: string; expires: string }>("/api/beszel/api-keys", {
				method: "POST",
				body: duration === NEVER ? { noExpiry: true } : { duration: Number(duration) },
			})
			setCreated(data)
		} catch (error) {
			showError(error)
		} finally {
			setIsLoading(false)
		}
	}

	const expires = created?.expires
		? new Date(created.expires).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })
		: ""

	return (
		<div>
			<h4 className="text-lg font-medium mb-2">
				<Trans>Create API key</Trans>
			</h4>
			<div className="flex flex-wrap items-end gap-3">
				<div className="grid gap-2">
					<Label htmlFor="api-key-duration">
						<Trans>Expires after</Trans>
					</Label>
					<Select value={duration} onValueChange={setDuration}>
						<SelectTrigger id="api-key-duration" className="w-44">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{durations.map(({ value, label }) => (
								<SelectItem key={value} value={String(value)}>
									{label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				<Button type="button" className="flex items-center gap-1" onClick={createKey} disabled={isLoading}>
					{isLoading ? (
						<LoaderCircleIcon className="size-4 me-0.5 animate-spin" />
					) : (
						<KeyRoundIcon className="size-4 me-0.5" />
					)}
					<Trans>Create API key</Trans>
				</Button>
			</div>
			{created && (
				<div className="mt-4 border rounded-md px-4 py-3 max-w-full grid gap-2">
					<p className="text-sm font-medium">
						<Trans>Copy this key now. It won't be shown again.</Trans>
					</p>
					<TokenValue key={created.token} token={created.token} label={t`API key`} />
					{expires && (
						<p className="text-sm text-muted-foreground">
							<Trans>Expires {expires}</Trans>
						</p>
					)}
					<p className="text-sm text-muted-foreground leading-relaxed">
						<Trans>
							Send the key in the <code className="bg-muted rounded-sm px-1 text-primary">Authorization</code> header of
							each request:
						</Trans>
					</p>
					<code
						dir="ltr"
						className="block bg-muted rounded-sm px-2 py-1.5 text-xs font-mono overflow-x-auto whitespace-nowrap"
					>
						curl -H "Authorization: $BESZEL_API_KEY" {getHubURL()}/api/collections/systems/records
					</code>
				</div>
			)}
		</div>
	)
}

function SectionRevoke() {
	const [open, setOpen] = useState(false)
	const [isLoading, setIsLoading] = useState(false)

	async function revokeKeys() {
		setIsLoading(true)
		try {
			const { token } = await pb.send<{ token: string }>("/api/beszel/api-keys/revoke", { method: "POST" })
			// the old session token is now invalid, so keep this tab logged in with the new one
			pb.authStore.save(token, pb.authStore.record)
			// reload so realtime subscriptions reconnect with the new token
			window.location.reload()
		} catch (error) {
			showError(error)
			setIsLoading(false)
		}
	}

	return (
		<div>
			<h4 className="text-lg font-medium mb-2">
				<Trans>Revoke API keys</Trans>
			</h4>
			<p className="text-sm text-muted-foreground leading-relaxed">
				<Trans>
					Revoking invalidates all of your API keys and signs you out of other devices. Changing your password has the
					same effect.
				</Trans>
			</p>
			<Button
				type="button"
				variant="destructive"
				className="mt-3 flex items-center gap-1"
				onClick={() => setOpen(true)}
				disabled={isLoading}
			>
				{isLoading ? (
					<LoaderCircleIcon className="size-4 me-0.5 animate-spin" />
				) : (
					<ShieldXIcon className="size-4 me-0.5" />
				)}
				<Trans>Revoke all API keys</Trans>
			</Button>
			<AlertDialog open={open} onOpenChange={setOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Revoke all API keys?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>
								Every API key you have created will stop working immediately, and you will be signed out of other
								devices.
							</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction className={cn(buttonVariants({ variant: "destructive" }))} onClick={revokeKeys}>
							<Trans>Revoke</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	)
}
