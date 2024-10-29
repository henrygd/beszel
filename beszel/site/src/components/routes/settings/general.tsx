import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from '@/components/ui/select'
import { chartTimeData } from '@/lib/utils'
import { Separator } from '@/components/ui/separator'
import { LoaderCircleIcon, SaveIcon } from 'lucide-react'
import { UserSettings } from '@/types'
import { saveSettings } from './layout'
import { useState, useEffect } from 'react'
// import { Input } from '@/components/ui/input'
import { useTranslation } from 'react-i18next'
import languages from '../../../lib/languages.json'

export default function SettingsProfilePage({ userSettings }: { userSettings: UserSettings }) {
	const { t, i18n } = useTranslation()

	useEffect(() => {
		document.documentElement.lang = i18n.language;
	}, [i18n.language]);

	const [isLoading, setIsLoading] = useState(false)

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault()
		setIsLoading(true)
		const formData = new FormData(e.target as HTMLFormElement)
		const data = Object.fromEntries(formData) as Partial<UserSettings>
		await saveSettings(data)
		setIsLoading(false)
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">{t('settings.general.title')}</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					{t('settings.general.subtitle')}
				</p>
			</div>
			<Separator className="my-4" />
			<form onSubmit={handleSubmit} className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">{t('settings.general.language.title')}</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							{t('settings.general.language.subtitle_1')}{' '}
							<a href="https://crowdin.com/project/beszel" className="link" target="_blank">
								Crowdin
							</a>{' '}
							{t('settings.general.language.subtitle_2')}
						</p>
					</div>
					<Label className="block" htmlFor="lang">
						{t('settings.general.language.preferred_language')}
					</Label>
					<Select defaultValue={i18n.language} onValueChange={(lang: string) => i18n.changeLanguage(lang)}>
						<SelectTrigger id="lang">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{languages.map((lang) => (
								<SelectItem key={lang.lang} value={lang.lang}>
									{lang.label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">{t('settings.general.chart_options.title')}</h3>
						<p className="text-sm text-muted-foreground leading-relaxed">
							{t('settings.general.chart_options.subtitle')}
						</p>
					</div>
					<Label className="block" htmlFor="chartTime">
						{t('settings.general.chart_options.default_time_period')}
					</Label>
					<Select
						name="chartTime"
						key={userSettings.chartTime}
						defaultValue={userSettings.chartTime}
					>
						<SelectTrigger id="chartTime">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{Object.entries(chartTimeData).map(([value, { label }]) => (
								<SelectItem key={label} value={value}>
									{label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
					<p className="text-[0.8rem] text-muted-foreground">
						{t('settings.general.chart_options.default_time_period_des')}
					</p>
				</div>
				<Separator />
				<Button
					type="submit"
					className="flex items-center gap-1.5 disabled:opacity-100"
					disabled={isLoading}
				>
					{isLoading ? (
						<LoaderCircleIcon className="h-4 w-4 animate-spin" />
					) : (
						<SaveIcon className="h-4 w-4" />
					)}
					{t('settings.save_settings')}
				</Button>
			</form>
		</div>
	)
}
