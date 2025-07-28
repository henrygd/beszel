import React from "react"
import { cn, isAdmin, isReadOnlyUser } from "@/lib/utils"
import { buttonVariants } from "../../ui/button"
import { $router, Link, navigate } from "../../router"
import { useStore } from "@nanostores/react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"

interface SidebarNavProps extends React.HTMLAttributes<HTMLElement> {
	items?: {
		href: string
		title: string
		icon?: React.FC<React.SVGProps<SVGSVGElement>>
		admin?: boolean
		noReadOnly?: boolean
	}[]
	sections?: {
		title: string
		items: {
			href: string
			title: string
			icon?: React.FC<React.SVGProps<SVGSVGElement>>
			admin?: boolean
			noReadOnly?: boolean
		}[]
	}[]
}

export function SidebarNav({ className, items, sections, ...props }: SidebarNavProps) {
	const page = useStore($router)

	// Flatten all items for mobile view
	const allItems = sections ? sections.flatMap(section => section.items) : (items || [])

	return (
		<>
			{/* Mobile View */}
			<div className="md:hidden">
				<Select onValueChange={navigate} value={page?.path}>
					<SelectTrigger className="w-full my-3.5">
						<SelectValue placeholder="Select page" />
					</SelectTrigger>
					<SelectContent>
						{allItems.map((item) => {
							if (item.admin && !isAdmin()) return null
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
			<nav className={cn("hidden md:grid gap-1 sticky top-6", className)} {...props}>
				{sections ? (
					sections.map((section, sectionIndex) => (
						<div key={section.title}>
							{sectionIndex > 0 && <Separator className="my-2" />}
							<div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
								{section.title}
							</div>
							{section.items.map((item) => {
								if ((item.admin && !isAdmin()) || (item.noReadOnly && isReadOnlyUser())) {
									return null
								}
								return (
									<Link
										key={item.href}
										href={item.href}
										className={cn(
											buttonVariants({ variant: "ghost" }),
											"flex items-center gap-3 justify-start truncate",
											page?.path === item.href ? "bg-muted hover:bg-muted" : "hover:bg-muted/50"
										)}
									>
										{item.icon && <item.icon className="size-4 shrink-0" />}
										<span className="truncate">{item.title}</span>
									</Link>
								)
							})}
						</div>
					))
				) : (
					items?.map((item) => {
						if ((item.admin && !isAdmin()) || (item.noReadOnly && isReadOnlyUser())) {
							return null
						}
						return (
							<Link
								key={item.href}
								href={item.href}
								className={cn(
									buttonVariants({ variant: "ghost" }),
									"flex items-center gap-3 justify-start truncate",
									page?.path === item.href ? "bg-muted hover:bg-muted" : "hover:bg-muted/50"
								)}
							>
								{item.icon && <item.icon className="size-4 shrink-0" />}
								<span className="truncate">{item.title}</span>
							</Link>
						)
					})
				)}
			</nav>
		</>
	)
}
