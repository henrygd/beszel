import * as React from "react"
import { ChevronDownIcon, HourglassIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { Button } from "./button"

interface CollapsibleProps {
	title: string
	children: React.ReactNode
	description?: React.ReactNode
	defaultOpen?: boolean
	className?: string
	icon?: React.ReactNode
}

export function Collapsible({ title, children, description, defaultOpen = false, className, icon }: CollapsibleProps) {
	const [isOpen, setIsOpen] = React.useState(defaultOpen)

	return (
		<div className={cn("border rounded-lg", className)}>
			<Button
				variant="ghost"
				className="w-full justify-between p-4 font-semibold"
				onClick={() => setIsOpen(!isOpen)}
			>
				<div className="flex items-center gap-2">
					{icon}
					{title}
				</div>
				<ChevronDownIcon
					className={cn("h-4 w-4 transition-transform duration-200", {
						"rotate-180": isOpen,
					})}
				/>
			</Button>
			{description && (
				<div className="px-4 pb-2 text-sm text-muted-foreground">
					{description}
				</div>
			)}
			{isOpen && (
				<div className="px-4 pb-4">
					<div className="grid gap-3">
						{children}
					</div>
				</div>
			)}
		</div>
	)
} 