import { Trans } from "@lingui/react/macro"
import { ChevronRightIcon, type LucideIcon, TriangleAlertIcon } from "lucide-react"
import type { ReactNode } from "react"
import { cn } from "@/lib/utils"
import { Link } from "./router"
import { Alert, AlertDescription, AlertTitle } from "./ui/alert"
import { Button } from "./ui/button"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "./ui/sheet"

/**
 * Destructive alert banner with a button that opens a sheet containing details.
 * Content agnostic so it can be reused for system alerts, network monitors, etc.
 */
export function AlertBannerSheet({
	open,
	onOpenChange,
	title,
	description,
	buttonLabel,
	sheetTitle,
	sheetDescription,
	icon: Icon = TriangleAlertIcon,
	className,
	children,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	title: ReactNode
	description?: ReactNode
	buttonLabel?: ReactNode
	sheetTitle: ReactNode
	sheetDescription?: ReactNode
	icon?: LucideIcon
	className?: string
	children: ReactNode
}) {
	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<Alert variant="destructive" className={cn("flex items-center gap-3 py-3 max-sm:flex-wrap", className)}>
				<div className="flex items-center gap-3 flex-1 min-w-0">
					<Icon className="size-5 shrink-0" />
					<div className="min-w-0">
						<AlertTitle className="m-0">{title}</AlertTitle>
						{description && <AlertDescription className="text-destructive/80">{description}</AlertDescription>}
					</div>
				</div>
				<SheetTrigger asChild>
					<Button
						variant="outline"
						size="sm"
						className="shrink-0 bg-transparent border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive dark:hover:bg-destructive/15 max-sm:w-full"
					>
						{buttonLabel ?? <Trans>View details</Trans>}
						<ChevronRightIcon className="size-4 ms-1 -me-1" />
					</Button>
				</SheetTrigger>
			</Alert>
			<SheetContent className="w-120 !max-w-full gap-0">
				<SheetHeader className="p-4 sm:p-6 pb-3 sm:pb-4 border-b">
					<SheetTitle>{sheetTitle}</SheetTitle>
					{sheetDescription && <SheetDescription>{sheetDescription}</SheetDescription>}
				</SheetHeader>
				<div className="flex-1 overflow-auto p-4 sm:p-6 pt-3 sm:pt-4 flex flex-col gap-2.5">{children}</div>
			</SheetContent>
		</Sheet>
	)
}

/** Clickable row for use inside AlertBannerSheet. */
export function AlertBannerSheetItem({
	href,
	onClick,
	icon: Icon,
	title,
	description,
}: {
	href: string
	onClick?: () => void
	icon: LucideIcon | React.FC<{ className?: string }>
	title: ReactNode
	description?: ReactNode
}) {
	return (
		<Link
			href={href}
			onClick={onClick}
			className="group flex items-start gap-3 rounded-lg border p-3 transition-colors hover:bg-accent/60"
		>
			<div className="rounded-md bg-destructive/10 p-2 text-destructive shrink-0">
				<Icon className="size-4" />
			</div>
			<div className="min-w-0 flex-1">
				<div className="font-medium leading-tight truncate">{title}</div>
				{description && <div className="text-sm text-muted-foreground mt-1">{description}</div>}
			</div>
			<ChevronRightIcon className="size-4 self-center shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
		</Link>
	)
}
