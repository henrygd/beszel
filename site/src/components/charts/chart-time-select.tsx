import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from '@/components/ui/select'
import { $chartTime } from '@/lib/stores'
import { chartTimeData, cn } from '@/lib/utils'
import { ChartTimes } from '@/types'
import { useStore } from '@nanostores/react'
import { useEffect } from 'react'

export default function ChartTimeSelect({ className }: { className?: string }) {
	const chartTime = useStore($chartTime)

	useEffect(() => {
		// todo make sure this doesn't cause multiple fetches on load
		return () => $chartTime.set('1h')
	}, [])

	return (
		<Select
			defaultValue="1h"
			value={chartTime}
			onValueChange={(value: ChartTimes) => $chartTime.set(value)}
		>
			<SelectTrigger className={cn(className, 'w-40 px-5')}>
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
	)
}
