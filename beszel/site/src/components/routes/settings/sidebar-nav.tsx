import React from "react"
import { cn, isAdmin } from "@/lib/utils"
import { buttonVariants } from "../../ui/button"
import { $router, Link, navigate } from "../../router"
import { useStore } from "@nanostores/react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"

interface SidebarNavProps extends React.HTMLAttributes<HTMLElement> {
	items: {
		href: string
		title: string
		icon?: React.FC<React.SVGProps<SVGSVGElement>>
		admin?: boolean
	}[]
}

export function SidebarNav({ className, items, ...props }: SidebarNavProps) {
	const page = useStore($router)

	return (
		<>
			{/* Mobile View */}
			<div className="md:hidden">
				<Select onValueChange={navigate} value={page?.path}>
					<SelectTrigger className="w-full my-3.5">
						<SelectValue placeholder="Select page" />
					</SelectTrigger>
					<SelectContent>
						{items.map((item) => {
							if (item.admin && !isAdmin()) return null
							return (
								<SelectItem key={item.href} value={item.href}>
									<span className="flex items-center gap-2">
										{item.icon && <item.icon className="h-4 w-4" />}
										{item.title}
									</span>
								</SelectItem>
							)
						})}
					</SelectContent>
				</Select>
				<Separator />
			</div>

			{/* Desktop View */}
			<nav className={cn("hidden md:grid gap-1", className)} {...props}>
				{items.map((item) => {
					if (item.admin && !isAdmin()) {
						return null
					}
					return (
						<Link
							key={item.href}
							href={item.href}
							className={cn(
								buttonVariants({ variant: "ghost" }),
								"flex items-center gap-3",
								page?.path === item.href ? "bg-muted hover:bg-muted" : "hover:bg-muted/50",
								"justify-start"
							)}
						>
							{item.icon && <item.icon className="h-4 w-4" />}
							{item.title}
						</Link>
					)
				})}
			</nav>
		</>
	)
}
