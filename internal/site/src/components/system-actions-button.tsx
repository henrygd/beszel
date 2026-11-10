import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import {
	BellIcon,
	CopyIcon,
	MoreHorizontalIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	Trash2Icon,
} from "lucide-react"
import { memo, useMemo, useRef, useState } from "react"
import { SystemDialog } from "@/components/add-system"
import { AlertDialogContent as AlertsSheetContent, alertsSheetClassName } from "@/components/alerts/alerts-sheet"
import { $router, navigate } from "@/components/router"
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
import { Button, buttonVariants } from "@/components/ui/button"
import { Dialog } from "@/components/ui/dialog"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent } from "@/components/ui/sheet"
import { isReadOnlyUser, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import { $alerts } from "@/lib/stores"
import { cn, copyToClipboard } from "@/lib/utils"
import type { SystemRecord } from "@/types"

/** Menu item that opens the alerts sheet; the bell is filled when the system has alerts configured. */
const AlertsMenuItem = ({ systemId, onSelect }: { systemId: string; onSelect: () => void }) => {
	const alerts = useStore($alerts)
	const hasSystemAlert = alerts[systemId]?.size > 0
	return (
		<DropdownMenuItem onSelect={onSelect}>
			<BellIcon className={cn("me-2.5 size-4", { "fill-primary": hasSystemAlert })} />
			<Trans>Alerts</Trans>
		</DropdownMenuItem>
	)
}

async function deleteSystem({ id, name }: SystemRecord) {
	await pb.collection("systems").delete(id)
	// leave the dashboard of the system that was just deleted (route param may be the id or legacy name slug)
	const page = $router.get()
	if (page?.route === "system" && (page.params.id === id || page.params.id === name)) {
		navigate(getPagePath($router, "home"))
	}
}

type ActionsButtonProps = {
	system: SystemRecord
	variant?: "ghost" | "outline"
	/** Include an item that opens the alerts sheet (used where no separate alerts button is shown). */
	showAlerts?: boolean
}

export const ActionsButton = memo(({ system, variant = "ghost", showAlerts = false }: ActionsButtonProps) => {
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [editOpen, setEditOpen] = useState(false)
	const editOpened = useRef(false)
	const [alertsOpen, setAlertsOpen] = useState(false)
	const alertsOpened = useRef(false)
	const { t } = useLingui()
	const { id, status, host, name } = system

	return useMemo(() => {
		return (
			<>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant={variant} size={"icon"}>
							<span className="sr-only">
								<Trans>Open menu</Trans>
							</span>
							<MoreHorizontalIcon className="w-5" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end" className="min-w-38">
						{showAlerts && (
							<>
								<AlertsMenuItem
									systemId={id}
									onSelect={() => {
										alertsOpened.current = true
										setAlertsOpen(true)
									}}
								/>
								<DropdownMenuSeparator />
							</>
						)}
						{!isReadOnlyUser() && (
							<DropdownMenuItem
								onSelect={() => {
									editOpened.current = true
									setEditOpen(true)
								}}
							>
								<PenBoxIcon className="me-2.5 size-4" />
								<Trans>Edit</Trans>
							</DropdownMenuItem>
						)}
						<DropdownMenuItem
							className={cn(isReadOnlyUser() && "hidden")}
							onClick={() => {
								pb.collection("systems").update(id, {
									status: status === SystemStatus.Paused ? SystemStatus.Pending : SystemStatus.Paused,
								})
							}}
						>
							{status === SystemStatus.Paused ? (
								<>
									<PlayCircleIcon className="me-2.5 size-4" />
									<Trans>Resume</Trans>
								</>
							) : (
								<>
									<PauseCircleIcon className="me-2.5 size-4" />
									<Trans>Pause</Trans>
								</>
							)}
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(name)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy name</Trans>
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(host)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy host</Trans>
						</DropdownMenuItem>
						<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
						<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")} onSelect={() => setDeleteOpen(true)}>
							<Trash2Icon className="me-2.5 size-4" />
							<Trans>Delete</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				{/* alerts sheet */}
				{showAlerts && (
					<Sheet open={alertsOpen} onOpenChange={setAlertsOpen}>
						<SheetContent className={alertsSheetClassName}>
							{alertsOpened.current && <AlertsSheetContent system={system} />}
						</SheetContent>
					</Sheet>
				)}
				{/* edit dialog */}
				<Dialog open={editOpen} onOpenChange={setEditOpen}>
					{editOpened.current && <SystemDialog system={system} setOpen={setEditOpen} />}
				</Dialog>
				{/* deletion dialog */}
				<AlertDialog open={deleteOpen} onOpenChange={(open) => setDeleteOpen(open)}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>
								<Trans>Are you sure you want to delete {name}?</Trans>
							</AlertDialogTitle>
							<AlertDialogDescription>
								<Trans>
									This action cannot be undone. This will permanently delete all current records for {name} from the
									database.
								</Trans>
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>
								<Trans>Cancel</Trans>
							</AlertDialogCancel>
							<AlertDialogAction
								className={cn(buttonVariants({ variant: "destructive" }))}
								onClick={() => deleteSystem(system)}
							>
								<Trans>Continue</Trans>
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</>
		)
	}, [id, status, host, name, system, t, deleteOpen, editOpen, alertsOpen, variant, showAlerts])
})
