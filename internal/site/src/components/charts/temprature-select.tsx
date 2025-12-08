import { useStore } from "@nanostores/react"
import { HistoryIcon } from "lucide-react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { $chartTime } from "@/lib/stores"
import { chartTimeData, cn, compareSemVer, parseSemVer } from "@/lib/utils"
import type { ChartData, ChartTimes, SemVer, SystemRecord } from "@/types"
import { memo } from "react"
import { pb } from "@/lib/api"
import { ThermometerIcon } from "../ui/icons"

export default memo(function TempratureSelect({
	className,
	agentVersion,
	chartData,
	system
}: {
	className?: string
	agentVersion: SemVer,
	chartData: ChartData,
	system: SystemRecord
}) {
	// console.log(system);
	let thisTempratureConfig = 'Default'
	const thisChart = Object.entries(chartData.systemStats[0].stats.t)
	if(system.config && system.config.temprature){
		thisTempratureConfig = system.config.temprature
	}
	else {
		thisTempratureConfig = 'null'
	}
	// console.log(thisChart);
	// const chartTime = thisChart
	
	// remove chart times that are not supported by the system agent version
	// const availableChartTimes = Object.entries(chartTimeData).filter(([_, { minVersion }]) => {
		// 	if (!minVersion) {
			// 		return true
			// 	}
			// 	return compareSemVer(agentVersion, parseSemVer(minVersion)) >= 0
			// })
			
	async function changeValue(value:string) {
		if(system.config == null){
			system.config = {}
		}
		system.config.temprature = value
		const prom = await pb.collection('systems').update(system.id, system)
		// console.log(prom);
		
	}
	
	return (
		<Select value={thisTempratureConfig} onValueChange={(value: any) => changeValue(value)}>
			<SelectTrigger className={cn(className, "relative ps-10 pe-5")}>
				<ThermometerIcon className="h-4 w-4 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				<SelectItem key={null} value={'null'}>
					Default
				</SelectItem>
				{thisChart.sort((a, b) => {
					const [, inta] = a 
					const [, intb] = b 
					return intb - inta;
				}).map(([x, i]) => (
					<SelectItem key={i+x} value={x}>
						{x}: {i}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	)
})
