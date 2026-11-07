import type { ComponentProps, FocusEvent } from "react"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

// Radix opens a tooltip on any focus, including when a sheet or dialog moves focus to
// the button on open / close. Limit it to keyboard focus.
function keyboardFocusOnly(e: FocusEvent<HTMLButtonElement>) {
	if (!e.currentTarget.matches(":focus-visible")) {
		e.preventDefault()
	}
}

export type IconButtonProps = ComponentProps<typeof Button> & {
	label: string
	onDarkBackground?: boolean
	tooltipClassName?: string
}

export function IconButton({
	label,
	className,
	onDarkBackground = false,
	tooltipClassName,
	variant = "ghost",
	size = "sm",
	...props
}: IconButtonProps) {
	return (
		<Tooltip>
			<TooltipTrigger asChild onFocus={keyboardFocusOnly}>
				<Button
					variant={variant}
					size={size}
					aria-label={label}
					className={cn(
						"p-0",
						size === "sm" && "h-8 w-8",
						onDarkBackground &&
							"text-white hover:bg-white/10 hover:text-white aria-pressed:bg-white/15 aria-pressed:text-white",
						className
					)}
					{...props}
				/>
			</TooltipTrigger>
			<TooltipContent className={tooltipClassName}>{label}</TooltipContent>
		</Tooltip>
	)
}
