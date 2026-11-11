import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import {
	BellIcon,
	LoaderCircleIcon,
	MailIcon,
	MoreHorizontalIcon,
	PenSquareIcon,
	PlusIcon,
	SendIcon,
	Trash2Icon,
} from "lucide-react"
import { type ChangeEventHandler, type FormEvent, useEffect, useState } from "react"
import * as v from "valibot"
import { prependBasePath } from "@/components/router"
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
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { InputTags } from "@/components/ui/input-tags"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"
import { $notificationChannels } from "@/lib/notification-channels"
import { cn } from "@/lib/utils"
import type { NotificationChannelRecord } from "@/types"
import { QuietHours } from "./quiet-hours"
import type { ClientResponseError } from "pocketbase"

interface ShoutrrrUrlCardProps {
	url: string
	onUrlChange: ChangeEventHandler<HTMLInputElement>
	onRemove: () => void
}

const ChannelSchema = v.object({
	name: v.pipe(v.string(), v.trim(), v.nonEmpty()),
	default: v.boolean(),
	emails: v.array(v.pipe(v.string(), v.rfcEmail())),
	webhooks: v.array(v.pipe(v.string(), v.url())),
})

const SettingsNotificationsPage = () => {
	const channels = useStore($notificationChannels)
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editingChannel, setEditingChannel] = useState<NotificationChannelRecord | null>(null)
	const [deletingChannel, setDeletingChannel] = useState<NotificationChannelRecord | null>(null)

	if (!isAdmin()) {
		return null
	}

	function openDialog(channel: NotificationChannelRecord | null) {
		setEditingChannel(channel)
		setDialogOpen(true)
	}

	async function deleteChannel(channel: NotificationChannelRecord) {
		try {
			await pb.collection("notification_channels").delete(channel.id)
		} catch (e: unknown) {
			toast({ title: t`Error`, description: (e as Error).message, variant: "destructive" })
		}
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>Notifications</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						Notification channels are destinations for alerts. Choose channels for each alert, or mark a channel as
						default to receive alerts without selected channels.
					</Trans>
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
				<div className="space-y-3">
					<div className="grid grid-cols-1 sm:flex items-center justify-between gap-4">
						<div>
							<h3 className="mb-1 text-lg font-medium">
								<Trans>Channels</Trans>
							</h3>
							<p className="text-sm text-muted-foreground leading-relaxed">
								<Trans>Send alerts by email or to services like Slack, Discord, or ntfy.</Trans>
							</p>
						</div>
						<Button type="button" variant="outline" className="h-10 shrink-0" onClick={() => openDialog(null)}>
							<PlusIcon className="size-4" />
							<span className="ms-1">
								<Trans>Add channel</Trans>
							</span>
						</Button>
					</div>
					{channels.length > 0 ? (
						<div className="rounded-md border overflow-x-auto whitespace-nowrap">
							<Table>
								<TableHeader>
									<TableRow className="border-border/50">
										<TableHead className="px-4">
											<Trans>Name</Trans>
										</TableHead>
										<TableHead className="px-4">
											<Trans>Destinations</Trans>
										</TableHead>
										<TableHead className="px-4 text-right sr-only">
											<Trans>Actions</Trans>
										</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{channels.map((channel) => (
										<TableRow key={channel.id}>
											<TableCell className="px-4 py-3">
												<span className="flex items-center gap-2">
													<span className="truncate max-w-60">{channel.name}</span>
													{channel.default && (
														<Badge variant="success">
															<Trans>Default</Trans>
														</Badge>
													)}
												</span>
											</TableCell>
											<TableCell className="px-4 py-3 text-muted-foreground">
												<span className="flex items-center gap-4">
													<span className="flex items-center gap-1.5">
														<MailIcon className="size-4" />
														{channel.emails?.length ?? 0}
													</span>
													<span className="flex items-center gap-1.5">
														<SendIcon className="size-4" />
														{channel.webhooks?.length ?? 0}
													</span>
												</span>
											</TableCell>
											<TableCell className="px-4 py-3 text-right">
												<DropdownMenu>
													<DropdownMenuTrigger asChild>
														<Button variant="ghost" size="icon" className="size-8">
															<span className="sr-only">
																<Trans>Open menu</Trans>
															</span>
															<MoreHorizontalIcon className="size-4" />
														</Button>
													</DropdownMenuTrigger>
													<DropdownMenuContent align="end">
														<DropdownMenuItem onClick={() => openDialog(channel)}>
															<PenSquareIcon className="me-2.5 size-4" />
															<Trans>Edit</Trans>
														</DropdownMenuItem>
														<DropdownMenuSeparator />
														<DropdownMenuItem onClick={() => setDeletingChannel(channel)}>
															<Trash2Icon className="me-2.5 size-4" />
															<Trans>Delete</Trans>
														</DropdownMenuItem>
													</DropdownMenuContent>
												</DropdownMenu>
											</TableCell>
										</TableRow>
									))}
								</TableBody>
							</Table>
						</div>
					) : (
						<p className="text-sm text-muted-foreground rounded-md border border-dashed p-4 text-center">
							<Trans>No channels yet. Alerts are recorded in the alert history but not sent anywhere.</Trans>
						</p>
					)}
				</div>
				<Separator />
				<div className="space-y-3">
					<QuietHours />
				</div>
			</div>
			<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
				{dialogOpen && <ChannelDialog channel={editingChannel} onClose={() => setDialogOpen(false)} />}
			</Dialog>
			<AlertDialog open={!!deletingChannel} onOpenChange={(open) => !open && setDeletingChannel(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Are you sure?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>
								This will delete the channel and its quiet hours. Alerts that only use this channel will fall back to
								the default channels.
							</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn(buttonVariants({ variant: "destructive" }))}
							onClick={() => deletingChannel && deleteChannel(deletingChannel)}
						>
							<Trans>Delete</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	)
}

