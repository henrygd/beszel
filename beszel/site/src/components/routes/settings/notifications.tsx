import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { pb } from '@/lib/stores'
import { Separator } from '@/components/ui/separator'
import { Card } from '@/components/ui/card'
import { LoaderCircleIcon, PlusIcon, SaveIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from '@/components/ui/use-toast'

interface UserSettings {
	webhooks: string[]
}

interface ShoutrrrUrlCardProps {
	url: string
	onUrlChange: (value: string) => void
	onRemove: () => void
}

const userSettings: UserSettings = {
	webhooks: ['generic://webhook.site/xxx'],
}

const SettingsNotificationsPage = () => {
	const [email, setEmail] = useState(pb.authStore.model?.email || '')
	const [webhooks, setWebhooks] = useState(userSettings.webhooks ?? [])

	const addWebhook = () => setWebhooks([...webhooks, ''])
	const removeWebhook = (index: number) => setWebhooks(webhooks.filter((_, i) => i !== index))

	const updateWebhook = (index: number, value: string) => {
		const newWebhooks = [...webhooks]
		newWebhooks[index] = value
		setWebhooks(newWebhooks)
	}

	const saveSettings = async () => {
		// TODO: Implement actual saving logic
		console.log('Saving settings:', { email, webhooks })
		toast({
			title: 'Settings saved',
			description: 'Your notification settings have been updated.',
		})
	}

	return (
		<div>
			{/* <div>
				<h3 className="text-xl font-medium mb-1">Notifications</h3>
				<p className="text-sm text-muted-foreground">Configure how you receive notifications.</p>
			</div>
			<Separator className="my-6" /> */}
			<div className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">Email notifications</h3>
						<p className="text-sm text-muted-foreground">
							Leave blank to disable email notifications.
						</p>
					</div>
					<Label className="block">To email(s)</Label>
					<Input
						placeholder="name@example.com"
						value={email}
						onChange={(e) => setEmail(e.target.value)}
					/>
					<p className="text-[0.8rem] text-muted-foreground">
						Separate multiple emails with commas.
					</p>
				</div>
				<Separator />
				<div className="space-y-4">
					<div>
						<h3 className="mb-1 text-lg font-medium">Webhook / Push notifications</h3>
						<p className="text-sm text-muted-foreground">
							Beszel uses{' '}
							<a
								href="https://containrrr.dev/shoutrrr/services/overview/"
								target="_blank"
								className="link"
							>
								Shoutrrr
							</a>{' '}
							to integrate with popular notification services.
						</p>
					</div>
					{webhooks.length > 0 && (
						<div className="grid gap-3">
							{webhooks.map((webhook, index) => (
								<ShoutrrrUrlCard
									key={index}
									url={webhook}
									onUrlChange={(value: string) => updateWebhook(index, value)}
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
						<PlusIcon className="h-4 w-4 -ml-0.5" />
						Add URL
					</Button>
				</div>
				<Separator />
				<Button type="button" className="flex items-center gap-1.5" onClick={saveSettings}>
					<SaveIcon className="h-4 w-4" />
					Save settings
				</Button>
			</div>
		</div>
	)
}

const ShoutrrrUrlCard = ({ url, onUrlChange, onRemove }: ShoutrrrUrlCardProps) => {
	const [isLoading, setIsLoading] = useState(false)

	const sendTestNotification = async () => {
		setIsLoading(true)
		const res = await pb.send('/api/beszel/send-test-notification', { url })
		if ('err' in res && !res.err) {
			toast({
				title: 'Test notification sent',
				description: 'Check your notification service',
			})
		} else {
			toast({
				title: 'Error',
				description: res.err ?? 'Failed to send test notification',
				variant: 'destructive',
			})
		}
		setIsLoading(false)
	}

	return (
		<Card className="bg-muted/30 p-2 md:p-3">
			<div className="flex items-center gap-1">
				<Input
					className="light:bg-card"
					required
					placeholder="generic://webhook.site/xxxxxx"
					value={url}
					onChange={(e) => onUrlChange(e.target.value)}
				/>
				<Button
					type="button"
					variant="outline"
					className="w-20 md:w-28"
					disabled={isLoading || url === ''}
					onClick={sendTestNotification}
				>
					{isLoading ? (
						<LoaderCircleIcon className="h-4 w-4 animate-spin" />
					) : (
						<span>
							Test <span className="hidden md:inline">URL</span>
						</span>
					)}
				</Button>
				<Button
					type="button"
					variant="outline"
					size="icon"
					className="shrink-0"
					aria-label="Delete"
					onClick={onRemove}
				>
					<Trash2Icon className="h-4 w-4" />
				</Button>
			</div>
		</Card>
	)
}

export default SettingsNotificationsPage
