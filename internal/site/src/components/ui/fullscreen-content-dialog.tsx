import { t } from "@lingui/core/macro"
import { XIcon } from "lucide-react"
import type { ReactNode, Ref } from "react"
import { Dialog, DialogClose, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { IconButton } from "@/components/ui/icon-button"
import { cn } from "@/lib/utils"

export function FullscreenContentDialog({
	open,
	onOpenChange,
	title,
	children,
	toolbar,
	scrollRef,
	contentClassName,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	title: string
	children: ReactNode
	toolbar?: ReactNode
	scrollRef?: Ref<HTMLDivElement>
	contentClassName?: string
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				aria-describedby={undefined}
				showCloseButton={false}
				className="w-[calc(100vw-20px)] h-[calc(100dvh-20px)] max-w-none p-0 bg-gh-dark border-0 text-white"
			>
				<DialogTitle className="sr-only">{title}</DialogTitle>
				<div ref={scrollRef} className="absolute inset-x-0 bottom-0 top-12 overflow-auto">
					<div className={cn("min-h-full w-full p-3 leading-relaxed rounded-md bg-gh-dark text-sm", contentClassName)}>
						{children}
					</div>
				</div>
				{toolbar && <div className="absolute end-10 top-2 flex items-center">{toolbar}</div>}
				<IconButton label={t`Dismiss`} onDarkBackground className="absolute end-2 top-2" asChild>
					<DialogClose>
						<XIcon className="size-4" />
					</DialogClose>
				</IconButton>
			</DialogContent>
		</Dialog>
	)
}
