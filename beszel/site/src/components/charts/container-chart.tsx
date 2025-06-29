import { Area, AreaChart, CartesianGrid, YAxis, Line, LineChart } from "recharts"
import { ChartConfig, ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
import { memo, useMemo } from "react"
import {
	useYAxisWidth,
	cn,
	formatShortDate,
	decimalString,
	chartMargin,
	toFixedFloat,
	getSizeAndUnit,
	toFixedWithoutTrailingZeros,
} from "@/lib/utils"
// import Spinner from '../spinner'
import { useStore } from "@nanostores/react"
import { $containerFilter, $containerColors } from "@/lib/stores"
import { ChartData } from "@/types"
import { Separator } from "../ui/separator"
import { ChartType } from "@/lib/enums"
import React from "react"

export default memo(function ContainerChart({
	dataKey,
	chartData,
	chartType,
	unit = "%",
}: {
	dataKey: string
	chartData: ChartData
	chartType: ChartType
	unit?: string
}) {
	const filter = useStore($containerFilter)
	const containerColors = useStore($containerColors)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	const { containerData } = chartData

	const isNetChart = chartType === ChartType.Network
	const isVolumeChart = chartType === ChartType.Volume
	const isHealthChart = chartType === ChartType.Health
	const isUptimeChart = chartType === ChartType.Uptime
	const isHealthUptimeChart = chartType === ChartType.HealthUptime

	// For volume charts, we need to process the data differently
	const volumeChartData = useMemo(() => {
		if (!isVolumeChart) return null

		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}
		const volumeSums = {} as Record<string, number>
		const volumeContainers = {} as Record<string, string[]> // Track which containers use each volume

		// Collect volume data from all containers
		for (let containerStats of containerData) {
			if (!containerStats.created) continue
			
			let newData = { created: containerStats.created } as Record<string, number | string>

			// Process each container's volume data
			for (let [containerName, containerData] of Object.entries(containerStats)) {
				if (containerName === "created") continue
				
				// Check if this is a ContainerStats object with volume data
				if (typeof containerData === 'object' && containerData && 'v' in containerData && containerData.v) {
					// Add volume data for this container
					for (let [volumeName, volumeSize] of Object.entries(containerData.v)) {
						if (typeof volumeSize === 'number' && volumeSize > 0) {
							newData[volumeName] = (newData[volumeName] as number || 0) + volumeSize
							volumeSums[volumeName] = (volumeSums[volumeName] ?? 0) + volumeSize
							
							// Track which containers use this volume
							if (!volumeContainers[volumeName]) {
								volumeContainers[volumeName] = []
							}
							if (!volumeContainers[volumeName].includes(containerName)) {
								volumeContainers[volumeName].push(containerName)
							}
						}
					}
				}
			}
			
			newChartData.data.push(newData)
		}
		
		// Apply container filtering
		const allowedVolumes = new Set<string>()
		if (Array.isArray(filter) && filter.length > 0) {
			for (const [vol, containers] of Object.entries(volumeContainers)) {
				if (containers.some((c) => filter.includes(c))) {
					allowedVolumes.add(vol)
				}
			}
		}
		
		// Filter data based on allowed volumes
		if (allowedVolumes.size > 0) {
			newChartData.data = newChartData.data.map(data => {
				const filteredData = { created: data.created } as Record<string, number | string>
				for (const [key, value] of Object.entries(data)) {
					if (key === "created" || allowedVolumes.has(key)) {
						filteredData[key] = value
					}
				}
				return filteredData
			})
		}
		
		// Assign colors based on containers that use each volume
		const keys = Object.keys(volumeSums).sort((a, b) => volumeSums[b] - volumeSums[a])
		for (let key of keys) {
			// Get the first container that uses this volume and use its color
			const containers = volumeContainers[key] || []
			if (containers.length > 0) {
				// Use the color of the first container that uses this volume
				const firstContainer = containers[0]
				newChartData.colors[key] = containerColors[firstContainer] || `hsl(${Math.random() * 360}, 60%, 55%)`
			} else {
				// Fallback to generated color if no containers found
				newChartData.colors[key] = `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
			}
		}
		
		return newChartData
	}, [containerData, containerColors, filter, isVolumeChart])

	// For health charts, we need to process the data differently
	const healthChartData = useMemo(() => {
		if (!isHealthChart) return null

		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}

		// Collect health data from all containers
		for (let containerStats of containerData) {
			if (!containerStats.created) continue
			
			let newData = { created: containerStats.created } as Record<string, number | string>

			// Process each container's health data
			for (let [containerName, containerData] of Object.entries(containerStats)) {
				if (containerName === "created") continue
				
				// Check if this is a ContainerStats object with health data
				if (typeof containerData === 'object' && containerData && 'h' in containerData && containerData.h) {
					// Convert health status to numeric value for charting
					const healthStatus = containerData.h as string
					let healthValue = 0 // Unknown/None
					
					switch (healthStatus.toLowerCase()) {
						case 'healthy':
							healthValue = 3
							break
						case 'unhealthy':
							healthValue = 1
							break
						case 'starting':
							healthValue = 2
							break
						default:
							healthValue = 0 // Unknown/None
					}
					
					newData[containerName] = healthValue
				}
			}
			
			newChartData.data.push(newData)
		}
		
		// Apply container filtering
		const allowedContainers = new Set<string>()
		if (Array.isArray(filter) && filter.length > 0) {
			for (const containerName of filter) {
				allowedContainers.add(containerName)
			}
		}
		
		// Filter data based on allowed containers
		if (allowedContainers.size > 0) {
			newChartData.data = newChartData.data.map(data => {
				const filteredData = { created: data.created } as Record<string, number | string>
				for (const [key, value] of Object.entries(data)) {
					if (key === "created" || allowedContainers.has(key)) {
						filteredData[key] = value
					}
				}
				return filteredData
			})
		}
		
		// Assign colors based on container names
		const keys = Object.keys(newChartData.data[0] || {}).filter(key => key !== "created")
		for (let key of keys) {
			newChartData.colors[key] = containerColors[key] || `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
		}
		
		return newChartData
	}, [containerData, containerColors, filter, isHealthChart])

	// For uptime charts, we need to process the data differently
	const uptimeChartData = useMemo(() => {
		if (!isUptimeChart) return null

		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}

		// Collect uptime data from all containers
		for (let containerStats of containerData) {
			if (!containerStats.created) continue
			
			let newData = { created: containerStats.created } as Record<string, number | string>

			// Process each container's uptime data
			for (let [containerName, containerData] of Object.entries(containerStats)) {
				if (containerName === "created") continue
				
				// Check if this is a ContainerStats object with uptime data
				if (typeof containerData === 'object' && containerData && 'u' in containerData && containerData.u) {
					// Convert seconds to hours for better readability
					const uptimeHours = (containerData.u as number) / 3600
					newData[containerName] = uptimeHours
				}
			}
			
			newChartData.data.push(newData)
		}
		
		// Apply container filtering
		const allowedContainers = new Set<string>()
		if (Array.isArray(filter) && filter.length > 0) {
			for (const containerName of filter) {
				allowedContainers.add(containerName)
			}
		}
		
		// Filter data based on allowed containers
		if (allowedContainers.size > 0) {
			newChartData.data = newChartData.data.map(data => {
				const filteredData = { created: data.created } as Record<string, number | string>
				for (const [key, value] of Object.entries(data)) {
					if (key === "created" || allowedContainers.has(key)) {
						filteredData[key] = value
					}
				}
				return filteredData
			})
		}
		
		// Assign colors based on container names
		const keys = Object.keys(newChartData.data[0] || {}).filter(key => key !== "created")
		for (let key of keys) {
			newChartData.colors[key] = containerColors[key] || `hsl(${((keys.indexOf(key) * 360) / keys.length) % 360}, 60%, 55%)`
		}
		
		return newChartData
	}, [containerData, containerColors, filter, isUptimeChart])

	// For combined health+uptime chart
	const healthUptimeChartData = useMemo(() => {
		if (!isHealthUptimeChart) return null
		
		const newChartData = { data: [], colors: {} } as {
			data: Record<string, number | string>[]
			colors: Record<string, string>
		}
		for (let containerStats of containerData) {
			if (!containerStats.created) continue
			let newData = { created: containerStats.created } as Record<string, number | string>
			for (let [containerName, containerData] of Object.entries(containerStats)) {
				if (containerName === "created") continue
				if (typeof containerData === 'object' && containerData) {
					// Uptime (in hours)
					if ('u' in containerData && containerData.u) {
						newData[containerName + "_uptime"] = (containerData.u as number) / 3600
					}
					// Health (categorical)
					if ('h' in containerData) {
						let healthValue = 0
						const healthStatus = (containerData.h as string || '').toLowerCase()
						switch (healthStatus) {
							case 'healthy': healthValue = 3; break
							case 'starting': healthValue = 2; break
							case 'unhealthy': healthValue = 1; break
							default: healthValue = 0
						}
						newData[containerName + "_health"] = healthValue
					}
				}
			}
			newChartData.data.push(newData)
		}
		
		// Assign colors for each container's health and uptime data
		const containerNames = new Set<string>()
		for (let stats of containerData) {
			for (let key in stats) {
				if (!key || key === "created") continue
				containerNames.add(key)
			}
		}
		
		for (let containerName of containerNames) {
			const color = containerColors[containerName] || `hsl(${Math.random() * 360}, 60%, 55%)`
			newChartData.colors[containerName + "_uptime"] = color
			newChartData.colors[containerName + "_health"] = color
		}
		
		return newChartData
	}, [containerData, containerColors, isHealthUptimeChart])

	const chartConfig = useMemo(() => {
		let config = {} as Record<
			string,
			{
				label: string
				color: string
			}
		>
		
		// Get all container names from the data
		const containerNames = new Set<string>()
		for (let stats of containerData) {
			for (let key in stats) {
				if (!key || key === "created") {
					continue
				}
				containerNames.add(key)
			}
		}
		
		// Use consistent colors from the store, fallback to generated colors if not available
		for (let containerName of containerNames) {
			const color = containerColors[containerName] || `hsl(${Math.random() * 360}, 60%, 55%)`
			config[containerName] = {
				label: containerName,
				color: color,
			}
		}
		
		return config satisfies ChartConfig
	}, [chartData, containerColors])

	const { toolTipFormatter, dataFunction, tickFormatter } = useMemo(() => {
		const obj = {} as {
			toolTipFormatter: (item: any, key: string) => React.ReactNode | string
			dataFunction: (key: string, data: any) => number | null
			tickFormatter: (value: any) => string
		}
		// tick formatter
		if (chartType === ChartType.CPU) {
			obj.tickFormatter = (value) => {
				const val = toFixedWithoutTrailingZeros(value, 2) + unit
				return updateYAxisWidth(val)
			}
		} else if (isHealthChart) {
			obj.tickFormatter = (value) => {
				let healthLabel = "Unknown"
				switch (value) {
					case 3:
						healthLabel = "Healthy"
						break
					case 2:
						healthLabel = "Starting"
						break
					case 1:
						healthLabel = "Unhealthy"
						break
					case 0:
						healthLabel = "None"
						break
				}
				return updateYAxisWidth(healthLabel)
			}
		} else if (isUptimeChart) {
			obj.tickFormatter = (value) => {
				const hours = Math.floor(value)
				const minutes = Math.floor((value - hours) * 60)
				const label = `${hours}h ${minutes}m`
				return updateYAxisWidth(label)
			}
		} else {
			obj.tickFormatter = (value) => {
				const { v, u } = getSizeAndUnit(value, false)
				return updateYAxisWidth(`${toFixedFloat(v, 2)}${u}${isNetChart ? "/s" : ""}`)
			}
		}
		// tooltip formatter
		if (isNetChart) {
			obj.toolTipFormatter = (item: any, key: string) => {
				try {
					const sent = item?.payload?.[key]?.ns ?? 0
					const received = item?.payload?.[key]?.nr ?? 0
					return (
						<span className="flex">
							{decimalString(received)} MB/s
							<span className="opacity-70 ms-0.5"> rx </span>
							<Separator orientation="vertical" className="h-3 mx-1.5 bg-primary/40" />
							{decimalString(sent)} MB/s
							<span className="opacity-70 ms-0.5"> tx</span>
						</span>
					)
				} catch (e) {
					return null
				}
			}
		} else if (chartType === ChartType.Memory) {
			obj.toolTipFormatter = (item: any) => {
				const { v, u } = getSizeAndUnit(item.value, false)
				return decimalString(v, 2) + u
			}
		} else if (isVolumeChart) {
			obj.toolTipFormatter = (item: any) => {
				const { v, u } = getSizeAndUnit(item.value, false)
				return decimalString(v, 2) + u
			}
		} else if (isHealthChart) {
			obj.toolTipFormatter = (item: any) => {
				let healthLabel = "Unknown"
				switch (item.value) {
					case 3:
						healthLabel = "Healthy"
						break
					case 2:
						healthLabel = "Starting"
						break
					case 1:
						healthLabel = "Unhealthy"
						break
					case 0:
						healthLabel = "None"
						break
				}
				return healthLabel
			}
		} else if (isUptimeChart) {
			obj.toolTipFormatter = (item: any) => {
				const hours = Math.floor(item.value)
				const minutes = Math.floor((item.value - hours) * 60)
				const days = Math.floor(hours / 24)
				const remainingHours = hours % 24
				
				if (days > 0) {
					return `${days}d ${remainingHours}h ${minutes}m`
				} else {
					return `${hours}h ${minutes}m`
				}
			}
		} else {
			obj.toolTipFormatter = (item: any) => decimalString(item.value) + unit
		}
		// data function
		if (isNetChart) {
			obj.dataFunction = (key: string, data: any) => (data[key] ? data[key].nr + data[key].ns : null)
		} else {
			obj.dataFunction = (key: string, data: any) => data[key]?.[dataKey] ?? null
		}
		return obj
	}, [chartType, isNetChart, isVolumeChart, unit])

	// console.log('rendered at', new Date())

	if (containerData.length === 0) {
		return null
	}

	// For volume charts, check if we have volume data
	if (isVolumeChart) {
		if (!volumeChartData || Object.keys(volumeChartData.colors).length === 0) {
			return null
		}
	}

	// For health charts, check if we have health data
	if (isHealthChart) {
		if (!healthChartData || Object.keys(healthChartData.colors).length === 0) {
			return null
		}
	}

	// For uptime charts, check if we have uptime data
	if (isUptimeChart) {
		if (!uptimeChartData || Object.keys(uptimeChartData.colors).length === 0) {
			return null
		}
	}

	// For combined health+uptime chart
	if (isHealthUptimeChart) {
		if (!healthUptimeChartData || healthUptimeChartData.data.length === 0) return null
		
		// Get the latest data point for table display
		const latestData = healthUptimeChartData.data[healthUptimeChartData.data.length - 1]
		if (!latestData) return null
		
		// Extract container data for table
		const containerTableData = []
		const containerNames = new Set<string>()
		
		// Get all container names from the data
		for (const key of Object.keys(latestData)) {
			if (key === 'created') continue
			const containerName = key.replace(/_uptime$/, '').replace(/_health$/, '')
			containerNames.add(containerName)
		}
		
		// Build table data
		for (const containerName of containerNames) {
			const uptimeKey = containerName + '_uptime'
			const healthKey = containerName + '_health'
			
			const uptimeHours = latestData[uptimeKey] || 0
			const healthValue = latestData[healthKey] || 0
			
			// Convert health value to readable string
			let healthStatus = "Unknown"
			switch (healthValue) {
				case 3: healthStatus = "Healthy"; break
				case 2: healthStatus = "Starting"; break
				case 1: healthStatus = "Unhealthy"; break
				case 0: healthStatus = "None"; break
			}
			
			// Format uptime
			const hours = Math.floor(uptimeHours)
			const minutes = Math.floor((uptimeHours - hours) * 60)
			const days = Math.floor(hours / 24)
			const remainingHours = hours % 24
			
			let uptimeDisplay = ""
			if (days > 0) {
				uptimeDisplay = `${days}d ${remainingHours}h ${minutes}m`
			} else {
				uptimeDisplay = `${hours}h ${minutes}m`
			}
			
			// Get stack/project information from container data
			let stackName = "—" // Default to dash when no stack info
			
			// Find the container data to get project information
			for (const containerStats of containerData) {
				if (containerStats.created && containerStats[containerName]) {
					const containerData = containerStats[containerName]
					if (typeof containerData === 'object' && containerData && 'p' in containerData && containerData.p) {
						stackName = containerData.p as string
						break
					}
				}
			}
			
			containerTableData.push({
				name: containerName,
				health: healthStatus,
				uptime: uptimeDisplay,
				uptimeHours: uptimeHours,
				healthValue: healthValue,
				stack: stackName,
				color: containerColors[containerName] || `hsl(${Math.random() * 360}, 60%, 55%)`
			})
		}
		
		// Sort by uptime (longest first)
		containerTableData.sort((a, b) => b.uptimeHours - a.uptimeHours)
		
		// Apply container filtering
		let filteredContainerData = containerTableData
		if (Array.isArray(filter) && filter.length > 0) {
			filteredContainerData = containerTableData.filter(container => 
				filter.includes(container.name)
			)
		}
		
		// Sorting logic
		const [sortField, setSortField] = React.useState<'name' | 'stack' | 'health' | 'uptime'>('uptime')
		const [sortDirection, setSortDirection] = React.useState<'asc' | 'desc'>('desc')
		
		// Sort the filtered data
		const sortedContainerData = [...filteredContainerData].sort((a, b) => {
			let aValue: string | number
			let bValue: string | number
			
			switch (sortField) {
				case 'name':
					aValue = a.name.toLowerCase()
					bValue = b.name.toLowerCase()
					break
				case 'stack':
					aValue = a.stack.toLowerCase()
					bValue = b.stack.toLowerCase()
					break
				case 'health':
					aValue = a.healthValue
					bValue = b.healthValue
					break
				case 'uptime':
					aValue = a.uptimeHours
					bValue = b.uptimeHours
					break
				default:
					return 0
			}
			
			if (aValue < bValue) return sortDirection === 'asc' ? -1 : 1
			if (aValue > bValue) return sortDirection === 'asc' ? 1 : -1
			return 0
		})
		
		// Handle sort click
		const handleSort = (field: 'name' | 'stack' | 'health' | 'uptime') => {
			if (sortField === field) {
				setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
			} else {
				setSortField(field)
				setSortDirection('asc')
			}
		}
		
		// Get sort icon
		const getSortIcon = (field: 'name' | 'stack' | 'health' | 'uptime') => {
			if (sortField !== field) return '↑↓'
			return sortDirection === 'asc' ? '↑' : '↓'
		}
		
		// Pagination logic
		const [currentPage, setCurrentPage] = React.useState(1)
		const containersPerPage = 5
		const totalPages = Math.ceil(sortedContainerData.length / containersPerPage)
		const startIndex = (currentPage - 1) * containersPerPage
		const endIndex = startIndex + containersPerPage
		const currentContainers = sortedContainerData.slice(startIndex, endIndex)
		
		return (
			<div className="w-full h-full flex flex-col opacity-100">
				<div className="flex-1 p-2 overflow-hidden">
					<div className="overflow-x-auto h-full">
						<table className="w-full text-xs table-fixed">
							<thead>
								<tr className="border-b border-border">
									<th 
										className="text-left font-medium p-1 w-2/5 cursor-pointer hover:bg-muted/50 transition-colors select-none"
										onClick={() => handleSort('name')}
									>
										<div className="flex items-center gap-1">
											Container
											<span className="text-xs opacity-60">{getSortIcon('name')}</span>
										</div>
									</th>
									<th 
										className="text-left font-medium p-1 w-1/5 cursor-pointer hover:bg-muted/50 transition-colors select-none"
										onClick={() => handleSort('stack')}
									>
										<div className="flex items-center gap-1">
											Stack
											<span className="text-xs opacity-60">{getSortIcon('stack')}</span>
										</div>
									</th>
									<th 
										className="text-left font-medium p-1 w-1/5 cursor-pointer hover:bg-muted/50 transition-colors select-none"
										onClick={() => handleSort('health')}
									>
										<div className="flex items-center gap-1">
											Health
											<span className="text-xs opacity-60">{getSortIcon('health')}</span>
										</div>
									</th>
									<th 
										className="text-left font-medium p-1 w-1/5 cursor-pointer hover:bg-muted/50 transition-colors select-none"
										onClick={() => handleSort('uptime')}
									>
										<div className="flex items-center gap-1">
											Uptime
											<span className="text-xs opacity-60">{getSortIcon('uptime')}</span>
										</div>
									</th>
								</tr>
							</thead>
							<tbody>
								{currentContainers.map((container, index) => (
									<tr key={container.name} className="border-b border-border/30 hover:bg-muted/30">
										<td className="p-1 w-2/5">
											<div className="flex items-center gap-1.5">
												<div 
													className="w-2.5 h-2.5 rounded-full flex-shrink-0" 
													style={{ backgroundColor: container.color }}
												/>
												<span className="text-xs truncate">{container.name}</span>
											</div>
										</td>
										<td className="p-1 w-1/5">
											<span className="text-xs text-muted-foreground truncate" title={container.stack}>
												{container.stack}
											</span>
										</td>
										<td className="p-1 w-1/5">
											<span className={cn(
												"px-1.5 py-0.5 rounded text-xs font-medium whitespace-nowrap",
												{
													"bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400": container.healthValue === 3,
													"bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400": container.healthValue === 2,
													"bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400": container.healthValue === 1,
													"bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-400": container.healthValue === 0,
												}
											)}>
												{container.health}
											</span>
										</td>
										<td className="p-1 w-1/5 text-xs whitespace-nowrap">
											{container.uptime}
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				</div>
				
				{/* Pagination Controls */}
				{totalPages > 1 && (
					<div className="flex items-center justify-between px-2 py-2 border-t border-border bg-muted/20">
						<div className="text-xs text-muted-foreground">
							Showing {startIndex + 1}-{Math.min(endIndex, sortedContainerData.length)} of {sortedContainerData.length} containers
						</div>
						<div className="flex items-center gap-1">
							<button
								onClick={() => setCurrentPage(prev => Math.max(1, prev - 1))}
								disabled={currentPage === 1}
								className={cn(
									"px-2 py-1 text-xs rounded border transition-colors",
									"hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed",
									"border-border hover:border-border/60"
								)}
							>
								Previous
							</button>
							
							{/* Page numbers */}
							{Array.from({ length: totalPages }, (_, i) => i + 1).map(page => (
								<button
									key={page}
									onClick={() => setCurrentPage(page)}
									className={cn(
										"px-2 py-1 text-xs rounded border transition-colors min-w-[28px]",
										page === currentPage
											? "bg-primary text-primary-foreground border-primary"
											: "border-border hover:bg-muted hover:border-border/60"
									)}
								>
									{page}
								</button>
							))}
							
							<button
								onClick={() => setCurrentPage(prev => Math.min(totalPages, prev + 1))}
								disabled={currentPage === totalPages}
								className={cn(
									"px-2 py-1 text-xs rounded border transition-colors",
									"hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed",
									"border-border hover:border-border/60"
								)}
							>
								Next
							</button>
						</div>
					</div>
				)}
			</div>
		)
	}

	// Only show selected containers, or all if none selected
	const visibleKeys = Array.isArray(filter) && filter.length > 0 ? filter : Object.keys(chartConfig)

	// Render volume chart
	if (isVolumeChart) {
		const colors = Object.keys(volumeChartData!.colors)
		return (
			<div className="w-full h-full">
				<ChartContainer
					className={cn("h-full w-full absolute bg-card opacity-0 transition-opacity", {
						"opacity-100": yAxisWidth,
					})}
				>
					<AreaChart accessibilityLayer data={volumeChartData!.data} margin={chartMargin} reverseStackOrder={true}>
						<CartesianGrid vertical={false} />
						<YAxis
							direction="ltr"
							orientation={chartData.orientation}
							className="tracking-tighter"
							domain={[0, "auto"]}
							width={yAxisWidth}
							tickFormatter={(value) => {
								const { v, u } = getSizeAndUnit(value, false)
								return updateYAxisWidth(toFixedFloat(v, 2) + u)
							}}
							tickLine={false}
							axisLine={false}
						/>
						{xAxis(chartData)}
						<ChartTooltip
							animationEasing="ease-out"
							animationDuration={150}
							// @ts-ignore
							itemSorter={(a, b) => b.value - a.value}
							content={
								<ChartTooltipContent
									truncate={true}
									labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
									contentFormatter={toolTipFormatter}
								/>
							}
						/>
						{colors.map((key) => {
							const filtered = Array.isArray(filter) && filter.length > 0 && !filter.includes(key)
							let fillOpacity = filtered ? 0.05 : 0.4
							let strokeOpacity = filtered ? 0.1 : 1
							return (
								<Area
									key={key}
									dataKey={key}
									name={key}
									type="monotoneX"
									fill={volumeChartData!.colors[key]}
									fillOpacity={fillOpacity}
									stroke={volumeChartData!.colors[key]}
									strokeOpacity={strokeOpacity}
									activeDot={{ opacity: filtered ? 0 : 1 }}
									stackId="a"
									isAnimationActive={false}
								/>
							)
						})}
					</AreaChart>
				</ChartContainer>
			</div>
		)
	}

	// Render health chart
	if (isHealthChart) {
		const colors = Object.keys(healthChartData!.colors)
		return (
			<div className="w-full h-full">
				<ChartContainer
					className={cn("h-full w-full absolute bg-card opacity-0 transition-opacity", {
						"opacity-100": yAxisWidth,
					})}
				>
					<LineChart accessibilityLayer data={healthChartData!.data} margin={chartMargin}>
						<CartesianGrid vertical={false} />
						<YAxis
							direction="ltr"
							orientation={chartData.orientation}
							className="tracking-tighter"
							domain={[0, 3]}
							width={yAxisWidth}
							tickFormatter={tickFormatter}
							tickLine={false}
							axisLine={false}
						/>
						{xAxis(chartData)}
						<ChartTooltip
							animationEasing="ease-out"
							animationDuration={150}
							// @ts-ignore
							itemSorter={(a, b) => b.value - a.value}
							content={
								<ChartTooltipContent
									truncate={true}
									labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
									contentFormatter={toolTipFormatter}
								/>
							}
						/>
						{colors.map((key) => (
							<Line
								key={key}
								dataKey={key}
								name={key}
								type="monotoneX"
								dot={false}
								strokeWidth={1.5}
								stroke={healthChartData!.colors[key]}
								isAnimationActive={false}
							/>
						))}
					</LineChart>
				</ChartContainer>
			</div>
		)
	}

	// Render uptime chart
	if (isUptimeChart) {
		const colors = Object.keys(uptimeChartData!.colors)
		return (
			<div className="w-full h-full">
				<ChartContainer
					className={cn("h-full w-full absolute bg-card opacity-0 transition-opacity", {
						"opacity-100": yAxisWidth,
					})}
				>
					<LineChart accessibilityLayer data={uptimeChartData!.data} margin={chartMargin}>
						<CartesianGrid vertical={false} />
						<YAxis
							direction="ltr"
							orientation={chartData.orientation}
							className="tracking-tighter"
							domain={[0, "auto"]}
							width={yAxisWidth}
							tickFormatter={tickFormatter}
							tickLine={false}
							axisLine={false}
						/>
						{xAxis(chartData)}
						<ChartTooltip
							animationEasing="ease-out"
							animationDuration={150}
							// @ts-ignore
							itemSorter={(a, b) => b.value - a.value}
							content={
								<ChartTooltipContent
									truncate={true}
									labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
									contentFormatter={toolTipFormatter}
								/>
							}
						/>
						{colors.map((key) => (
							<Line
								key={key}
								dataKey={key}
								name={key}
								type="monotoneX"
								dot={false}
								strokeWidth={1.5}
								stroke={uptimeChartData!.colors[key]}
								isAnimationActive={false}
							/>
						))}
					</LineChart>
				</ChartContainer>
			</div>
		)
	}

	// Render regular container chart (Area chart)
	return (
		<div className="w-full h-full">
			<ChartContainer
				className={cn("h-full w-full absolute bg-card opacity-0 transition-opacity", {
					"opacity-100": yAxisWidth,
				})}
			>
				<AreaChart
					accessibilityLayer
					data={containerData}
					margin={chartMargin}
					reverseStackOrder={true}
				>
					<CartesianGrid vertical={false} />
					<YAxis
						direction="ltr"
						orientation={chartData.orientation}
						className="tracking-tighter"
						width={yAxisWidth}
						tickFormatter={tickFormatter}
						tickLine={false}
						axisLine={false}
					/>
					{xAxis(chartData)}
					<ChartTooltip
						animationEasing="ease-out"
						animationDuration={150}
						truncate={true}
						labelFormatter={(_, data) => formatShortDate(data[0].payload.created)}
						// @ts-ignore
						itemSorter={(a, b) => b.value - a.value}
						content={<ChartTooltipContent contentFormatter={toolTipFormatter} />}
					/>
					{visibleKeys.map((key) => {
						const filtered = Array.isArray(filter) && filter.length > 0 && !filter.includes(key)
						let fillOpacity = filtered ? 0.05 : 0.4
						let strokeOpacity = filtered ? 0.1 : 1
						return (
							<Area
								key={key}
								isAnimationActive={false}
								dataKey={dataFunction.bind(null, key)}
								name={key}
								type="monotoneX"
								fill={chartConfig[key].color}
								fillOpacity={fillOpacity}
								stroke={chartConfig[key].color}
								strokeOpacity={strokeOpacity}
								activeDot={{ opacity: filtered ? 0 : 1 }}
								stackId="a"
							/>
						)
					})}
				</AreaChart>
			</ChartContainer>
		</div>
	)
})
