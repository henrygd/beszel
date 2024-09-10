import { Suspense, lazy, useEffect } from 'react'
import { Separator } from '../../ui/separator'
import { SidebarNav } from './sidebar-nav.tsx'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card.tsx'
import { useStore } from '@nanostores/react'
import { $router } from '@/components/router.tsx'
import { redirectPage } from '@nanostores/router'
import { BellIcon, SettingsIcon } from 'lucide-react'

const General = lazy(() => import('./general.tsx'))
const Notifications = lazy(() => import('./notifications.tsx'))

const sidebarNavItems = [
	{
		title: 'General',
		href: '/settings/general',
		icon: SettingsIcon,
	},
	{
		title: 'Notifications',
		href: '/settings/notifications',
		icon: BellIcon,
	},
]

export default function SettingsLayout() {
	const page = useStore($router)

	useEffect(() => {
		document.title = 'Settings / Beszel'
		// redirect to account page if no page is specified
		if (page?.path === '/settings') {
			redirectPage($router, 'settings', { name: 'general' })
		}
	}, [])

	return (
		<Card className="pt-5 px-4 pb-9 sm:pt-6 sm:px-7">
			<CardHeader className="p-0">
				<CardTitle className="mb-1">Settings</CardTitle>
				<CardDescription>Manage your account settings and set e-mail preferences.</CardDescription>
			</CardHeader>
			<CardContent className="p-0">
				<Separator className="my-5" />
				<div className="flex flex-col space-y-8 lg:flex-row lg:space-x-12 lg:space-y-0">
					<aside className="lg:w-48 w-full overflow-auto">
						<SidebarNav items={sidebarNavItems} />
					</aside>
					<div className="flex-1">
						<Suspense>
							{/* @ts-ignore */}
							<SettingsContent name={page?.params?.name ?? 'general'} />
						</Suspense>
					</div>
				</div>
			</CardContent>
		</Card>
	)
}

function SettingsContent({ name }: { name: string }) {
	switch (name) {
		case 'general':
			return <General />
		// case 'display':
		// 	return <Display />
		case 'notifications':
			return <Notifications />
	}
	return ''
}
