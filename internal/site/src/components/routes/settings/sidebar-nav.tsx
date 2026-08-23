import { useStore } from "@nanostores/react"
import type React from "react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { isAdmin, isReadOnlyUser } from "@/lib/api"
import { cn } from "@/lib/utils"
import { $router, Link, navigate } from "../../router"

interface SidebarNavProps extends React.HTMLAttributes<HTMLElement> {
	items: {
		href: string
		title: string
		icon?: React.FC<React.SVGProps<SVGSVGElement>>
		admin?: boolean
		noReadOnly?: boolean
		preload?: () => Promise<unknown>
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
							if ((item.admin && !isAdmin()) || (item.noReadOnly && isReadOnlyUser())) return null
							return (
								<SelectItem key={item.href} value={item.href}>
									<span className="flex items-center gap-2 truncate">
										{item.icon && <item.icon className="size-4" />}
										<span className="truncate">{item.title}</span>
									</span>
								</SelectItem>
							)
						})}
					</SelectContent>
				</Select>
				<Separator />
			</div>

			{/* Desktop View */}
			<nav className={cn("hidden md:grid gap-0.5 sticky top-6", className)} {...props}>
				{items.map((item) => {
					if ((item.admin && !isAdmin()) || (item.noReadOnly && isReadOnlyUser())) {
						return null
					}
					const active = page?.path === item.href
					return (
						<Link
							onMouseEnter={() => item.preload?.()}
							key={item.href}
							href={item.href}
							className={cn(
								"relative flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
								active
									? "bg-accent text-accent-foreground"
									: "text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground"
							)}
							aria-current={active ? "page" : undefined}
						>
							{active && (
								<span
									aria-hidden="true"
									className="absolute start-0 top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-full bg-primary"
								/>
							)}
							{item.icon && <item.icon className="size-4 shrink-0" strokeWidth={1.75} />}
							<span className="truncate">{item.title}</span>
						</Link>
					)
				})}
			</nav>
		</>
	)
}
