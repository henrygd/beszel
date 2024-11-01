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
import { useTranslation } from "react-i18next"
import { t } from "i18next"

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
	const { t } = useTranslation()

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
				title: t("settings.failed_to_save"),
				description: e.message,
				variant: "destructive",
			})
		}
		setIsLoading(false)
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">{t("settings.notifications.title")}</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">{t("settings.notifications.subtitle_1")}</p>
				<p className="text-sm text-muted-foreground mt-1.5 leading-relaxed">
					{t("settings.notifications.subtitle_2")} <BellIcon className="inline h-4 w-4" />{" "}
					{t("settings.notifications.subtitle_3")}
				</p>
			</div>
			<Separator className="my-4" />
			<div className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">{t("settings.notifications.email.title")}</h3>
						{isAdmin() && (
							<p className="text-sm text-muted-foreground leading-relaxed">
								{t("settings.notifications.email.please")}{" "}
								<a href="/_/#/settings/mail" className="link" target="_blank">
									{t("settings.notifications.email.configure_an_SMTP_server")}
								</a>{" "}
								{t("settings.notifications.email.to_ensure_alerts_are_delivered")}{" "}
							</p>
						)}
					</div>
					<Label className="block" htmlFor="email">
						{t("settings.notifications.email.to_emails")}
					</Label>
					<InputTags
						value={emails}
						onChange={setEmails}
						placeholder={t("settings.notifications.email.enter_email_address")}
						className="w-full"
						type="email"
						id="email"
					/>
					<p className="text-[0.8rem] text-muted-foreground">{t("settings.notifications.email.des")}</p>
				</div>
				<Separator />
				<div className="space-y-3">
					<div>
						<h3 className="mb-1 text-lg font-medium">{t("settings.notifications.webhook.title")}</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							{t("settings.notifications.webhook.des_1")}{" "}
							<a href="https://containrrr.dev/shoutrrr/services/overview/" target="_blank" className="link">
								Shoutrrr
							</a>{" "}
							{t("settings.notifications.webhook.des_2")}
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
						{t("settings.notifications.webhook.add")} URL
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
					{t("settings.save_settings")}
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
				title: t("settings.notifications.webhook.test_sent"),
				description: t("settings.notifications.webhook.test_sent_des"),
			})
		} else {
			toast({
				title: t("error"),
				description: res.err ?? "Failed to send test notification",
				variant: "destructive",
			})
		}
		setIsLoading(false)
	}

	return (
		<Card className="bg-muted/30 p-2 md:p-3">
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
							{t("settings.notifications.webhook.test")} <span className="hidden sm:inline">URL</span>
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
