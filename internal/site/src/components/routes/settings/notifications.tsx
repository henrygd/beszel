import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ChevronDownIcon, LoaderCircleIcon, PlusIcon, SaveIcon, Trash2Icon } from "lucide-react"
import { type ChangeEventHandler, useEffect, useState } from "react"
import * as v from "valibot"
import { prependBasePath } from "@/components/router"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { InputTags } from "@/components/ui/input-tags"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { toast } from "@/components/ui/use-toast"
import { getPagePath } from "@nanostores/router"
import { $router, Link } from "@/components/router"
import { isAdmin, pb } from "@/lib/api"
import { $systems } from "@/lib/stores"
import type { UserSettings } from "@/types"
import { saveSettings } from "./layout"
import { QuietHours } from "./quiet-hours"
import type { ClientResponseError } from "pocketbase"

interface ShoutrrrUrlCardProps {
	url: string
	onUrlChange: ChangeEventHandler<HTMLInputElement>
	onRemove: () => void
}

const NotificationSchema = v.object({
	emails: v.array(v.pipe(v.string(), v.rfcEmail())),
	webhooks: v.array(v.pipe(v.string(), v.url())),
})

const SettingsNotificationsPage = ({ userSettings }: { userSettings: UserSettings }) => {
	const systems = useStore($systems)
	const [notificationsEnabled, setNotificationsEnabled] = useState(userSettings.notificationsEnabled ?? false)
	const [subscribedSystems, setSubscribedSystems] = useState<string[]>(userSettings.systems ?? [])
	const [webhooks, setWebhooks] = useState(userSettings.webhooks ?? [])
	const [emails, setEmails] = useState<string[]>(userSettings.emails ?? [])
	const [isLoading, setIsLoading] = useState(false)

	// update values when userSettings changes
	useEffect(() => {
		setNotificationsEnabled(userSettings.notificationsEnabled ?? false)
		setSubscribedSystems(userSettings.systems ?? [])
		setWebhooks(userSettings.webhooks ?? [])
		setEmails(userSettings.emails ?? [])
	}, [userSettings])

	function toggleSystem(systemId: string) {
		setSubscribedSystems((prev) =>
			prev.includes(systemId) ? prev.filter((id) => id !== systemId) : [...prev, systemId]
		)
	}

	function addWebhook() {
		setWebhooks([...webhooks, ""])
		// focus on the new input
		queueMicrotask(() => {
			const inputs = document.querySelectorAll("#webhooks input") as NodeListOf<HTMLInputElement>
			inputs[inputs.length - 1]?.focus()
		})
	}
	const removeWebhook = (index: number) => setWebhooks(webhooks.filter((_, i) => i !== index))

	function updateWebhook(index: number, value: string) {
		const newWebhooks = [...webhooks]
		newWebhooks[index] = value
		setWebhooks(newWebhooks)
	}

	async function updateSettings() {
		setIsLoading(true)
		try {
			const parsedData = v.parse(NotificationSchema, { emails, webhooks })
			await saveSettings({ ...parsedData, notificationsEnabled, systems: subscribedSystems })
		} catch (e: unknown) {
			toast({
				title: t`Failed to save settings`,
				description: (e as Error).message,
				variant: "destructive",
			})
		}
		setIsLoading(false)
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>Notifications</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>Configure how you receive alert notifications.</Trans>
				</p>
				{isAdmin() && (
					<p className="text-sm text-muted-foreground mt-1.5 leading-relaxed">
						<Trans>
							Alerts are configured in{" "}
							<Link href={getPagePath($router, "settings", { name: "global-alerts" })} className="link">
								Global Alerts
							</Link>
							.
						</Trans>
					</p>
				)}
			</div>
			<Separator className="my-4" />
			<div className="space-y-5">
				<label htmlFor="notif-enabled" className="flex items-center justify-between gap-4 cursor-pointer">
					<div>
						<p className="font-medium mb-0.5">
							<Trans>Receive alert notifications</Trans>
						</p>
						<p className="text-sm text-muted-foreground">
							<Trans>Enable to receive notifications when alerts are triggered.</Trans>
						</p>
					</div>
					<Switch
						id="notif-enabled"
						checked={notificationsEnabled}
						onCheckedChange={setNotificationsEnabled}
					/>
				</label>
				{notificationsEnabled && systems.length > 0 && (
					<div>
						<p className="font-medium mb-1">
							<Trans>Systems</Trans>
						</p>
						<p className="text-sm text-muted-foreground mb-2">
							{subscribedSystems.length === 0 ? (
								<Trans>Receiving notifications for all systems.</Trans>
							) : (
								<Trans>Receiving notifications for {subscribedSystems.length} system(s).</Trans>
							)}
						</p>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline" size="sm" className="text-xs gap-1.5 h-8">
									{subscribedSystems.length === 0 ? (
										<Trans>All systems</Trans>
									) : (
										<Trans>{subscribedSystems.length} selected</Trans>
									)}
									<ChevronDownIcon className="h-3.5 w-3.5" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="start" className="max-h-64 overflow-auto min-w-44">
								{systems.map((system) => (
									<DropdownMenuCheckboxItem
										key={system.id}
										checked={subscribedSystems.includes(system.id)}
										onCheckedChange={() => toggleSystem(system.id)}
										onSelect={(e) => e.preventDefault()}
									>
										{system.name}
									</DropdownMenuCheckboxItem>
								))}
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				)}
				<Separator />
				<div className="grid gap-2">
					<div className="mb-2">
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Email notifications</Trans>
						</h3>
						{isAdmin() && (
							<p className="text-sm text-muted-foreground leading-relaxed">
								<Trans>
									Please{" "}
									<a href={prependBasePath("/_/#/settings/mail")} className="link" target="_blank">
										configure an SMTP server
									</a>{" "}
									to ensure alerts are delivered.
								</Trans>
							</p>
						)}
					</div>
					<Label className="block" htmlFor="email">
						<Trans>To email(s)</Trans>
					</Label>
					<InputTags
						value={emails}
						onChange={setEmails}
						placeholder={t`Enter email address...`}
						className="w-full"
						type="email"
						id="email"
					/>
					<p className="text-[0.8rem] text-muted-foreground">
						<Trans>Save address using enter key or comma. Leave blank to disable email notifications.</Trans>
					</p>
				</div>
				<Separator />
				<div className="space-y-3">
					<div className="grid grid-cols-1 sm:flex items-center justify-between gap-4">
						<div>
							<h3 className="mb-1 text-lg font-medium">
								<Trans>Webhook / Push notifications</Trans>
							</h3>
							<p className="text-sm text-muted-foreground leading-relaxed">
								<Trans>
									Beszel uses{" "}
									<a href="https://beszel.dev/guide/notifications" target="_blank" className="link" rel="noopener">
										Shoutrrr
									</a>{" "}
									to integrate with popular notification services.
								</Trans>
							</p>
						</div>
						<Button type="button" variant="outline" className="h-10 shrink-0" onClick={addWebhook}>
							<PlusIcon className="size-4" />
							<span className="ms-1">
								<Trans>Add URL</Trans>
							</span>
						</Button>
					</div>
					{webhooks.length > 0 && (
						<div className="grid gap-2.5" id="webhooks">
							{webhooks.map((webhook, index) => (
								<ShoutrrrUrlCard
									key={index}
									url={webhook}
									onUrlChange={(e: React.ChangeEvent<HTMLInputElement>) => updateWebhook(index, e.target.value)}
									onRemove={() => removeWebhook(index)}
								/>
							))}
						</div>
					)}
				</div>
				<Separator />
				<div className="space-y-3">
					<QuietHours />
				</div>
				<Separator />
				<Button
					type="button"
					className="flex items-center gap-1.5 disabled:opacity-100"
					onClick={updateSettings}
					disabled={isLoading}
				>
					{isLoading ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : <SaveIcon className="h-4 w-4" />}
					<Trans>Save Settings</Trans>
				</Button>
			</div>
		</div>
	)
}