function ChannelDialog({ channel, onClose }: { channel: NotificationChannelRecord | null; onClose: () => void }) {
	const [name, setName] = useState(channel?.name ?? "")
	const [isDefault, setIsDefault] = useState(channel?.default ?? false)
	const [emails, setEmails] = useState<string[]>(channel?.emails ?? [])
	const [webhooks, setWebhooks] = useState<string[]>(channel?.webhooks ?? [])
	const [isLoading, setIsLoading] = useState(false)

	// focus the newest webhook input
	const [focusLastWebhook, setFocusLastWebhook] = useState(false)
	useEffect(() => {
		if (focusLastWebhook) {
			const inputs = document.querySelectorAll("#webhooks input") as NodeListOf<HTMLInputElement>
			inputs[inputs.length - 1]?.focus()
			setFocusLastWebhook(false)
		}
	}, [focusLastWebhook])

	function updateWebhook(index: number, value: string) {
		const newWebhooks = [...webhooks]
		newWebhooks[index] = value
		setWebhooks(newWebhooks)
	}

	async function handleSubmit(e: FormEvent) {
		e.preventDefault()
		setIsLoading(true)
		try {
			const data = v.parse(ChannelSchema, { name, default: isDefault, emails, webhooks })
			if (channel) {
				await pb.collection("notification_channels").update(channel.id, data)
			} else {
				await pb.collection("notification_channels").create(data)
			}
			onClose()
		} catch (e: unknown) {
			toast({
				title: t`Failed to save settings`,
				description: (e as Error).message,
				variant: "destructive",
			})
		} finally {
			setIsLoading(false)
		}
	}

	return (
		<DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
			<DialogHeader>
				<DialogTitle>{channel ? <Trans>Edit channel</Trans> : <Trans>Add channel</Trans>}</DialogTitle>
				<DialogDescription>
					<Trans>Alerts sent to this channel go to every email address and URL below.</Trans>
				</DialogDescription>
			</DialogHeader>
			<form onSubmit={handleSubmit} className="space-y-5">
				<div className="grid gap-2">
					<Label htmlFor="channel-name">
						<Trans>Name</Trans>
					</Label>
					<Input
						id="channel-name"
						value={name}
						onChange={(e) => setName(e.target.value)}
						placeholder={t`e.g. Ops team`}
						maxLength={100}
						required
					/>
				</div>
				<label htmlFor="channel-default" className="flex items-center justify-between gap-4 cursor-pointer">
					<div>
						<p className="text-sm font-medium mb-0.5">
							<Trans>Default channel</Trans>
						</p>
						<p className="text-sm text-muted-foreground">
							<Trans>Receives alerts that have no channels selected, as well as SMART and ZFS alerts.</Trans>
						</p>
					</div>
					<Switch id="channel-default" checked={isDefault} onCheckedChange={setIsDefault} />
				</label>
				<Separator />
				<div className="grid gap-2">
					<div>
						<h4 className="mb-1 font-medium">
							<Trans>Email notifications</Trans>
						</h4>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>
								Please{" "}
								<a href={prependBasePath("/_/#/settings/mail")} className="link" target="_blank">
									configure an SMTP server
								</a>{" "}
								to ensure alerts are delivered.
							</Trans>
						</p>
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
						<Trans>Save address using enter key or comma.</Trans>
					</p>
				</div>
				<Separator />
				<div className="space-y-3">
					<div className="grid grid-cols-1 sm:flex items-center justify-between gap-4">
						<div>
							<h4 className="mb-1 font-medium">
								<Trans>Webhook / Push notifications</Trans>
							</h4>
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
						<Button
							type="button"
							variant="outline"
							className="h-10 shrink-0"
							onClick={() => {
								setWebhooks([...webhooks, ""])
								setFocusLastWebhook(true)
							}}
						>
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
									onUrlChange={(e) => updateWebhook(index, e.target.value)}
									onRemove={() => setWebhooks(webhooks.filter((_, i) => i !== index))}
								/>
							))}
						</div>
					)}
				</div>
				<DialogFooter>
					<Button type="button" variant="outline" onClick={onClose}>
						<Trans>Cancel</Trans>
					</Button>
					<Button type="submit" disabled={isLoading} className="disabled:opacity-100">
						{isLoading && <LoaderCircleIcon className="h-4 w-4 animate-spin" />}
						{channel ? <Trans>Update</Trans> : <Trans>Create</Trans>}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
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
