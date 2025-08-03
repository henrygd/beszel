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
import { Unit } from "@/lib/enums"

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
							<Trans comment="Temperature / network units">Unit preferences</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Change display units for metrics.</Trans>
						</p>
					</div>
					<div className="grid sm:grid-cols-3 gap-4">
						<div className="space-y-2">
							<Label className="block" htmlFor="unitTemp">
								<Trans>Temperature unit</Trans>
							</Label>
							<Select
								name="unitTemp"
								key={userSettings.unitTemp}
								defaultValue={userSettings.unitTemp?.toString() || String(Unit.Celsius)}
							>
								<SelectTrigger id="unitTemp">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={String(Unit.Celsius)}>
										<Trans>Celsius (°C)</Trans>
									</SelectItem>
									<SelectItem value={String(Unit.Fahrenheit)}>
										<Trans>Fahrenheit (°F)</Trans>
									</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div className="space-y-2">
							<Label className="block" htmlFor="unitNet">
								<Trans comment="Context: Bytes or bits">Network unit</Trans>
							</Label>
							<Select
								name="unitNet"
								key={userSettings.unitNet}
								defaultValue={userSettings.unitNet?.toString() ?? String(Unit.Bytes)}
							>
								<SelectTrigger id="unitNet">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={String(Unit.Bytes)}>
										<Trans>Bytes (KB/s, MB/s, GB/s)</Trans>
									</SelectItem>
									<SelectItem value={String(Unit.Bits)}>
										<Trans>Bits (Kbps, Mbps, Gbps)</Trans>
									</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div className="space-y-2">
							<Label className="block" htmlFor="unitDisk">
								<Trans>Disk unit</Trans>
							</Label>
							<Select
								name="unitDisk"
								key={userSettings.unitDisk}
								defaultValue={userSettings.unitDisk?.toString() ?? String(Unit.Bytes)}
							>
								<SelectTrigger id="unitDisk">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={String(Unit.Bytes)}>
										<Trans>Bytes (KB/s, MB/s, GB/s)</Trans>
									</SelectItem>
									<SelectItem value={String(Unit.Bits)}>
										<Trans>Bits (Kbps, Mbps, Gbps)</Trans>
									</SelectItem>
								</SelectContent>
							</Select>
						</div>
					</div>
				</div>
				<Separator />
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Warning thresholds</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Set percentage thresholds for meter colors.</Trans>
						</p>
					</div>
					<div className="grid grid-cols-2 lg:grid-cols-3 gap-4 items-end">
						<div className="space-y-1">
							<Label htmlFor="colorWarn">
								<Trans>Warning (%)</Trans>
							</Label>
							<Input
								id="colorWarn"
								name="colorWarn"
								type="number"
								min={1}
								max={100}
								className="min-w-24"
								defaultValue={userSettings.colorWarn ?? 65}
							/>
						</div>
						<div className="space-y-1">
							<Label htmlFor="colorCrit">
								<Trans>Critical (%)</Trans>
							</Label>
							<Input
								id="colorCrit"
								name="colorCrit"
								type="number"
								min={1}
								max={100}
								className="min-w-24"
								defaultValue={userSettings.colorCrit ?? 90}
							/>
						</div>
					</div>
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
