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
import { SaveIcon } from 'lucide-react'

export default function SettingsProfilePage() {
	return (
		<div>
			{/* <div>
				<h3 className="text-lg font-medium mb-1">General</h3>
				<p className="text-sm text-muted-foreground">Set your preferred language and timezone.</p>
			</div>
			<Separator className="mt-6 mb-5" /> */}
			<div className="space-y-5">
				<div className="space-y-2">
					<div className="mb-4">
						<h3 className="mb-1 text-lg font-medium">Chart options</h3>
						<p className="text-sm text-muted-foreground">Adjust display options for charts.</p>
					</div>
					<Label className="block">Default time period</Label>
					<Select defaultValue="1h">
						<SelectTrigger>
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
				<Button type="submit" className="flex items-center gap-1.5">
					<SaveIcon className="h-4 w-4" />
					Save settings
				</Button>
			</div>
		</div>
	)
}
