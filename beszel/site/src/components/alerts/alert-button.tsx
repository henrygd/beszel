import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { memo, useMemo, useState } from "react"
import { useStore } from "@nanostores/react"
import { $alerts } from "@/lib/stores"
import {
	Dialog,
	DialogTrigger,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { BellIcon, GlobeIcon, ServerIcon } from "lucide-react"
import { alertInfo, cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { AlertRecord, SystemRecord } from "@/types"
import { $router, Link } from "../router"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Checkbox } from "../ui/checkbox"
import { SystemAlert, SystemAlertGlobal } from "./alerts-system"
import { getPagePath } from "@nanostores/router"

export default memo(function AlertsButton({ system }: { system: SystemRecord }) {
	const alerts = useStore($alerts)
	const [opened, setOpened] = useState(false)

	const hasAlert = alerts.some((alert) => alert.system === system.id)

	return useMemo(
		() => (
			<Dialog>
				<DialogTrigger asChild>
					<Button variant="ghost" size="icon" aria-label={t`Alerts`} data-nolink onClick={() => setOpened(true)}>
						<BellIcon
							className={cn("h-[1.2em] w-[1.2em] pointer-events-none", {
								"fill-primary": hasAlert,
							})}
						/>
					</Button>
				</DialogTrigger>
				<DialogContent className="max-h-full sm:max-h-[95svh] overflow-auto max-w-[37rem]">
					{opened && <AlertDialogContent system={system} />}
				</DialogContent>
			</Dialog>
		),
		[opened, hasAlert]
	)

	// return useMemo(
	// 	() => (
	// 		<Sheet>
	// 			<SheetTrigger asChild>
	// 				<Button variant="ghost" size="icon" aria-label={t`Alerts`} data-nolink onClick={() => setOpened(true)}>
	// 					<BellIcon
	// 						className={cn("h-[1.2em] w-[1.2em] pointer-events-none", {
	// 							"fill-primary": hasAlert,
	// 						})}
	// 					/>
	// 				</Button>
	// 			</SheetTrigger>
	// 			<SheetContent className="max-h-full overflow-auto w-[35em] p-4 sm:p-5">
	// 				{opened && <AlertDialogContent system={system} />}
	// 			</SheetContent>
	// 		</Sheet>
	// 	),
	// 	[opened, hasAlert]
	// )
})

function AlertDialogContent({ system }: { system: SystemRecord }) {
	const alerts = useStore($alerts)
	const [overwriteExisting, setOverwriteExisting] = useState<boolean | "indeterminate">(false)

	/* key to prevent re-rendering */
	const alertsSignature: string[] = []

	const systemAlerts = alerts.filter((alert) => {
		if (alert.system === system.id) {
			alertsSignature.push(alert.name, alert.min, alert.value)
			return true
		}
		return false
	}) as AlertRecord[]

	return useMemo(() => {
		const data = Object.keys(alertInfo).map((name) => {
			const alert = alertInfo[name as keyof typeof alertInfo]
			return {
				name: name as keyof typeof alertInfo,
				alert,
				system,
			}
		})

		return (
			<>
				<DialogHeader>
					<DialogTitle className="text-xl">
						<Trans>Alerts</Trans>
					</DialogTitle>
					<DialogDescription>
						<Trans>
							See{" "}
							<Link href={getPagePath($router, "settings", { name: "notifications" })} className="link">
								notification settings
							</Link>{" "}
							to configure how you receive alerts.
						</Trans>
					</DialogDescription>
				</DialogHeader>
				<Tabs defaultValue="system">
					<TabsList className="mb-1 -mt-0.5">
						<TabsTrigger value="system">
							<ServerIcon className="me-2 h-3.5 w-3.5" />
							{system.name}
						</TabsTrigger>
						<TabsTrigger value="global">
							<GlobeIcon className="me-1.5 h-3.5 w-3.5" />
							<Trans>All Systems</Trans>
						</TabsTrigger>
					</TabsList>
					<TabsContent value="system">
						<div className="grid gap-3">
							{data.map((d) => (
								<SystemAlert key={d.name} system={system} data={d} systemAlerts={systemAlerts} />
							))}
						</div>
					</TabsContent>
					<TabsContent value="global">
						<label
							htmlFor="ovw"
							className="mb-3 flex gap-2 items-center justify-center cursor-pointer border rounded-sm py-3 px-4 border-destructive text-destructive font-semibold text-sm"
						>
							<Checkbox
								id="ovw"
								className="text-destructive border-destructive data-[state=checked]:bg-destructive"
								checked={overwriteExisting}
								onCheckedChange={setOverwriteExisting}
							/>
							<Trans>Overwrite existing alerts</Trans>
						</label>
						<div className="grid gap-3">
							{data.map((d) => (
								<SystemAlertGlobal key={d.name} data={d} overwrite={overwriteExisting} />
							))}
						</div>
					</TabsContent>
				</Tabs>
			</>
		)
	}, [alertsSignature.join(""), overwriteExisting])
}
