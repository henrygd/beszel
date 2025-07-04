import { Trans } from "@lingui/react/macro"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { chartTimeData } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import { LanguagesIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { UserSettings } from "@/types"
import { saveSettings } from "./layout"
import { useState } from "react"
import languages from "@/lib/languages"
import { dynamicActivate } from "@/lib/i18n"
import { useLingui } from "@lingui/react/macro"
import { Input } from "@/components/ui/input"
// import { setLang } from "@/lib/i18n"

export default function SettingsProfilePage({ userSettings }: { userSettings: UserSettings }) {
	const [isLoading, setIsLoading] = useState(false)
	const { i18n } = useLingui()

	// Remove all per-metric threshold state and UI
	// Only keep general yellow/red threshold state and UI
	const [yellow, setYellow] = useState(userSettings.meterThresholds?.yellow ?? 65)
	const [red, setRed] = useState(userSettings.meterThresholds?.red ?? 90)

	function handleResetThresholds() {
		setYellow(65)
		setRed(90)
	}

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault()
		setIsLoading(true)
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Partial<UserSettings>
		data.meterThresholds = { yellow, red }
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
								Want to help improve our translations? Check{" "}
								<a href="https://crowdin.com/project/beszel" className="link" target="_blank" rel="noopener noreferrer">
									Crowdin
								</a>{" "}
								for details.
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
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Dashboard meter thresholds</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Choose when the dashboard meters changes colors, based on percentage values.</Trans>
						</p>
					</div>
					<div className="flex gap-4 items-end">
						<div>
							<Label htmlFor="yellow-threshold"><Trans>Warning threshold (%)</Trans></Label>
							<Input
								type="number"
								id="yellow-threshold"
								min={1}
								max={100}
								value={yellow}
								onChange={e => setYellow(Number(e.target.value))}
							/>
						</div>
						<div>
							<Label htmlFor="red-threshold"><Trans>Danger threshold (%)</Trans></Label>
							<Input
								type="number"
								id="red-threshold"
								min={1}
								max={100}
								value={red}
								onChange={e => setRed(Number(e.target.value))}
							/>
						</div>
						<Button type="button" variant="outline" onClick={handleResetThresholds} disabled={isLoading} className="mt-4">
							<Trans>Reset to default</Trans>
						</Button>
					</div>
				</div>
				<Separator />
				<div className="flex gap-2">
					<Button type="submit" className="flex items-center gap-1.5 disabled:opacity-100" disabled={isLoading}>
						{isLoading ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : <SaveIcon className="h-4 w-4" />}
						<Trans>Save Settings</Trans>
					</Button>
				</div>
			</form>
		</div>
	)
}
