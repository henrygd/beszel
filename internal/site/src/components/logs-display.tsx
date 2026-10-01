/** biome-ignore-all lint/security/noDangerouslySetInnerHtml: log HTML is generated locally by Shiki */
import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { ClockIcon, RefreshCwIcon } from "lucide-react"
import { type ComponentProps, type FocusEvent, type RefObject, useEffect, useRef } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle, dialogIconButtonClassName } from "@/components/ui/dialog"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { $showLogTimestamps, toggleLogTimestamps } from "@/lib/stores"
import { cn } from "@/lib/utils"

type LogsDisplayProps = {
	logsDisplay: string
	containerRef: RefObject<HTMLDivElement | null>
}

// Radix opens a tooltip on any focus, including when a sheet or dialog moves focus to
// the button on open / close. Limit it to keyboard focus.
function keyboardFocusOnly(e: FocusEvent<HTMLButtonElement>) {
	if (!e.currentTarget.matches(":focus-visible")) {
		e.preventDefault()
	}
}

// Icon button with a tooltip, used in the log panel headers.
export function LogsIconButton({ label, className, ...props }: { label: string } & ComponentProps<typeof Button>) {
	return (
		<Tooltip>
			<TooltipTrigger asChild onFocus={keyboardFocusOnly}>
				<Button variant="ghost" size="sm" aria-label={label} className={cn("h-8 w-8 p-0", className)} {...props} />
			</TooltipTrigger>
			<TooltipContent>{label}</TooltipContent>
		</Tooltip>
	)
}

// Timestamp visibility is a single persisted preference shared by all log views.
export function LogsTimestampToggle({ className }: { className?: string }) {
	const showTimestamps = useStore($showLogTimestamps)
	return (
		<LogsIconButton
			label={showTimestamps ? t`Hide timestamps` : t`Show timestamps`}
			onClick={toggleLogTimestamps}
			className={cn(showTimestamps && "bg-accent text-accent-foreground", className)}
			aria-label={t`Show timestamps`}
			aria-pressed={showTimestamps}
		>
			<ClockIcon className="size-4" />
		</LogsIconButton>
	)
}

// Shared by Docker and systemd service sheets so logs behave identically.
export function LogsDisplay({ logsDisplay, containerRef }: LogsDisplayProps) {
	const showTimestamps = useStore($showLogTimestamps)
	return (
		<div
			ref={containerRef}
			className={cn(
				"max-h-[calc(50dvh-10rem)] w-full overflow-auto p-3 rounded-md bg-gh-dark text-white text-sm",
				!showTimestamps && "[&_.log-timestamp]:hidden",
				!logsDisplay && ["animate-pulse", "h-full"]
			)}
		>
			<div dangerouslySetInnerHTML={{ __html: logsDisplay }} />
		</div>
	)
}

export function LogsFullscreenDialog({
	open,
	onOpenChange,
	logsDisplay,
	name,
	onRefresh,
	isRefreshing,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	logsDisplay: string
	name: string
	onRefresh: () => void | Promise<void>
	isRefreshing: boolean
}) {
	const showTimestamps = useStore($showLogTimestamps)
	const outerContainerRef = useRef<HTMLDivElement>(null)

	useEffect(() => {
		if (open && logsDisplay) {
			setTimeout(() => {
				if (outerContainerRef.current) {
					outerContainerRef.current.scrollTop = outerContainerRef.current.scrollHeight
				}
			}, 50)
		}
	}, [open, logsDisplay])

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="w-[calc(100vw-20px)] h-[calc(100dvh-20px)] max-w-none p-0 bg-gh-dark border-0 text-white">
				<DialogTitle className="sr-only">{name} logs</DialogTitle>
				<div ref={outerContainerRef} className="absolute inset-x-0 bottom-0 top-12 overflow-auto">
					<div
						className={cn(
							"min-h-full w-full px-3 leading-relaxed rounded-md bg-gh-dark text-sm",
							!showTimestamps && "[&_.log-timestamp]:hidden"
						)}
					>
						<div className="py-3" dangerouslySetInnerHTML={{ __html: logsDisplay }} />
					</div>
				</div>
				<LogsTimestampToggle className="absolute end-18 top-2 hover:bg-white/10 hover:text-white aria-pressed:bg-white/15 aria-pressed:text-white" />
				<Tooltip>
					<TooltipTrigger asChild onFocus={keyboardFocusOnly}>
						<button
							onClick={onRefresh}
							className={cn("absolute end-11 top-3 opacity-60 hover:opacity-100", dialogIconButtonClassName)}
							disabled={isRefreshing}
							aria-label={t`Refresh`}
						>
							<RefreshCwIcon
								className={cn("size-4 transition-transform duration-300", isRefreshing && "animate-spin")}
							/>
						</button>
					</TooltipTrigger>
					<TooltipContent>{t`Refresh`}</TooltipContent>
				</Tooltip>
			</DialogContent>
		</Dialog>
	)
}
