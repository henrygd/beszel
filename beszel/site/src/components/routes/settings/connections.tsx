import { Button } from "@/components/ui/button"
import { connectionActionsData, isAdmin } from "@/lib/utils"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { LoaderCircleIcon, PickaxeIcon, SaveIcon } from "lucide-react"
import { ConnectionSettings, ConnectionSettingsActions, UserSettings } from "@/types"
import { useEffect, useState } from "react"
import { Trans, t } from "@lingui/macro"
import languages from "@/lib/languages"
import { dynamicActivate } from "@/lib/i18n"
import { useLingui } from "@lingui/react"
import { redirectPage } from "@nanostores/router"
import { $router } from "@/components/router"
import { pb } from "@/lib/stores"
import { toast } from "@/components/ui/use-toast"





export async function saveConnectionSettings(newSettings: Partial<ConnectionSettings>) {
	try {
		// get fresh copy of settings
		const req = await pb.collection("connection_settings").getFirstListItem("")
		// update user settings
		await pb.collection("connection_settings").update(req.id, {
				...newSettings,
			},
		)
		toast({
			title: t`Connection settings saved`,
			description: t`Connection settings have been updated.`,
		})
	} catch (e) {
		toast({
			title: t`Failed to save settings`,
			description: t`Check logs for more details.`,
			variant: "destructive",
		})
	}
}

export default function Connections() {
	const [isLoading, setIsLoading] = useState(false)
	const { i18n } = useLingui()

	const [configContent, setConfigContent] = useState<ConnectionSettings>()

	async function fetchConnectionsConfig() {
		try {
			setIsLoading(true)
			const record = await pb.collection("connection_settings").getFirstListItem('')
			let c: ConnectionSettings = {
				withoutAPIKey: record.withoutAPIKey,
				withAPIKey: record.withAPIKey
			}
			setConfigContent(c)
		} catch (error: any) {

		} finally {
			setIsLoading(false)
		}
	}

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault()
		setIsLoading(true)
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Partial<ConnectionSettings>
		await saveConnectionSettings(data)
		setIsLoading(false)
	}

	const handleWithAPIChange = (value: ConnectionSettingsActions) => {
		if (configContent) {
			setConfigContent({
				...configContent,
				withAPIKey: value
			})
		}
	}

	const handleWithoutAPIChange = (value: ConnectionSettingsActions) => {
		if (configContent) {
			setConfigContent({
				...configContent,
				withoutAPIKey: value
			})
		}
	}

	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
	}

	useEffect(() => {
		fetchConnectionsConfig()
	}, [])


	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>Connections</Trans>
				</h3>

			</div>
			<Separator className="my-4" />
			<form onSubmit={handleSubmit} className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium flex items-center gap-2">
							<PickaxeIcon className="h-4 w-4" />
							<Trans>Actions</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>
								Define how beszel should react when a new client connection occurs.
							</Trans>
						</p>
					</div>


					<Label className="block" htmlFor="withAPI">
						<Trans>With API Key</Trans>
					</Label>
					<Select name="withAPIKey" value={configContent?.withAPIKey} onValueChange={handleWithAPIChange}>
						<SelectTrigger id="withAPI">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{Object.entries(connectionActionsData).map(([value, { label }]) => (
								<SelectItem key={value} value={value}>
									{label()}
								</SelectItem>
							))}
						</SelectContent>
					</Select>

					<Label className="block" htmlFor="withoutAPI">
						<Trans>Without API Key</Trans>
					</Label>
					<Select name="withoutAPIKey" value={configContent?.withoutAPIKey} onValueChange={handleWithoutAPIChange}>
						<SelectTrigger id="withoutAPI">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{Object.entries(connectionActionsData).map(([value, { label }]) => (
								<SelectItem key={value} value={value}>
									{label()}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				<Button type="submit" className="flex items-center gap-1.5 disabled:opacity-100" disabled={isLoading}>
					{isLoading ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : <SaveIcon className="h-4 w-4" />}
					<Trans>Save Settings</Trans>
				</Button>
			</form>
		</div>
	)
}
