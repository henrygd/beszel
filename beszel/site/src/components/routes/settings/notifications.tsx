import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { pb } from "@/lib/stores"
import { Separator } from "@/components/ui/separator"
import { Card } from "@/components/ui/card"
import { BellIcon, LoaderCircleIcon, PlusIcon, SaveIcon, Trash2Icon } from "lucide-react"
import { ChangeEventHandler, useEffect, useState } from "react"
import { toast } from "@/components/ui/use-toast"
import { InputTags } from "@/components/ui/input-tags"
import { UserSettings } from "@/types"
import { saveSettings } from "./layout"
import * as v from "valibot"
import { isAdmin } from "@/lib/utils"
import { Trans, t } from "@lingui/macro"
import { prependBasePath } from "@/components/router"

interface ShoutrrrUrlCardProps {
	url: string
	onUrlChange: ChangeEventHandler<HTMLInputElement>
	onRemove: () => void
}

const NotificationSchema = v.object({
	emails: v.array(v.pipe(v.string(), v.email())),
	webhooks: v.array(v.pipe(v.string(), v.url())),
})

const SettingsNotificationsPage = ({ userSettings }: { userSettings: UserSettings }) => {
	const [webhooks, setWebhooks] = useState(userSettings.webhooks ?? [])
	const [emails, setEmails] = useState<string[]>(userSettings.emails ?? [])
	const [isLoading, setIsLoading] = useState(false)

	// update values when userSettings changes
	useEffect(() => {
		setWebhooks(userSettings.webhooks ?? [])
		setEmails(userSettings.emails ?? [])
	}, [userSettings])

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
			await saveSettings(parsedData)
		} catch (e: any) {
			toast({
				title: t`Failed to save settings`,
				description: e.message,
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
				<p className="text-sm text-muted-foreground mt-1.5 leading-relaxed">
					<Trans>
						Looking instead for where to create alerts? Click the bell <BellIcon className="inline h-4 w-4" /> icons in
						the systems table.
					</Trans>
				</p>
			</div>
			<Separator className="my-4" />
			<div className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
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
					<div>
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Webhook / Push notifications</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>
								Beszel uses{" "}
								<a href="https://containrrr.dev/shoutrrr/services/overview/" target="_blank" className="link">
									Shoutrrr
								</a>{" "}
								to integrate with popular notification services.
							</Trans>
						</p>
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
					<Button
						type="button"
						variant="outline"
						size="sm"
						className="mt-2 flex items-center gap-1"
						onClick={addWebhook}
					>
						<PlusIcon className="h-4 w-4 -ms-0.5" />
						<Trans>Add URL</Trans>
					</Button>
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

const ShoutrrrUrlCard = ({ url, onUrlChange, onRemove }: ShoutrrrUrlCardProps) => {
	const [isLoading, setIsLoading] = useState(false)

	const sendTestNotification = async () => {
		setIsLoading(true)
		const res = await pb.send("/api/beszel/send-test-notification", { url })
		if ("err" in res && !res.err) {
			toast({
				title: t`Test notification sent`,
				description: t`Check your notification service`,
			})
		} else {
			toast({
				title: t`Error`,
				description: res.err ?? t`Failed to send test notification`,
				variant: "destructive",
			})
		}
		setIsLoading(false)
	}

	return (
		<Card className="bg-muted/40 p-2 md:p-3">
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
