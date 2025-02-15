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
import { Input } from "@/components/ui/input"





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

	const [configContent, setConfigContent] = useState<ConnectionSettings>()

	async function fetchConnectionsConfig() {
		try {
			setIsLoading(true)
			const record = await pb.collection("connection_settings").getFirstListItem('')
			let c: ConnectionSettings = {
				max_awaiting_size: record.max_awaiting_size,
				withAPIKey: record.withAPIKey,
				external_address: record.external_address
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
					<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>
								Define how beszel should react when a new client connection occurs.
							</Trans>
						</p>
				</h3>

			</div>
			<Separator className="my-4" />
			<form onSubmit={handleSubmit} className="space-y-5">
				<div className="space-y-2">

					<Label className="block" htmlFor="withAPI">
						<Trans>New clients should</Trans>
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

					<Label className="block" htmlFor="withAPI">
						<Trans>Max number of waiting connections</Trans>
					</Label>
					{/* TODO make this changeable in beszel UI */}
					<Input value={configContent?.max_awaiting_size} readOnly={true}></Input>

					<Label className="block" htmlFor="withAPI">
						<Trans>External address</Trans>
					</Label>
					{/* TODO make this changeable in beszel UI */}
					<Input value={configContent?.external_address} readOnly={true}></Input>

				</div>
				<Button type="submit" className="flex items-center gap-1.5 disabled:opacity-100" disabled={isLoading}>
					{isLoading ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : <SaveIcon className="h-4 w-4" />}
					<Trans>Save Settings</Trans>
				</Button>
			</form>
		</div>
	)
}