function showTestNotificationError(msg: string) {
	toast({
		title: t`Error`,
		description: msg ?? t`Failed to send test notification`,
		variant: "destructive",
	})
}

const ShoutrrrUrlCard = ({ url, onUrlChange, onRemove }: ShoutrrrUrlCardProps) => {
	const [isLoading, setIsLoading] = useState(false)

	const sendTestNotification = async () => {
		setIsLoading(true)
		try {
			const res = await pb.send("/api/beszel/test-notification", { method: "POST", body: { url } })
			if ("err" in res && !res.err) {
				toast({
					title: t`Test notification sent`,
					description: t`Check your notification service`,
				})
			} else {
				showTestNotificationError(res.err)
			}
		} catch (e: unknown) {
			showTestNotificationError((e as ClientResponseError).data?.message)
		} finally {
			setIsLoading(false)
		}
	}

	return (
		<Card className="bg-table-header p-2 md:p-3">
			<div className="flex items-center gap-1">
				<Input
					type="url"
					className="light:bg-card"
					required
					placeholder="generic://webhook.site/xxxxxx"
					value={url}
					onChange={onUrlChange}
				/>
				<Button type="button" variant="outline" disabled={isLoading || url === ""} onClick={sendTestNotification}>
					{isLoading ? (
						<LoaderCircleIcon className="h-4 w-4 animate-spin" />
					) : (
						<span>
							<Trans>
								Test <span className="hidden sm:inline">URL</span>
							</Trans>
						</span>
					)}
				</Button>
				<Button type="button" variant="outline" size="icon" className="shrink-0" aria-label="Delete" onClick={onRemove}>
					<Trash2Icon className="h-4 w-4" />
				</Button>
			</div>
		</Card>
	)
}

export default SettingsNotificationsPage
