'use client'

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import {
	ChartConfig,
	ChartContainer,
	ChartLegend,
	ChartLegendContent,
	ChartTooltip,
	ChartTooltipContent,
} from '@/components/ui/chart'
import { useMemo, useState } from 'react'
import { formatShortDate, formatShortTime } from '@/lib/utils'

export default function ({ chartData }: { chartData: Record<string, number | string>[] }) {
	const [containerNames, setContainerNames] = useState([] as string[])

	const chartConfig = useMemo(() => {
		console.log('chartData', chartData)
		let config = {} as Record<
			string,
			{
				label: string
				color: string
			}
		>
		const lastRecord = chartData.at(-1)
		// @ts-ignore
		let allKeys = new Set(Object.keys(lastRecord))
		allKeys.delete('time')
		const keys = Array.from(allKeys)
		keys.sort((a, b) => (lastRecord![b] as number) - (lastRecord![a] as number))
		setContainerNames(keys)
		const length = keys.length
		for (let i = 0; i < length; i++) {
			const key = keys[i]
			const hue = ((i * 360) / length) % 360
			config[key] = {
				label: key,
				color: `hsl(${hue}, 60%, 60%)`,
			}
		}
		console.log('config', config)
		return config satisfies ChartConfig
	}, [chartData])

	if (!containerNames.length) {
		return null
	}

	return (
		<ChartContainer config={chartConfig} className="h-full w-full absolute aspect-auto">
			<AreaChart
				accessibilityLayer
				data={chartData}
				margin={{
					left: 12,
					right: 12,
					top: 12,
				}}
				// reverseStackOrder={true}
			>
				<CartesianGrid vertical={false} />
				{/* <YAxis domain={[0, 250]} tickCount={5} tickLine={false} axisLine={false} tickMargin={8} /> */}
				<XAxis
					dataKey="time"
					tickLine={false}
					axisLine={false}
					tickMargin={8}
					tickFormatter={formatShortTime}
				/>
				<ChartTooltip
					cursor={false}
					labelFormatter={formatShortDate}
					// itemSorter={(item) => {
					// 	console.log('itemSorter', item)
					// 	return -item.value
					// }}
					content={
						<ChartTooltipContent
							// itemSorter={(item) => {
							// 	console.log('itemSorter', item)
							// 	return -item.value
							// }}
							indicator="line"
						/>
					}
				/>
				{containerNames.map((key) => (
					<Area
						key={key}
						dataKey={key}
						type="natural"
						fill={chartConfig[key].color}
						fillOpacity={0.4}
						stroke={chartConfig[key].color}
						stackId="a"
					/>
				))}
				{/* <Area
					dataKey="other"
					type="natural"
					fill="var(--color-other)"
					fillOpacity={0.4}
					stroke="var(--color-other)"
					stackId="a"
				/> */}
				{/* <ChartLegend content={<ChartLegendContent />} className="flex-wrap gap-y-2 mb-2" /> */}
			</AreaChart>
		</ChartContainer>
	)
}
