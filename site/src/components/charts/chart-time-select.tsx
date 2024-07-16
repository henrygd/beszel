import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from '@/components/ui/select'
import { $chartTime } from '@/lib/stores'
import { cn } from '@/lib/utils'
import { useStore } from '@nanostores/react'
import { useEffect } from 'react'

export default function ChartTimeSelect({ className }: { className?: string }) {
	const chartTime = useStore($chartTime)

	useEffect(() => {
		// todo make sure this doesn't cause multiple fetches on load
		return () => $chartTime.set('1h')
	}, [])

	return (
		<Select defaultValue="1h" value={chartTime} onValueChange={(value) => $chartTime.set(value)}>
			<SelectTrigger className={cn(className, 'w-40 px-5')}>
				<SelectValue placeholder="1h" />
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="1h">1 hour</SelectItem>
				<SelectItem value="12h">12 hours</SelectItem>
				<SelectItem value="24h">24 hours</SelectItem>
				<SelectItem value="1w">1 week</SelectItem>
				<SelectItem value="30d">30 days</SelectItem>
			</SelectContent>
		</Select>
	)
}
