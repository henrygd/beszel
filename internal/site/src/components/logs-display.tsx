/** biome-ignore-all lint/security/noDangerouslySetInnerHtml: log HTML is generated locally by Shiki */
import { t } from "@lingui/core/macro"
import { RefreshCwIcon } from "lucide-react"
import { type RefObject, useEffect, useRef } from "react"
import { Dialog, DialogContent, DialogTitle, dialogIconButtonClassName } from "@/components/ui/dialog"
import { cn } from "@/lib/utils"

type LogsDisplayProps = {
	logsDisplay: string
	containerRef: RefObject<HTMLDivElement | null>
}

// Shared by Docker and systemd service sheets so logs behave identically.
export function LogsDisplay({ logsDisplay, containerRef }: LogsDisplayProps) {
	return (
		<div
			ref={containerRef}
			className={cn(
				"max-h-[calc(50dvh-10rem)] w-full overflow-auto p-3 rounded-md bg-gh-dark text-white text-sm",
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
					<div className="min-h-full w-full px-3 leading-relaxed rounded-md bg-gh-dark text-sm">
						<div className="py-3" dangerouslySetInnerHTML={{ __html: logsDisplay }} />
					</div>
				</div>
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
