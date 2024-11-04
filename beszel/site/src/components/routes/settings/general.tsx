import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { chartTimeData } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import { LanguagesIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { UserSettings } from "@/types"
import { saveSettings } from "./layout"
import { useState } from "react"
import { Trans } from "@lingui/macro"
import languages from "@/lib/languages"
import { dynamicActivate } from "@/lib/i18n"
import { useLingui } from "@lingui/react"
// import { setLang } from "@/lib/i18n"

export default function SettingsProfilePage({ userSettings }: { userSettings: UserSettings }) {
	const [isLoading, setIsLoading] = useState(false)
	const { i18n } = useLingui()

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault()
		setIsLoading(true)
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Partial<UserSettings>
		await saveSettings(data)
		setIsLoading(false)
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>General</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>Change general application options.</Trans>
				</p>
			</div>
			<Separator className="my-4" />
			<form onSubmit={handleSubmit} className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium flex items-center gap-2">
							<LanguagesIcon className="h-4 w-4" />
							<Trans>Language</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>
								Want to help us make our translations even better? Check out{" "}
								<a href="https://crowdin.com/project/beszel" className="link" target="_blank" rel="noopener noreferrer">
									Crowdin
								</a>{" "}
								for more details.
							</Trans>
						</p>
					</div>
					<Label className="block" htmlFor="lang">
						<Trans>Preferred Language</Trans>
					</Label>
					<Select value={i18n.locale} onValueChange={(lang: string) => dynamicActivate(lang)}>
						<SelectTrigger id="lang">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{languages.map((lang) => (
								<SelectItem key={lang.lang} value={lang.lang}>
									<span className="me-2.5">{lang.e}</span>
									{lang.label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				<Separator />
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Chart options</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Adjust display options for charts.</Trans>
						</p>
					</div>
					<Label className="block" htmlFor="chartTime">
						<Trans>Default time period</Trans>
					</Label>
					<Select name="chartTime" key={userSettings.chartTime} defaultValue={userSettings.chartTime}>
						<SelectTrigger id="chartTime">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{Object.entries(chartTimeData).map(([value, { label }]) => (
								<SelectItem key={value} value={value}>
									{label()}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
					<p className="text-[0.8rem] text-muted-foreground">
						<Trans>Sets the default time range for charts when a system is viewed.</Trans>
					</p>
				</div>
				<Separator />
				<Button type="submit" className="flex items-center gap-1.5 disabled:opacity-100" disabled={isLoading}>
					{isLoading ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : <SaveIcon className="h-4 w-4" />}
					<Trans>Save Settings</Trans>
				</Button>
			</form>
		</div>
	)
}
