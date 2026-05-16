import { useStore } from "@nanostores/react"
import type React from "react"
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { isAdmin } from "@/lib/api"
import { cn } from "@/lib/utils"
import { $router, Link, navigate } from "../../router"
import { buttonVariants } from "../../ui/button"

export interface NavItem {
	href: string
	title: string
	icon?: React.FC<React.SVGProps<SVGSVGElement>>
	admin?: boolean
	group?: string
	preload?: () => Promise<{ default: React.ComponentType<any> }>
}

interface SidebarNavProps extends React.HTMLAttributes<HTMLElement> {
	items: NavItem[]
}

export function SidebarNav({ className, items, ...props }: SidebarNavProps) {
	const page = useStore($router)

	const visibleItems = items.filter((item) => !item.admin || isAdmin())

	// Group items by their group label, preserving order
	const groups: { label: string | undefined; items: NavItem[] }[] = []
	for (const item of visibleItems) {
		const last = groups[groups.length - 1]
		if (!last || last.label !== item.group) {
			groups.push({ label: item.group, items: [item] })
		} else {
			last.items.push(item)
		}
	}

	return (
		<>
			{/* Mobile View */}
			<div className="md:hidden">
				<Select onValueChange={navigate} value={page?.path}>
					<SelectTrigger className="w-full my-3.5">
						<SelectValue placeholder="Select page" />
					</SelectTrigger>
					<SelectContent>
						{groups.map((group) =>
							group.label ? (
								<SelectGroup key={group.label}>
									<SelectLabel>{group.label}</SelectLabel>
									{group.items.map((item) => (
										<SelectItem key={item.href} value={item.href}>
											<span className="flex items-center gap-2 truncate">
												{item.icon && <item.icon className="size-4" />}
												<span className="truncate">{item.title}</span>
											</span>
										</SelectItem>
									))}
								</SelectGroup>
							) : (
								group.items.map((item) => (
									<SelectItem key={item.href} value={item.href}>
										<span className="flex items-center gap-2 truncate">
											{item.icon && <item.icon className="size-4" />}
											<span className="truncate">{item.title}</span>
										</span>
									</SelectItem>
								))
							)
						)}
					</SelectContent>
				</Select>
				<Separator />
			</div>

			{/* Desktop View */}
			<nav className={cn("hidden md:grid gap-1 sticky top-6", className)} {...props}>
				{groups.map((group, i) => (
					<div key={group.label ?? i} className="grid gap-1">
						{group.label && (
							<p className={cn("px-3 text-xs font-medium text-muted-foreground tracking-wide uppercase truncate", i > 0 && "mt-3")}>
								{group.label}
							</p>
						)}
						{group.items.map((item) => (
							<Link
								onMouseEnter={() => item.preload?.()}
								key={item.href}
								href={item.href}
								className={cn(
									buttonVariants({ variant: "ghost" }),
									"flex items-center gap-3 justify-start truncate duration-50",
									page?.path === item.href ? "bg-muted hover:bg-accent/70" : "hover:bg-accent/50"
								)}
							>
								{item.icon && <item.icon className="size-4 shrink-0" />}
								<span className="truncate">{item.title}</span>
							</Link>
						))}
					</div>
				))}
			</nav>
		</>
	)
}
