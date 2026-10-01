/** biome-ignore-all lint/security/noDangerouslySetInnerHtml: log HTML is generated locally by Shiki */
import { t } from "@lingui/core/macro"
import { ClockIcon, RefreshCwIcon } from "lucide-react"
import { type RefObject, useEffect, useRef } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle, dialogIconButtonClassName } from "@/components/ui/dialog"
import { cn } from "@/lib/utils"

type LogsDisplayProps = {
	logsDisplay: string
	containerRef: RefObject<HTMLDivElement | null>
	showTimestamps?: boolean
}

export function LogsTimestampToggle({
	showTimestamps,
	onToggle,
	className,
}: {
	showTimestamps: boolean
	onToggle: () => void
	className?: string
}) {
	return (
		<Button
			variant="ghost"
			size="sm"
			onClick={onToggle}
			className={cn("h-8 w-8 p-0", showTimestamps && "bg-accent text-accent-foreground", className)}
			aria-label={t`Show timestamps`}
			aria-pressed={showTimestamps}
			title={showTimestamps ? t`Hide timestamps` : t`Show timestamps`}
		>
			<ClockIcon className="size-4" />
		</Button>
	)
}

// Shared by Docker and systemd service sheets so logs behave identically.
export function LogsDisplay({ logsDisplay, containerRef, showTimestamps = true }: LogsDisplayProps) {
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
	showTimestamps,
	onToggleTimestamps,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	logsDisplay: string
	name: string
	onRefresh: () => void | Promise<void>
	isRefreshing: boolean
	showTimestamps: boolean
	onToggleTimestamps: () => void
}) {
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
				<LogsTimestampToggle
					showTimestamps={showTimestamps}
					onToggle={onToggleTimestamps}
					className="absolute end-18 top-2 hover:bg-white/10 hover:text-white aria-pressed:bg-white/15 aria-pressed:text-white"
				/>
				<button
					onClick={onRefresh}
					className={cn("absolute end-11 top-3 opacity-60 hover:opacity-100", dialogIconButtonClassName)}
					disabled={isRefreshing}
					title={t`Refresh`}
					aria-label={t`Refresh`}
				>
					<RefreshCwIcon className={cn("size-4 transition-transform duration-300", isRefreshing && "animate-spin")} />
				</button>
			</DialogContent>
		</Dialog>
	)
}
