/** biome-ignore-all lint/correctness/useUniqueElementIds: component is only rendered once */
import { Trans, useLingui } from "@lingui/react/macro"
import { DownloadCloudIcon, LanguagesIcon, LoaderCircleIcon, RefreshCcwIcon, SaveIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { useStore } from "@nanostores/react"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
	AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { toast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import Slider from "@/components/ui/slider"
import { HourFormat, Unit } from "@/lib/enums"
import { dynamicActivate } from "@/lib/i18n"
import languages from "@/lib/languages"
import { $chartTime, $newVersion, $userSettings, defaultLayoutWidth, getUserChartTime } from "@/lib/stores"
import { chartTimeData, currentHour12 } from "@/lib/utils"
import type { UpdateInfo, UserSettings } from "@/types"
import { saveSettings } from "./layout"

export default function SettingsProfilePage({ userSettings }: { userSettings: UserSettings }) {
	const [isLoading, setIsLoading] = useState(false)
	const { i18n } = useLingui()
	const currentUserSettings = useStore($userSettings)
	const layoutWidth = currentUserSettings.layoutWidth ?? defaultLayoutWidth
	// without a value the hidden select submits an empty string, which would persist
	// a chart time that no longer loads any data (#2104)
	const chartTime = getUserChartTime(userSettings)

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault()
		setIsLoading(true)
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Partial<UserSettings>
		await saveSettings(data)
		// apply the saved default time period to the active charts
		$chartTime.set(getUserChartTime())
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
			<AppUpdatePanel />
			<Separator className="my-5" />
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
					<Select name="lang" value={i18n.locale} onValueChange={(lang: string) => dynamicActivate(lang)}>
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
							<Select name="chartTime" key={chartTime} defaultValue={chartTime}>
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

function AppUpdatePanel() {
	const updateInfo = useStore($newVersion)
	const [checking, setChecking] = useState(false)
	const [applying, setApplying] = useState(false)
	const [restartPending, setRestartPending] = useState(false)

	async function refreshUpdateInfo() {
		setChecking(true)
		try {
			const info = await pb.send<UpdateInfo>("/api/beszel/update", {})
			$newVersion.set(info)
		} catch (err) {
			toast({
				title: "Update check failed",
				description: err instanceof Error ? err.message : "Could not check for updates.",
				variant: "destructive",
			})
		} finally {
			setChecking(false)
		}
	}

	async function applyUpdate() {
		setApplying(true)
		try {
			const res = await pb.send<{ message: string }>("/api/beszel/update/apply", { method: "POST" })
			toast({
				title: "Update started",
				description: res.message,
			})
			setRestartPending(true)
			$newVersion.set(updateInfo ? { ...updateInfo, status: "updating", message: res.message } : undefined)
			const restarted = await waitForRestartAndReload()
			if (!restarted) {
				toast({
					title: "Still waiting for restart",
					description: "Beszel did not come back before the timeout. Check the Docker container logs.",
					variant: "destructive",
				})
				setRestartPending(false)
				setApplying(false)
				await refreshUpdateInfo()
			}
		} catch (err) {
			toast({
				title: "Update failed",
				description: err instanceof Error ? err.message : "Could not start the update.",
				variant: "destructive",
			})
			setApplying(false)
		}
	}

	useEffect(() => {
		if (!updateInfo) {
			refreshUpdateInfo()
		}
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [])

	const status = updateInfo?.status ?? "checking"
	const message = restartPending
		? "Update started. Waiting for Beszel to restart..."
		: (updateInfo?.message ?? "Checking GHCR for the latest image.")
	const canUpdate = Boolean(updateInfo?.canApply && updateInfo?.updateAvailable && !applying && !restartPending)

	return (
		<div className="rounded-md border bg-card/50 p-4">
			<div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
				<div className="space-y-1.5">
					<h3 className="text-lg font-medium flex items-center gap-2">
						<DownloadCloudIcon className="h-4 w-4" />
						<Trans>App update</Trans>
					</h3>
					<p className="text-sm text-muted-foreground leading-relaxed">{message}</p>
				</div>
				<StatusBadge status={status} updateAvailable={Boolean(updateInfo?.updateAvailable)} />
			</div>
			<div className="mt-4 grid gap-2 text-sm sm:grid-cols-2">
				<UpdateMeta label="Image" value={updateInfo?.image ?? "ghcr.io/dvorinka/beszel:latest"} />
				<UpdateMeta label="Current version" value={updateInfo?.currentVersion ?? "..."} />
				<UpdateMeta label="Running digest" value={shortDigest(updateInfo?.currentDigest)} />
				<UpdateMeta label="Latest digest" value={shortDigest(updateInfo?.latestDigest)} />
			</div>
			<div className="mt-4 flex flex-col gap-2 sm:flex-row">
				<Button
					type="button"
					variant="outline"
					onClick={refreshUpdateInfo}
					disabled={checking || applying || restartPending}
				>
					{checking ? (
						<LoaderCircleIcon className="me-2 h-4 w-4 animate-spin" />
					) : (
						<RefreshCcwIcon className="me-2 h-4 w-4" />
					)}
					<Trans>Check now</Trans>
				</Button>
				<AlertDialog>
					<AlertDialogTrigger asChild>
						<Button type="button" disabled={!canUpdate}>
							{applying || restartPending ? (
								<LoaderCircleIcon className="me-2 h-4 w-4 animate-spin" />
							) : (
								<DownloadCloudIcon className="me-2 h-4 w-4" />
							)}
							{restartPending ? <Trans>Restarting</Trans> : <Trans>Update now</Trans>}
						</Button>
					</AlertDialogTrigger>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>
								<Trans>Update Beszel now?</Trans>
							</AlertDialogTitle>
							<AlertDialogDescription>
								<Trans>
									Beszel will pull ghcr.io/dvorinka/beszel:latest, recreate the running container, and restart the app.
									All signed-in users can start this action.
								</Trans>
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>
								<Trans>Cancel</Trans>
							</AlertDialogCancel>
							<AlertDialogAction onClick={applyUpdate}>
								<Trans>Start update</Trans>
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</div>
		</div>
	)
}

function StatusBadge({ status, updateAvailable }: { status: string; updateAvailable: boolean }) {
	const label = updateAvailable
		? "Update available"
		: status === "up-to-date"
			? "Up to date"
			: status.replaceAll("-", " ")
	return (
		<span className="inline-flex h-7 items-center self-start rounded-md border bg-background px-2.5 text-xs font-medium capitalize text-muted-foreground">
			{label}
		</span>
	)
}

function UpdateMeta({ label, value }: { label: string; value: string }) {
	return (
		<div className="min-w-0 rounded-md bg-muted/45 px-3 py-2">
			<div className="text-xs text-muted-foreground">{label}</div>
			<div className="truncate font-mono text-xs">{value}</div>
		</div>
	)
}

function shortDigest(value?: string) {
	if (!value) return "unknown"
	const digest = value.includes("@") ? value.split("@").at(-1) : value
	if (!digest) return value
	return digest.length > 24 ? `${digest.slice(0, 24)}...` : digest
}

async function waitForRestartAndReload() {
	await sleep(10_000)
	for (let attempt = 0; attempt < 45; attempt++) {
		try {
			const res = await fetch("/api/health", { cache: "no-store" })
			if (res.ok) {
				window.location.reload()
				return true
			}
		} catch {
			// Hub is expected to be unavailable while Docker replaces the container.
		}
		await sleep(2_000)
	}
	return false
}

function sleep(ms: number) {
	return new Promise((resolve) => window.setTimeout(resolve, ms))
}
