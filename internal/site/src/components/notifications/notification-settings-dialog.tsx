"use client"

import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { z } from "zod"
import { useToast } from "@/components/ui/use-toast"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import {
	Form,
	FormControl,
	FormDescription,
	FormField,
	FormItem,
	FormLabel,
	FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import {
	createNotification,
	updateNotification,
	testNotification,
	getDefaultSettings,
	getProviderLabel,
	type Notification,
	type NotificationSettings,
	type NotificationType,
	type UpdateNotificationRequest,
} from "@/lib/notifications"

const providerTypes: NotificationType[] = [
	"email",
	"webhook",
	"discord",
	"slack",
	"telegram",
	"gotify",
	"pushover",
]

interface NotificationSettingsDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	notification?: Notification | null
	isEdit?: boolean
}

export function NotificationSettingsDialog({
	open,
	onOpenChange,
	notification,
	isEdit = false,
}: NotificationSettingsDialogProps) {
	const { toast } = useToast()
	const queryClient = useQueryClient()
	const [selectedType, setSelectedType] = useState<NotificationType>(
		notification?.type || "email"
	)

	const createMutation = useMutation({
		mutationFn: createNotification,
		onSuccess: () => {
			toast({ title: "Notification provider created successfully" })
			queryClient.invalidateQueries({ queryKey: ["notifications"] })
			onOpenChange(false)
		},
		onError: (error: Error) => {
			toast({
				title: "Failed to create notification",
				description: error.message,
				variant: "destructive",
			})
		},
	})

	const updateMutation = useMutation({
		mutationFn: ({ id, data }: { id: string; data: UpdateNotificationRequest }) =>
			updateNotification(id, data),
		onSuccess: () => {
			toast({ title: "Notification provider updated successfully" })
			queryClient.invalidateQueries({ queryKey: ["notifications"] })
			onOpenChange(false)
		},
		onError: (error: Error) => {
			toast({
				title: "Failed to update notification",
				description: error.message,
				variant: "destructive",
			})
		},
	})

	const testMutation = useMutation({
		mutationFn: testNotification,
		onSuccess: () => {
			toast({ title: "Test notification sent successfully" })
		},
		onError: (error: Error) => {
			toast({
				title: "Failed to send test notification",
				description: error.message,
				variant: "destructive",
			})
		},
	})

	const handleSubmit = (data: FormData) => {
		if (isEdit && notification) {
			updateMutation.mutate({
				id: notification.id,
				data: {
					name: data.name,
					settings: data.settings as unknown as NotificationSettings,
					is_default: data.is_default,
				},
			})
		} else {
			createMutation.mutate({
				name: data.name,
				type: selectedType,
				settings: data.settings as unknown as NotificationSettings,
				is_default: data.is_default,
			})
		}
	}

	const handleTest = () => {
		if (notification?.id) {
			testMutation.mutate(notification.id)
		}
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-[500px]">
				<DialogHeader>
					<DialogTitle>
						{isEdit ? "Edit Notification Provider" : "Add Notification Provider"}
					</DialogTitle>
					<DialogDescription>
						Configure how you want to receive alerts when monitors go down.
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4 py-4">
					{!isEdit && (
						<div className="space-y-2">
							<label className="text-sm font-medium">Provider Type</label>
							<Select
								value={selectedType}
								onValueChange={(v) => setSelectedType(v as NotificationType)}
							>
								<SelectTrigger>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{providerTypes.map((type) => (
										<SelectItem key={type} value={type}>
											{getProviderLabel(type)}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					)}

					<ProviderForm
						type={selectedType}
						isEdit={isEdit}
						notification={notification}
						onSubmit={handleSubmit}
						isPending={createMutation.isPending || updateMutation.isPending}
						onCancel={() => onOpenChange(false)}
						onTest={isEdit ? handleTest : undefined}
						testPending={testMutation.isPending}
					/>
				</div>
			</DialogContent>
		</Dialog>
	)
}

interface ProviderFormProps {
	type: NotificationType
	isEdit: boolean
	notification?: Notification | null
	onSubmit: (data: FormData) => void
	isPending: boolean
	onCancel: () => void
	onTest?: () => void
	testPending?: boolean
}

interface FormData {
	name: string
	is_default: boolean
	settings: Record<string, unknown>
}

function ProviderForm({
	type,
	notification,
	onSubmit,
	isPending,
	onCancel,
	onTest,
	testPending,
}: ProviderFormProps) {
	const defaultValues: FormData = {
		name: notification?.name || "",
		is_default: notification?.is_default || false,
		settings: (notification?.settings || getDefaultSettings(type)) as Record<string, unknown>,
	}

	switch (type) {
		case "email":
			return (
				<EmailForm
					defaultValues={defaultValues as EmailFormData}
					onSubmit={onSubmit}
					isPending={isPending}
					onCancel={onCancel}
					onTest={onTest}
					testPending={testPending}
				/>
			)
		case "webhook":
			return (
				<WebhookForm
					defaultValues={defaultValues as WebhookFormData}
					onSubmit={onSubmit}
					isPending={isPending}
					onCancel={onCancel}
					onTest={onTest}
					testPending={testPending}
				/>
			)
		case "discord":
			return (
				<DiscordForm
					defaultValues={defaultValues as DiscordFormData}
					onSubmit={onSubmit}
					isPending={isPending}
					onCancel={onCancel}
					onTest={onTest}
					testPending={testPending}
				/>
			)
		case "slack":
			return (
				<SlackForm
					defaultValues={defaultValues as SlackFormData}
					onSubmit={onSubmit}
					isPending={isPending}
					onCancel={onCancel}
					onTest={onTest}
					testPending={testPending}
				/>
			)
		case "telegram":
			return (
				<TelegramForm
					defaultValues={defaultValues as TelegramFormData}
					onSubmit={onSubmit}
					isPending={isPending}
					onCancel={onCancel}
					onTest={onTest}
					testPending={testPending}
				/>
			)
		case "gotify":
			return (
				<GotifyForm
					defaultValues={defaultValues as GotifyFormData}
					onSubmit={onSubmit}
					isPending={isPending}
					onCancel={onCancel}
					onTest={onTest}
					testPending={testPending}
				/>
			)
		case "pushover":
			return (
				<PushoverForm
					defaultValues={defaultValues as PushoverFormData}
					onSubmit={onSubmit}
					isPending={isPending}
					onCancel={onCancel}
					onTest={onTest}
					testPending={testPending}
				/>
			)
		default:
			return null
	}
}

// Form schemas and components for each provider type
const emailSchema = z.object({
	name: z.string().min(1, "Name is required"),
	is_default: z.boolean(),
	settings: z.object({
		smtp_host: z.string().min(1, "SMTP host is required"),
		smtp_port: z.coerce.number().min(1, "Port is required"),
		smtp_user: z.string(),
		smtp_password: z.string(),
		from_email: z.string().email("Invalid email"),
		to_email: z.string().email("Invalid email"),
		use_tls: z.boolean(),
	}),
})

type EmailFormData = z.infer<typeof emailSchema>

function EmailForm({
	defaultValues,
	onSubmit,
	isPending,
	onCancel,
	onTest,
	testPending,
}: FormComponentProps<EmailFormData>) {
	const form = useForm<z.input<typeof emailSchema>, any, z.output<typeof emailSchema>>({
		resolver: zodResolver(emailSchema),
		defaultValues,
	})

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				<FormField
					control={form.control}
					name="name"
					render={({ field }) => (
						<FormItem>
							<FormLabel>Name</FormLabel>
							<FormControl>
								<Input placeholder="My Email Notification" {...field} />
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				<div className="grid grid-cols-2 gap-4">
					<FormField
						control={form.control}
						name="settings.smtp_host"
						render={({ field }) => (
							<FormItem>
								<FormLabel>SMTP Host</FormLabel>
								<FormControl>
									<Input placeholder="smtp.gmail.com" {...field} />
								</FormControl>
								<FormMessage />
							</FormItem>
						)}
					/>
					<FormField
						control={form.control}
						name="settings.smtp_port"
						render={({ field }) => (
							<FormItem>
								<FormLabel>SMTP Port</FormLabel>
								<FormControl>
									<Input type="number" {...field} value={field.value as string | number | undefined} />
								</FormControl>
								<FormMessage />
							</FormItem>
						)}
					/>
				</div>

				<div className="grid grid-cols-2 gap-4">
					<FormField
						control={form.control}
						name="settings.smtp_user"
						render={({ field }) => (
							<FormItem>
								<FormLabel>SMTP Username</FormLabel>
								<FormControl>
									<Input {...field} />
								</FormControl>
							</FormItem>
						)}
					/>
					<FormField
						control={form.control}
						name="settings.smtp_password"
						render={({ field }) => (
							<FormItem>
								<FormLabel>SMTP Password</FormLabel>
								<FormControl>
									<Input type="password" {...field} />
								</FormControl>
							</FormItem>
						)}
					/>
				</div>

				<div className="grid grid-cols-2 gap-4">
					<FormField
						control={form.control}
						name="settings.from_email"
						render={({ field }) => (
							<FormItem>
								<FormLabel>From Email</FormLabel>
								<FormControl>
									<Input placeholder="alerts@example.com" {...field} />
								</FormControl>
								<FormMessage />
							</FormItem>
						)}
					/>
					<FormField
						control={form.control}
						name="settings.to_email"
						render={({ field }) => (
							<FormItem>
								<FormLabel>To Email</FormLabel>
								<FormControl>
									<Input placeholder="admin@example.com" {...field} />
								</FormControl>
								<FormMessage />
							</FormItem>
						)}
					/>
				</div>

				<FormField
					control={form.control}
					name="settings.use_tls"
					render={({ field }) => (
						<FormItem className="flex items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>Use TLS</FormLabel>
								<FormDescription>Enable TLS encryption</FormDescription>
							</div>
							<FormControl>
								<Switch
									checked={field.value}
									onCheckedChange={field.onChange}
								/>
							</FormControl>
						</FormItem>
					)}
				/>

				<FormField
					control={form.control}
					name="is_default"
					render={({ field }) => (
						<FormItem className="flex items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>Default Provider</FormLabel>
								<FormDescription>
									Automatically enable for new monitors
								</FormDescription>
							</div>
							<FormControl>
								<Switch
									checked={field.value}
									onCheckedChange={field.onChange}
								/>
							</FormControl>
						</FormItem>
					)}
				/>

				<DialogFooter className="gap-2">
					{onTest && (
						<Button
							type="button"
							variant="outline"
							onClick={onTest}
							disabled={testPending}
						>
							{testPending ? "Sending..." : "Test"}
						</Button>
					)}
					<Button type="button" variant="outline" onClick={onCancel}>
						Cancel
					</Button>
					<Button type="submit" disabled={isPending}>
						{isPending ? "Saving..." : "Save"}
					</Button>
				</DialogFooter>
			</form>
		</Form>
	)
}

// Stub form components for other notification providers

const webhookSchema = z.object({
	name: z.string().min(1, "Name is required"),
	is_default: z.boolean(),
	settings: z.object({
		url: z.string().min(1, "Webhook URL is required"),
	}),
})

type WebhookFormData = z.infer<typeof webhookSchema>

function WebhookForm({ defaultValues, onSubmit, isPending, onCancel, onTest, testPending }: FormComponentProps<WebhookFormData>) {
	const form = useForm<z.input<typeof webhookSchema>, any, z.output<typeof webhookSchema>>({ resolver: zodResolver(webhookSchema), defaultValues })
	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				<FormField control={form.control} name="name" render={({ field }) => (
					<FormItem><FormLabel>Name</FormLabel><FormControl><Input placeholder="My Webhook" {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<FormField control={form.control} name="settings.url" render={({ field }) => (
					<FormItem><FormLabel>Webhook URL</FormLabel><FormControl><Input placeholder="https://hooks.example.com/..." {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<FormField control={form.control} name="is_default" render={({ field }) => (
					<FormItem className="flex items-center justify-between rounded-lg border p-3">
						<div className="space-y-0.5"><FormLabel>Default Provider</FormLabel></div>
						<FormControl><Switch checked={field.value} onCheckedChange={field.onChange} /></FormControl>
					</FormItem>
				)} />
				<DialogFooter className="gap-2">
					{onTest && <Button type="button" variant="outline" onClick={onTest} disabled={testPending}>{testPending ? "Sending..." : "Test"}</Button>}
					<Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
					<Button type="submit" disabled={isPending}>{isPending ? "Saving..." : "Save"}</Button>
				</DialogFooter>
			</form>
		</Form>
	)
}

const discordSchema = z.object({
	name: z.string().min(1, "Name is required"),
	is_default: z.boolean(),
	settings: z.object({
		webhook_url: z.string().min(1, "Discord Webhook URL is required"),
	}),
})

type DiscordFormData = z.infer<typeof discordSchema>

function DiscordForm({ defaultValues, onSubmit, isPending, onCancel, onTest, testPending }: FormComponentProps<DiscordFormData>) {
	const form = useForm<z.input<typeof discordSchema>, any, z.output<typeof discordSchema>>({ resolver: zodResolver(discordSchema), defaultValues })
	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				<FormField control={form.control} name="name" render={({ field }) => (
					<FormItem><FormLabel>Name</FormLabel><FormControl><Input placeholder="Discord Alerts" {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<FormField control={form.control} name="settings.webhook_url" render={({ field }) => (
					<FormItem><FormLabel>Discord Webhook URL</FormLabel><FormControl><Input placeholder="https://discord.com/api/webhooks/..." {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<FormField control={form.control} name="is_default" render={({ field }) => (
					<FormItem className="flex items-center justify-between rounded-lg border p-3">
						<div className="space-y-0.5"><FormLabel>Default Provider</FormLabel></div>
						<FormControl><Switch checked={field.value} onCheckedChange={field.onChange} /></FormControl>
					</FormItem>
				)} />
				<DialogFooter className="gap-2">
					{onTest && <Button type="button" variant="outline" onClick={onTest} disabled={testPending}>{testPending ? "Sending..." : "Test"}</Button>}
					<Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
					<Button type="submit" disabled={isPending}>{isPending ? "Saving..." : "Save"}</Button>
				</DialogFooter>
			</form>
		</Form>
	)
}

const slackSchema = z.object({
	name: z.string().min(1, "Name is required"),
	is_default: z.boolean(),
	settings: z.object({
		webhook_url: z.string().min(1, "Slack Webhook URL is required"),
	}),
})

type SlackFormData = z.infer<typeof slackSchema>

function SlackForm({ defaultValues, onSubmit, isPending, onCancel, onTest, testPending }: FormComponentProps<SlackFormData>) {
	const form = useForm<z.input<typeof slackSchema>, any, z.output<typeof slackSchema>>({ resolver: zodResolver(slackSchema), defaultValues })
	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				<FormField control={form.control} name="name" render={({ field }) => (
					<FormItem><FormLabel>Name</FormLabel><FormControl><Input placeholder="Slack Alerts" {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<FormField control={form.control} name="settings.webhook_url" render={({ field }) => (
					<FormItem><FormLabel>Slack Webhook URL</FormLabel><FormControl><Input placeholder="https://hooks.slack.com/services/..." {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<FormField control={form.control} name="is_default" render={({ field }) => (
					<FormItem className="flex items-center justify-between rounded-lg border p-3">
						<div className="space-y-0.5"><FormLabel>Default Provider</FormLabel></div>
						<FormControl><Switch checked={field.value} onCheckedChange={field.onChange} /></FormControl>
					</FormItem>
				)} />
				<DialogFooter className="gap-2">
					{onTest && <Button type="button" variant="outline" onClick={onTest} disabled={testPending}>{testPending ? "Sending..." : "Test"}</Button>}
					<Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
					<Button type="submit" disabled={isPending}>{isPending ? "Saving..." : "Save"}</Button>
				</DialogFooter>
			</form>
		</Form>
	)
}

const telegramSchema = z.object({
	name: z.string().min(1, "Name is required"),
	is_default: z.boolean(),
	settings: z.object({
		bot_token: z.string().min(1, "Bot Token is required"),
		chat_id: z.string().min(1, "Chat ID is required"),
	}),
})

type TelegramFormData = z.infer<typeof telegramSchema>

function TelegramForm({ defaultValues, onSubmit, isPending, onCancel, onTest, testPending }: FormComponentProps<TelegramFormData>) {
	const form = useForm<z.input<typeof telegramSchema>, any, z.output<typeof telegramSchema>>({ resolver: zodResolver(telegramSchema), defaultValues })
	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				<FormField control={form.control} name="name" render={({ field }) => (
					<FormItem><FormLabel>Name</FormLabel><FormControl><Input placeholder="Telegram Alerts" {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<div className="grid grid-cols-2 gap-4">
					<FormField control={form.control} name="settings.bot_token" render={({ field }) => (
						<FormItem><FormLabel>Bot Token</FormLabel><FormControl><Input placeholder="123456:ABC-DEF..." {...field} /></FormControl><FormMessage /></FormItem>
					)} />
					<FormField control={form.control} name="settings.chat_id" render={({ field }) => (
						<FormItem><FormLabel>Chat ID</FormLabel><FormControl><Input placeholder="-100123456789" {...field} /></FormControl><FormMessage /></FormItem>
					)} />
				</div>
				<FormField control={form.control} name="is_default" render={({ field }) => (
					<FormItem className="flex items-center justify-between rounded-lg border p-3">
						<div className="space-y-0.5"><FormLabel>Default Provider</FormLabel></div>
						<FormControl><Switch checked={field.value} onCheckedChange={field.onChange} /></FormControl>
					</FormItem>
				)} />
				<DialogFooter className="gap-2">
					{onTest && <Button type="button" variant="outline" onClick={onTest} disabled={testPending}>{testPending ? "Sending..." : "Test"}</Button>}
					<Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
					<Button type="submit" disabled={isPending}>{isPending ? "Saving..." : "Save"}</Button>
				</DialogFooter>
			</form>
		</Form>
	)
}

const gotifySchema = z.object({
	name: z.string().min(1, "Name is required"),
	is_default: z.boolean(),
	settings: z.object({
		server_url: z.string().min(1, "Server URL is required"),
		app_token: z.string().min(1, "App Token is required"),
	}),
})

type GotifyFormData = z.infer<typeof gotifySchema>

function GotifyForm({ defaultValues, onSubmit, isPending, onCancel, onTest, testPending }: FormComponentProps<GotifyFormData>) {
	const form = useForm<z.input<typeof gotifySchema>, any, z.output<typeof gotifySchema>>({ resolver: zodResolver(gotifySchema), defaultValues })
	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				<FormField control={form.control} name="name" render={({ field }) => (
					<FormItem><FormLabel>Name</FormLabel><FormControl><Input placeholder="Gotify Alerts" {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<div className="grid grid-cols-2 gap-4">
					<FormField control={form.control} name="settings.server_url" render={({ field }) => (
						<FormItem><FormLabel>Server URL</FormLabel><FormControl><Input placeholder="https://gotify.example.com" {...field} /></FormControl><FormMessage /></FormItem>
					)} />
					<FormField control={form.control} name="settings.app_token" render={({ field }) => (
						<FormItem><FormLabel>App Token</FormLabel><FormControl><Input placeholder="Abc..." {...field} /></FormControl><FormMessage /></FormItem>
					)} />
				</div>
				<FormField control={form.control} name="is_default" render={({ field }) => (
					<FormItem className="flex items-center justify-between rounded-lg border p-3">
						<div className="space-y-0.5"><FormLabel>Default Provider</FormLabel></div>
						<FormControl><Switch checked={field.value} onCheckedChange={field.onChange} /></FormControl>
					</FormItem>
				)} />
				<DialogFooter className="gap-2">
					{onTest && <Button type="button" variant="outline" onClick={onTest} disabled={testPending}>{testPending ? "Sending..." : "Test"}</Button>}
					<Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
					<Button type="submit" disabled={isPending}>{isPending ? "Saving..." : "Save"}</Button>
				</DialogFooter>
			</form>
		</Form>
	)
}

const pushoverSchema = z.object({
	name: z.string().min(1, "Name is required"),
	is_default: z.boolean(),
	settings: z.object({
		user_key: z.string().min(1, "User Key is required"),
		app_token: z.string().min(1, "App Token is required"),
	}),
})

type PushoverFormData = z.infer<typeof pushoverSchema>

function PushoverForm({ defaultValues, onSubmit, isPending, onCancel, onTest, testPending }: FormComponentProps<PushoverFormData>) {
	const form = useForm<z.input<typeof pushoverSchema>, any, z.output<typeof pushoverSchema>>({ resolver: zodResolver(pushoverSchema), defaultValues })
	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				<FormField control={form.control} name="name" render={({ field }) => (
					<FormItem><FormLabel>Name</FormLabel><FormControl><Input placeholder="Pushover Alerts" {...field} /></FormControl><FormMessage /></FormItem>
				)} />
				<div className="grid grid-cols-2 gap-4">
					<FormField control={form.control} name="settings.user_key" render={({ field }) => (
						<FormItem><FormLabel>User Key</FormLabel><FormControl><Input placeholder="u..." {...field} /></FormControl><FormMessage /></FormItem>
					)} />
					<FormField control={form.control} name="settings.app_token" render={({ field }) => (
						<FormItem><FormLabel>App Token</FormLabel><FormControl><Input placeholder="a..." {...field} /></FormControl><FormMessage /></FormItem>
					)} />
				</div>
				<FormField control={form.control} name="is_default" render={({ field }) => (
					<FormItem className="flex items-center justify-between rounded-lg border p-3">
						<div className="space-y-0.5"><FormLabel>Default Provider</FormLabel></div>
						<FormControl><Switch checked={field.value} onCheckedChange={field.onChange} /></FormControl>
					</FormItem>
				)} />
				<DialogFooter className="gap-2">
					{onTest && <Button type="button" variant="outline" onClick={onTest} disabled={testPending}>{testPending ? "Sending..." : "Test"}</Button>}
					<Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
					<Button type="submit" disabled={isPending}>{isPending ? "Saving..." : "Save"}</Button>
				</DialogFooter>
			</form>
		</Form>
	)
}

type FormComponentProps<T> = {
	defaultValues: T
	onSubmit: (data: any) => void
	isPending: boolean
	onCancel: () => void
	onTest?: () => void
	testPending?: boolean
}
