/** biome-ignore-all lint/correctness/useUniqueElementIds: component is only rendered once */
import { Trans, useLingui } from "@lingui/react/macro"
import { DatabaseIcon, LanguagesIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { useStore } from "@nanostores/react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import Slider from "@/components/ui/slider"
import { toast } from "@/components/ui/use-toast"
import { HourFormat, Unit } from "@/lib/enums"
import { dynamicActivate } from "@/lib/i18n"
import languages from "@/lib/languages"
import { $userSettings, defaultLayoutWidth } from "@/lib/stores"
import { chartTimeData, currentHour12 } from "@/lib/utils"
import { isAdmin, pb } from "@/lib/api"
import type { UserSettings } from "@/types"
import { saveSettings } from "./layout"

const retentionOptions = [
	{ value: "30d", label: "30 days (default)" },
	{ value: "60d", label: "60 days" },
	{ value: "90d", label: "90 days (3 months)" },
	{ value: "180d", label: "180 days (6 months)" },
	{ value: "365d", label: "365 days (1 year)" },
	{ value: "730d", label: "730 days (2 years)" },
	{ value: "never", label: "Never delete" },
] as const

export default function SettingsProfilePage({ userSettings }: { userSettings: UserSettings }) {
	const [isLoading, setIsLoading] = useState(false)
	const { i18n } = useLingui()
	const { t } = useLingui()
	const currentUserSettings = useStore($userSettings)
	const layoutWidth = currentUserSettings.layoutWidth ?? defaultLayoutWidth
	const [retention, setRetention] = useState<string>("30d")
	const [hubSettingsId, setHubSettingsId] = useState<string>("hubsettings00001")
	const [retentionLoading, setRetentionLoading] = useState(true)
	const [retentionSaving, setRetentionSaving] = useState(false)
	const admin = isAdmin()

	useEffect(() => {
		if (!admin) {
			setRetentionLoading(false)
			return
		}
		pb.collection("hub_settings")
			.getFirstListItem("", { fields: "id,retention" })
			.then((rec) => {
				setRetention((rec as unknown as { retention: string }).retention)
				setHubSettingsId(rec.id)
			})
			.catch(() => {
				// fallback try direct id
				pb.collection("hub_settings")
					.getOne("hubsettings00001", { fields: "id,retention" })
					.then((rec) => {
						setRetention((rec as unknown as { retention: string }).retention)
						setHubSettingsId(rec.id)
					})
					.catch(() => {})
			})
			.finally(() => setRetentionLoading(false))
	}, [admin])

	async function handleRetentionSave() {
		setRetentionSaving(true)
		try {
			await pb.collection("hub_settings").update(hubSettingsId, { retention })
			toast({ title: t`Retention saved`, description: t`Data retention updated to ${retention}. New setting applies on next hourly cleanup.` })
		} catch (_e) {
			toast({ title: t`Failed to save retention`, variant: "destructive" })
		}
		setRetentionSaving(false)
	}

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
				<div className="grid gap-2">
					<div className="mb-2">
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
							{languages.map(([lang, label, e]) => (
								<SelectItem key={lang} value={lang}>
									<span className="me-2.5">
										{e || (
											<code
												aria-hidden="true"
												className="font-mono bg-muted text-[.65em] w-5 h-4 inline-grid place-items-center"
											>
												{lang}
											</code>
										)}
									</span>
									{label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				<Separator />
				<div className="grid gap-2">
					<div className="mb-2">
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Layout width</Trans>
						</h3>
						<Label htmlFor="layoutWidth" className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Adjust the width of the main layout</Trans> ({layoutWidth}px)
						</Label>
					</div>
					<Slider
						id="layoutWidth"
						name="layoutWidth"
						value={[layoutWidth]}
						onValueChange={(val) => $userSettings.setKey("layoutWidth", val[0])}
						min={1000}
						max={2000}
						step={10}
						className="w-full mb-1"
					/>
				</div>
				<Separator />
				<div className="grid gap-2">
					<div className="mb-2">
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Chart options</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Adjust display options for charts.</Trans>
						</p>
					</div>
					<div className="grid sm:grid-cols-3 gap-4">
						<div className="grid gap-2">
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
						</div>
						<div className="grid gap-2">
							<Label className="block" htmlFor="hourFormat">
								<Trans>Time format</Trans>
							</Label>
							<Select
								name="hourFormat"
								key={userSettings.hourFormat}
								defaultValue={userSettings.hourFormat ?? (currentHour12() ? HourFormat["12h"] : HourFormat["24h"])}
							>
								<SelectTrigger id="hourFormat">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{Object.keys(HourFormat).map((value) => (
										<SelectItem key={value} value={value}>
											{value}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					</div>
				</div>
				{admin && (
					<>
						<Separator />
						<div className="grid gap-2">
							<div className="mb-2">
								<h3 className="mb-1 text-lg font-medium flex items-center gap-2">
									<DatabaseIcon className="h-4 w-4" />
									<Trans>Data retention</Trans>
								</h3>
								<p className="text-sm text-muted-foreground leading-relaxed">
									<Trans>
										How long to keep 8-hour (480m) aggregated stats. Shorter tiers are fixed: 1m for 1 hour, 10m for 12 hours, 20m for 1
										day, 120m for 7 days. Longer retention uses more disk (about 5-10 MB per system per year).
									</Trans>
								</p>
								<p className="text-xs text-muted-foreground mt-1">
									<Trans>Overridden by BESZEL_HUB_RETENTION env var if set. Requires hub restart to apply env change.</Trans>
								</p>
							</div>
							<div className="grid sm:grid-cols-3 gap-4 items-end">
								<div className="grid gap-2">
									<Label className="block" htmlFor="retention">
										<Trans>Long-term retention</Trans>
									</Label>
									<Select value={retention} onValueChange={setRetention} disabled={retentionLoading}>
										<SelectTrigger id="retention">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{retentionOptions.map((opt) => (
												<SelectItem key={opt.value} value={opt.value}>
													{opt.label}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</div>
								<div className="flex items-end">
									<Button type="button" onClick={handleRetentionSave} disabled={retentionSaving || retentionLoading} className="flex items-center gap-1.5">
										{retentionSaving ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : <SaveIcon className="h-4 w-4" />}
										<Trans>Save Retention</Trans>
									</Button>
								</div>
							</div>
						</div>
					</>
				)}
				<Separator />
				<div className="grid gap-2">
					<div className="mb-2">
						<h3 className="mb-1 text-lg font-medium">
							<Trans comment="Temperature / network units">Unit preferences</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Change display units for metrics.</Trans>
						</p>
					</div>
					<div className="grid sm:grid-cols-3 gap-4">
						<div className="grid gap-2">
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
						<div className="grid gap-2">
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
						<div className="grid gap-2">
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
				<div className="grid gap-2">
					<div className="mb-2">
						<h3 className="mb-1 text-lg font-medium">
							<Trans>Warning thresholds</Trans>
						</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							<Trans>Set percentage thresholds for meter colors.</Trans>
						</p>
					</div>
					<div className="grid grid-cols-2 lg:grid-cols-3 gap-4 items-end">
						<div className="grid gap-2">
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
						<div className="grid gap-1">
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
