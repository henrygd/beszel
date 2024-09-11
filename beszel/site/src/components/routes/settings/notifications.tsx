import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { pb } from '@/lib/stores'
import { Separator } from '@/components/ui/separator'
import { Card } from '@/components/ui/card'
// import { Switch } from '@/components/ui/switch'
import { LoaderCircleIcon, PlusIcon, SaveIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from '@/components/ui/use-toast'

export default function SettingsNotificationsPage() {
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
							Leave the emails field to disable email notifications.
						</p>
					</div>
					<Label className="block">To email(s)</Label>
					<Input placeholder="name@example.com" defaultValue={pb.authStore.model?.email} />
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
					<ShoutrrrUrlCard />
					<Button
						type="button"
						variant="outline"
						size="sm"
						className="mt-2 flex items-center gap-1"
						// onClick={() => append({ value: '' })}
					>
						<PlusIcon className="h-4 w-4 -ml-0.5" />
						Add URL
					</Button>
				</div>
				<Separator />
				<Button type="submit" className="flex items-center gap-1.5">
					<SaveIcon className="h-4 w-4" />
					Save settings
				</Button>
			</div>
		</div>
	)
}

async function sendTestNotification(url: string) {
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
}

// todo unique ids
function ShoutrrrUrlCard() {
	const [url, setUrl] = useState('')
	const [isLoading, setIsLoading] = useState(false)

	return (
		<Card className="bg-muted/30 p-3.5">
			<div className="flex items-center gap-1">
				<Label htmlFor="name" className="sr-only">
					URL
				</Label>
				<Input
					id="name"
					name="name"
					className="light:bg-card"
					required
					placeholder="generic://webhook.site/xxxxxx"
					onChange={(e) => setUrl(e.target.value)}
				/>
				<Button
					type="button"
					variant="outline"
					className="w-28"
					disabled={isLoading || url === ''}
					onClick={async () => {
						setIsLoading(true)
						await sendTestNotification(url)
						setIsLoading(false)
					}}
				>
					{isLoading ? <LoaderCircleIcon className="sh-4 w-4 animate-spin" /> : 'Test URL'}
				</Button>
				<Button
					type="button"
					variant="outline"
					size="icon"
					className="shrink-0"
					// onClick={() => append({ value: '' })}
				>
					<Trash2Icon className="sh-4 w-4" />
				</Button>
				{/* <Label htmlFor="enabled-01" className="sr-only">
								Enabled
							</Label>
							<Switch defaultChecked id="enabled-01" className="ml-2" /> */}
			</div>
		</Card>
	)
}
