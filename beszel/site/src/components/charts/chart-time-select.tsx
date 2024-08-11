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

export default function ChartTimeSelect({ className }: { className?: string }) {
	const chartTime = useStore($chartTime)

	return (
		<Select
			defaultValue="1h"
			value={chartTime}
			onValueChange={(value: ChartTimes) => $chartTime.set(value)}
		>
			<SelectTrigger className={cn(className, 'px-5')}>
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
