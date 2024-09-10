import { cn } from '@/lib/utils'
import { buttonVariants } from '../../ui/button'
import { $router, Link } from '../../router'
import { useStore } from '@nanostores/react'
import React from 'react'

interface SidebarNavProps extends React.HTMLAttributes<HTMLElement> {
	items: {
		href: string
		title: string
		icon?: React.FC<React.SVGProps<SVGSVGElement>>
	}[]
}

export function SidebarNav({ className, items, ...props }: SidebarNavProps) {
	const page = useStore($router)

	return (
		<nav
			className={cn('flex space-x-2 lg:flex-col lg:space-x-0 lg:space-y-1', className)}
			{...props}
		>
			{items.map((item) => (
				<Link
					key={item.href}
					href={item.href}
					className={cn(
						buttonVariants({ variant: 'ghost' }),
						'flex items-center gap-3',
						page?.path === item.href ? 'bg-muted hover:bg-muted' : 'hover:bg-muted/50',
						'justify-start'
					)}
				>
					{item.icon && <item.icon className="h-4 w-4" />}
					{item.title}
				</Link>
			))}
		</nav>
	)
}
