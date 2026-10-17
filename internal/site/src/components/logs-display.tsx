/** biome-ignore-all lint/security/noDangerouslySetInnerHtml: log HTML is generated locally by Shiki */
import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { ClockIcon, RefreshCwIcon } from "lucide-react"
import { type RefObject, useEffect, useRef } from "react"
import { IconButton, type IconButtonProps } from "@/components/ui/icon-button"
import { FullscreenContentDialog } from "@/components/ui/fullscreen-content-dialog"
import { $showLogTimestamps, toggleLogTimestamps } from "@/lib/stores"
import { cn } from "@/lib/utils"

type LogsDisplayProps = {
	logsDisplay: string
	containerRef: RefObject<HTMLDivElement | null>
}

// Timestamp visibility is a single persisted preference shared by all log views.
export function LogsTimestampToggle({
	className,
	onDarkBackground,
}: Pick<IconButtonProps, "className" | "onDarkBackground">) {
	const showTimestamps = useStore($showLogTimestamps)
	return (
		<IconButton
			label={showTimestamps ? t`Hide timestamps` : t`Show timestamps`}
			onClick={toggleLogTimestamps}
			onDarkBackground={onDarkBackground}
			className={cn(showTimestamps && !onDarkBackground && "bg-accent text-accent-foreground", className)}
			aria-label={t`Show timestamps`}
			aria-pressed={showTimestamps}
		>
			<ClockIcon className="size-4" />
		</IconButton>
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
		<FullscreenContentDialog
			open={open}
			onOpenChange={onOpenChange}
			title={`${name} logs`}
			scrollRef={outerContainerRef}
			contentClassName={!showTimestamps ? "[&_.log-timestamp]:hidden" : undefined}
			toolbar={
				<>
					<LogsTimestampToggle onDarkBackground />
					<IconButton label={t`Refresh`} onClick={onRefresh} disabled={isRefreshing} onDarkBackground>
						<RefreshCwIcon className={cn("size-4 transition-transform duration-300", isRefreshing && "animate-spin")} />
					</IconButton>
				</>
			}
		>
			<div dangerouslySetInnerHTML={{ __html: logsDisplay }} />
		</FullscreenContentDialog>
	)
}
