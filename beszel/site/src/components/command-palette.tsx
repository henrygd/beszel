import {
	DatabaseBackupIcon,
	Github,
	LayoutDashboard,
	LockKeyholeIcon,
	LogsIcon,
	MailIcon,
	Server,
	SettingsIcon,
	UsersIcon,
} from 'lucide-react'

import {
	CommandDialog,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
	CommandSeparator,
	CommandShortcut,
} from '@/components/ui/command'
import { useEffect, useState } from 'react'
import { useStore } from '@nanostores/react'
import { $systems } from '@/lib/stores'
import { isAdmin } from '@/lib/utils'
import { navigate } from './router'
import { useTranslation } from 'react-i18next'

export default function CommandPalette() {
	const { t } = useTranslation()

	const [open, setOpen] = useState(false)
	const systems = useStore($systems)

	useEffect(() => {
		const down = (e: KeyboardEvent) => {
			if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
				e.preventDefault()
				setOpen((open) => !open)
			}
		}

		document.addEventListener('keydown', down)
		return () => document.removeEventListener('keydown', down)
	}, [])

	return (
		<CommandDialog open={open} onOpenChange={setOpen}>
			<CommandInput placeholder={t('command.search')} />
			<CommandList>
				<CommandEmpty>No results found.</CommandEmpty>
				{systems.length > 0 && (
					<>
						<CommandGroup>
							{systems.map((system) => (
								<CommandItem
									key={system.id}
									onSelect={() => {
										navigate(`/system/${encodeURIComponent(system.name)}`)
										setOpen(false)
									}}
								>
									<Server className="mr-2 h-4 w-4" />
									<span>{system.name}</span>
									<CommandShortcut>{system.host}</CommandShortcut>
								</CommandItem>
							))}
						</CommandGroup>
						<CommandSeparator className="mb-1.5" />
					</>
				)}
				<CommandGroup heading={t('command.pages_settings')}>
					<CommandItem
						keywords={['home']}
						onSelect={() => {
							navigate('/')
							setOpen((open) => !open)
						}}
					>
						<LayoutDashboard className="mr-2 h-4 w-4" />
						<span>{t('command.dashboard')}</span>
						<CommandShortcut>{t('command.page')}</CommandShortcut>
					</CommandItem>
					<CommandItem
						onSelect={() => {
							navigate('/settings/general')
							setOpen((open) => !open)
						}}
					>
						<SettingsIcon className="mr-2 h-4 w-4" />
						<span>{t('settings.settings')}</span>
						<CommandShortcut>{t('settings.settings')}</CommandShortcut>
					</CommandItem>
					<CommandItem
						keywords={['alerts']}
						onSelect={() => {
							navigate('/settings/notifications')
							setOpen((open) => !open)
						}}
					>
						<MailIcon className="mr-2 h-4 w-4" />
						<span>{t('settings.notifications.title')}</span>
						<CommandShortcut>{t('settings.settings')}</CommandShortcut>
					</CommandItem>
					<CommandItem
						keywords={['github']}
						onSelect={() => {
							window.location.href = 'https://github.com/henrygd/beszel/blob/main/readme.md'
						}}
					>
						<Github className="mr-2 h-4 w-4" />
						<span>{t('command.documentation')}</span>
						<CommandShortcut>GitHub</CommandShortcut>
					</CommandItem>
				</CommandGroup>
				{isAdmin() && (
					<>
						<CommandSeparator className="mb-1.5" />
						<CommandGroup heading={t("command.admin")}>
							<CommandItem
								keywords={['pocketbase']}
								onSelect={() => {
									setOpen(false)
									window.open('/_/', '_blank')
								}}
							>
								<UsersIcon className="mr-2 h-4 w-4" />
								<span>{t('user_dm.users')}</span>
								<CommandShortcut>{t("command.admin")}</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/logs', '_blank')
								}}
							>
								<LogsIcon className="mr-2 h-4 w-4" />
								<span>{t('user_dm.logs')}</span>
								<CommandShortcut>{t("command.admin")}</CommandShortcut>
							</CommandItem>
							<CommandItem
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/settings/backups', '_blank')
								}}
							>
								<DatabaseBackupIcon className="mr-2 h-4 w-4" />
								<span>{t('user_dm.backups')}</span>
								<CommandShortcut>{t("command.admin")}</CommandShortcut>
							</CommandItem>
							<CommandItem
								keywords={['oauth', 'oicd']}
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/settings/auth-providers', '_blank')
								}}
							>
								<LockKeyholeIcon className="mr-2 h-4 w-4" />
								<span>{t('user_dm.auth_providers')}</span>
								<CommandShortcut>{t("command.admin")}</CommandShortcut>
							</CommandItem>
							<CommandItem
								keywords={['email']}
								onSelect={() => {
									setOpen(false)
									window.open('/_/#/settings/mail', '_blank')
								}}
							>
								<MailIcon className="mr-2 h-4 w-4" />
								<span>{t('command.SMTP_settings')}</span>
								<CommandShortcut>{t("command.admin")}</CommandShortcut>
							</CommandItem>
						</CommandGroup>
					</>
				)}
			</CommandList>
		</CommandDialog>
	)
}
