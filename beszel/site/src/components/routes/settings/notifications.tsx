import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { pb } from '@/lib/stores'
import { Separator } from '@/components/ui/separator'
import { Card } from '@/components/ui/card'
// import { Switch } from '@/components/ui/switch'
import { Trash2Icon } from 'lucide-react'

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
							Get notified when new alerts are created.
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
								href="https://containrrr.dev/shoutrrr/v0.8/services/overview/"
								target="_blank"
								className="font-medium text-primary opacity-80 hover:opacity-100 duration-100"
							>
								Shoutrrr
							</a>{' '}
							to integrate with popular notification services.
						</p>
					</div>
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
								placeholder="generic://example.com?@header=value"
							/>
							<Button
								type="button"
								variant="outline"
								className=""
								// onClick={() => append({ value: '' })}
							>
								Test
							</Button>
							<Button
								type="button"
								variant="outline"
								size="icon"
								className="shrink-0"
								// onClick={() => append({ value: '' })}
							>
								<Trash2Icon className="h-4 w-4" />
							</Button>
							{/* <Label htmlFor="enabled-01" className="sr-only">
								Enabled
							</Label>
							<Switch defaultChecked id="enabled-01" className="ml-2" /> */}
						</div>
					</Card>
					<Button
						type="button"
						variant="outline"
						size="sm"
						className="mt-2"
						// onClick={() => append({ value: '' })}
					>
						Add URL
					</Button>
				</div>
				<Separator />
				<Button type="submit">Save settings</Button>
			</div>
		</div>
	)
}
