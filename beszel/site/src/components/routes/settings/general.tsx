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
import { useState } from 'react'

export default function SettingsProfilePage({ userSettings }: { userSettings: UserSettings }) {
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
				<h3 className="text-xl font-medium mb-2">General</h3>
				<p className="text-sm text-muted-foreground">
					Set your preferred language and chart display options.
				</p>
			</div>
			<Separator className="my-4" />
			<form onSubmit={handleSubmit} className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">Language</h3>
						<p className="text-sm text-muted-foreground">
							Additional language support coming soon.
						</p>
					</div>
					<Label className="block" htmlFor="lang">
						Preferred language
					</Label>
					<Select defaultValue="en">
						<SelectTrigger id="lang">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="en">English</SelectItem>
						</SelectContent>
					</Select>
				</div>
				<Separator />
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">Chart options</h3>
						<p className="text-sm text-muted-foreground">Adjust display options for charts.</p>
					</div>
					<Label className="block" htmlFor="chartTime">
						Default time period
					</Label>
					<Select name="chartTime" defaultValue={userSettings.chartTime}>
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
						Sets the default time range for charts when a system is viewed.
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
					Save settings
				</Button>
			</form>
		</div>
	)
}
