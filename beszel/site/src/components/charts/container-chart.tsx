import { Area, AreaChart, CartesianGrid, YAxis, Line, LineChart } from "recharts"
import { ChartContainer, ChartTooltip, ChartTooltipContent, xAxis } from "@/components/ui/chart"
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
import { $containerFilter, $stackFilter, $containerColors } from "@/lib/stores"
import { ChartData } from "@/types"
import { Separator } from "../ui/separator"
import { ChartType } from "@/lib/enums"
import React from "react"
import { Badge } from "@/components/ui/badge"


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
	const containerFilter = useStore($containerFilter)
	const stackFilter = useStore($stackFilter)
	const containerColors = useStore($containerColors)
	const { yAxisWidth, updateYAxisWidth } = useYAxisWidth()

	const { containerData } = chartData

	const isNetChart = chartType === ChartType.Network
	const isVolumeChart = chartType === ChartType.Volume
	const isHealthChart = chartType === ChartType.Health
	const isUptimeChart = chartType === ChartType.Uptime
	const isHealthUptimeChart = chartType === ChartType.HealthUptime
	const isDiskIOChart = chartType === ChartType.DiskIO

	// Centralized data processing for all chart types
	const chartDatasets = useMemo(() => {
		const volumeChartData = { data: [], colors: {} } as { data: Record<string, number | string>[]; colors: Record<string, string> }
		const healthChartData = { data: [], colors: {} } as { data: Record<string, number | string>[]; colors: Record<string, string> }
		const uptimeChartData = { data: [], colors: {} } as { data: Record<string, number | string>[]; colors: Record<string, string> }
		const healthUptimeChartData = { data: [], colors: {} } as { data: Record<string, number | string>[]; colors: Record<string, string> }
		const chartConfig = {} as Record<string, { label: string; color: string }>

		const volumeSums: Record<string, number> = {}
		const volumeContainers: Record<string, string[]> = {}
		const allContainerNames = new Set<string>()
		const healthUptimeContainerNames = new Set<string>()

		for (let containerStats of containerData) {
			if (!containerStats.created) {
				// For gaps in data
				volumeChartData.data.push({ created: "" })
				healthChartData.data.push({ created: "" })
				uptimeChartData.data.push({ created: "" })
				healthUptimeChartData.data.push({ created: "" })
				continue
			}

			let volumeData = { created: containerStats.created } as Record<string, number | string>
			let healthData = { created: containerStats.created } as Record<string, number | string>
			let uptimeData = { created: containerStats.created } as Record<string, number | string>
			let healthUptimeData = { created: containerStats.created } as Record<string, number | string>

			for (let [containerName, containerDataObj] of Object.entries(containerStats)) {
				if (containerName === "created") continue
				
				// Apply container filter
				if (containerFilter.length > 0 && !containerFilter.includes(containerName)) {
					continue
				}
				
				// Apply stack filter
				if (stackFilter.length > 0 && typeof containerDataObj === 'object' && containerDataObj) {
					const stackName = (containerDataObj as any).p || "—"
					if (!stackFilter.includes(stackName)) {
						continue
					}
				}
				
				allContainerNames.add(containerName)

				if (typeof containerDataObj === 'object' && containerDataObj) {
					// Volume
					if ('v' in containerDataObj && containerDataObj.v) {
						for (let [volumeName, volumeSize] of Object.entries(containerDataObj.v)) {
							if (typeof volumeSize === 'number' && volumeSize > 0) {
								volumeData[volumeName] = (volumeData[volumeName] as number || 0) + volumeSize
								volumeSums[volumeName] = (volumeSums[volumeName] ?? 0) + volumeSize
								if (!volumeContainers[volumeName]) volumeContainers[volumeName] = []
								if (!volumeContainers[volumeName].includes(containerName)) volumeContainers[volumeName].push(containerName)
							}
						}
					}
					// Health
					if ('h' in containerDataObj) {
						const healthStatus = (containerDataObj.h as string || '').toLowerCase()
						let healthValue = 0
						switch (healthStatus) {
							case 'healthy': healthValue = 3; break
							case 'starting': healthValue = 2; break
							case 'unhealthy': healthValue = 1; break
							default: healthValue = 0
						}
						healthData[containerName] = healthValue
						// Health+Uptime
						healthUptimeData[containerName + "_health"] = healthValue
						healthUptimeContainerNames.add(containerName)
					}
					// Uptime
					if ('u' in containerDataObj && containerDataObj.u) {
						uptimeData[containerName] = (containerDataObj.u as number) / 3600
						// Health+Uptime
						healthUptimeData[containerName + "_uptime"] = (containerDataObj.u as number) / 3600
						healthUptimeContainerNames.add(containerName)
					}
				}
			}
			volumeChartData.data.push(volumeData)
			healthChartData.data.push(healthData)
			uptimeChartData.data.push(uptimeData)
			healthUptimeChartData.data.push(healthUptimeData)
		}

		// Only process volumes attached to containers
		const volumeKeys = Object.keys(volumeSums)
			.filter(key => (volumeContainers[key] || []).length > 0)
			.sort((a, b) => volumeSums[b] - volumeSums[a])
		for (let key of volumeKeys) {
			const containers = volumeContainers[key] || []
			const firstContainer = containers[0]
			volumeChartData.colors[key] = containerColors[firstContainer] || `hsl(${Math.random() * 360}, 60%, 55%)`
		}
		const healthKeys = Object.keys(healthChartData.data[0] || {}).filter(key => key !== "created")
		for (let key of healthKeys) {
			healthChartData.colors[key] = containerColors[key] || `hsl(${((healthKeys.indexOf(key) * 360) / healthKeys.length) % 360}, 60%, 55%)`
		}
		const uptimeKeys = Object.keys(uptimeChartData.data[0] || {}).filter(key => key !== "created")
		for (let key of uptimeKeys) {
			uptimeChartData.colors[key] = containerColors[key] || `hsl(${((uptimeKeys.indexOf(key) * 360) / uptimeKeys.length) % 360}, 60%, 55%)`
		}
		for (let containerName of healthUptimeContainerNames) {
			const color = containerColors[containerName] || `hsl(${Math.random() * 360}, 60%, 55%)`
			healthUptimeChartData.colors[containerName + "_uptime"] = color
			healthUptimeChartData.colors[containerName + "_health"] = color
		}
		for (let containerName of allContainerNames) {
			const color = containerColors[containerName] || `hsl(${Math.random() * 360}, 60%, 55%)`
			chartConfig[containerName] = { label: containerName, color }
		}

		return {
			volumeChartData,
			healthChartData,
			uptimeChartData,
			healthUptimeChartData,
			chartConfig,
		}
	}, [containerData, containerColors, containerFilter, stackFilter])

	const { volumeChartData, healthChartData, uptimeChartData, healthUptimeChartData, chartConfig } = chartDatasets

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
				return updateYAxisWidth(`${toFixedFloat(v, 2)}${u}${isNetChart || isDiskIOChart ? "/s" : ""}`)
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
		} else if (isDiskIOChart) {
			obj.toolTipFormatter = (item: any, key: string) => {
				try {
					const read = item?.payload?.[key]?.dr ?? 0
					const write = item?.payload?.[key]?.dw ?? 0
					return (
						<span className="flex">
							{decimalString(read)} MB/s
							<span className="opacity-70 ms-0.5"> read </span>
							<Separator orientation="vertical" className="h-3 mx-1.5 bg-primary/40" />
							{decimalString(write)} MB/s
							<span className="opacity-70 ms-0.5"> write</span>
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
		} else if (isDiskIOChart) {
			obj.dataFunction = (key: string, data: any) => (data[key] ? (data[key].dr || 0) + (data[key].dw || 0) : null)
		} else {
			obj.dataFunction = (key: string, data: any) => data[key]?.[dataKey] ?? null
		}
		return obj
	}, [chartType, isNetChart, isVolumeChart, isDiskIOChart, unit])

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
		return (
			<HealthUptimeTable
				containerData={containerData}
				healthUptimeChartData={healthUptimeChartData}
				containerColors={containerColors}
				filter={containerFilter}
			/>
		)
	}

	// Only show selected containers, or all if none selected
	const filterableKeys = isVolumeChart
		? Object.keys(chartConfig)
		: Object.keys(chartConfig).filter(
			(key) =>
				!(chartConfig[key] && chartConfig[key].label && chartConfig[key].label.startsWith("(orphaned volume)"))
		);

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
						{colors.map((key) => (
							<Area
								key={key}
								dataKey={key}
								name={key}
								type="monotoneX"
								fill={volumeChartData!.colors[key]}
								fillOpacity={0.4}
								stroke={volumeChartData!.colors[key]}
								strokeOpacity={1}
								activeDot={{ opacity: 1 }}
								stackId="a"
								isAnimationActive={false}
							/>
						))}
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
					{filterableKeys.map((key) => (
						<Area
							key={key}
							isAnimationActive={false}
							dataKey={dataFunction.bind(null, key)}
							name={key}
							type="monotoneX"
							fill={chartConfig[key].color}
							fillOpacity={0.4}
							stroke={chartConfig[key].color}
							strokeOpacity={1}
							activeDot={{ opacity: 1 }}
							stackId="a"
						/>
					))}
				</AreaChart>
			</ChartContainer>
		</div>
	)
})

// Extracted HealthUptimeTable component
const HealthUptimeTable = React.memo(function HealthUptimeTable({
	containerData,
	healthUptimeChartData,
	containerColors,
	filter
}: {
	containerData: any[],
	healthUptimeChartData: { data: any[]; colors: Record<string, string> },
	containerColors: Record<string, string>,
	filter: string[]
}) {
	const stackFilter = useStore($stackFilter)
	// Get the latest data point for table display
	const latestData = healthUptimeChartData.data[healthUptimeChartData.data.length - 1]
	if (!latestData) return null

	// Extract container data for table
	const containerTableData = React.useMemo(() => {
		const containerNames = new Set<string>()
		for (const key of Object.keys(latestData)) {
			if (key === 'created') continue
			const containerName = key.replace(/_uptime$/, '').replace(/_health$/, '')
			// Skip orphaned volumes
			if (containerName.startsWith('(orphaned volume)')) continue
			containerNames.add(containerName)
		}
		const tableData = []
		for (const containerName of containerNames) {
			const uptimeKey = containerName + '_uptime'
			const healthKey = containerName + '_health'
			const uptimeHours = latestData[uptimeKey]
			const healthValue = latestData[healthKey] || 0
			let healthStatus = 'Unknown'
			switch (healthValue) {
				case 3: healthStatus = 'Healthy'; break
				case 2: healthStatus = 'Starting'; break
				case 1: healthStatus = 'Unhealthy'; break
				case 0: healthStatus = 'None'; break
			}
			let uptimeDisplay = 'N/A'
			if (typeof uptimeHours === 'number' && !isNaN(uptimeHours)) {
				const hours = Math.floor(uptimeHours)
				const minutes = Math.floor((uptimeHours - hours) * 60)
				const days = Math.floor(hours / 24)
				const remainingHours = hours % 24
				if (days > 0) {
					uptimeDisplay = `${days}d ${remainingHours}h ${minutes}m`
				} else {
					uptimeDisplay = `${hours}h ${minutes}m`
				}
			}
			let stackName = '—'
			let statusInfo = '—'
			let idShort = ''
			for (let i = containerData.length - 1; i >= 0; i--) {
				const containerStats = containerData[i]
				if (containerStats.created && containerStats[containerName]) {
					const containerDataObj = containerStats[containerName]
					if (typeof containerDataObj === 'object' && containerDataObj) {
						if ('p' in containerDataObj) {
							stackName = containerDataObj.p as string
						}
						if ('s' in containerDataObj) {
							statusInfo = containerDataObj.s as string
						}
						if ('idShort' in containerDataObj) {
							idShort = containerDataObj.idShort as string
						}
						break
					}
				}
			}
			tableData.push({
				name: containerName,
				idShort,
				health: healthStatus,
				status: statusInfo,
				uptime: uptimeDisplay,
				uptimeHours: uptimeHours,
				healthValue: healthValue,
				stack: stackName,
				color: containerColors[containerName] || `hsl(${Math.random() * 360}, 60%, 55%)`
			})
		}
		return tableData
	}, [containerData, latestData, containerColors])

	// Sort and filter state
	const [sortField, setSortField] = React.useState<'name' | 'idShort' | 'stack' | 'health' | 'status' | 'uptime'>('uptime')
	const [sortDirection, setSortDirection] = React.useState<'asc' | 'desc'>('desc')
	const [currentPage, setCurrentPage] = React.useState(1)
	const containersPerPage = 4

	// Filtered data
	const filteredContainerData = React.useMemo(() => {
		let filtered = containerTableData
		
		// Apply container filter
		if (Array.isArray(filter) && filter.length > 0) {
			filtered = filtered.filter(container => filter.includes(container.name))
		}
		
		// Apply stack filter
		if (Array.isArray(stackFilter) && stackFilter.length > 0) {
			filtered = filtered.filter(container => stackFilter.includes(container.stack))
		}
		
		return filtered
	}, [containerTableData, filter, stackFilter])

	// Sorted data
	const sortedContainerData = React.useMemo(() => {
		return [...filteredContainerData].sort((a, b) => {
			let aValue: string | number
			let bValue: string | number
			switch (sortField) {
				case 'name':
					aValue = a.name?.toLowerCase?.() || ''
					bValue = b.name?.toLowerCase?.() || ''
					break
				case 'idShort':
					aValue = a.idShort || ''
					bValue = b.idShort || ''
					break
				case 'stack':
					aValue = a.stack?.toLowerCase?.() || ''
					bValue = b.stack?.toLowerCase?.() || ''
					break
				case 'health':
					aValue = typeof a.healthValue === 'number' ? a.healthValue : 0
					bValue = typeof b.healthValue === 'number' ? b.healthValue : 0
					break
				case 'status':
					aValue = a.status?.toLowerCase?.() || ''
					bValue = b.status?.toLowerCase?.() || ''
					break
				case 'uptime':
					aValue = typeof a.uptimeHours === 'number' ? a.uptimeHours : 0
					bValue = typeof b.uptimeHours === 'number' ? b.uptimeHours : 0
					break
				default:
					return 0
			}
			if (aValue < bValue) return sortDirection === 'asc' ? -1 : 1
			if (aValue > bValue) return sortDirection === 'asc' ? 1 : -1
			return 0
		})
	}, [filteredContainerData, sortField, sortDirection])

	// Pagination
	const totalPages = Math.ceil(sortedContainerData.length / containersPerPage)
	const startIndex = (currentPage - 1) * containersPerPage
	const endIndex = startIndex + containersPerPage
	const currentContainers = sortedContainerData.slice(startIndex, endIndex)

	// Handlers
	const handleSort = (field: 'name' | 'idShort' | 'stack' | 'health' | 'status' | 'uptime') => {
		if (sortField === field) {
			setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
		} else {
			setSortField(field)
			setSortDirection('asc')
		}
	}
	const getSortIcon = (field: 'name' | 'idShort' | 'stack' | 'health' | 'status' | 'uptime') => {
		if (sortField !== field) return '↑↓'
		return sortDirection === 'asc' ? '↑' : '↓'
	}

	React.useEffect(() => {
		setCurrentPage(1)
	}, [filteredContainerData, sortField, sortDirection])

	return (
		<div className="w-full h-full flex flex-col opacity-100">
			<div className="flex-1 p-2 overflow-hidden">
				<div className="overflow-x-auto h-full">
					<table className="w-full text-xs table-fixed">
						<thead>
							<tr className="border-b border-border">
								<th className="text-left font-medium p-1 w-1/6 cursor-pointer hover:bg-muted/50 transition-colors select-none" onClick={() => handleSort('idShort')}>
									<div className="flex items-center gap-1">
										ID
										<span className="text-xs opacity-60">{getSortIcon('idShort')}</span>
									</div>
								</th>
								<th className="text-left font-medium p-1 w-1/4 cursor-pointer hover:bg-muted/50 transition-colors select-none" onClick={() => handleSort('name')}>
									<div className="flex items-center gap-1">
										Container
										<span className="text-xs opacity-60">{getSortIcon('name')}</span>
									</div>
								</th>
								<th className="text-left font-medium p-1 w-1/6 cursor-pointer hover:bg-muted/50 transition-colors select-none" onClick={() => handleSort('stack')}>
									<div className="flex items-center gap-1">
										Stack
										<span className="text-xs opacity-60">{getSortIcon('stack')}</span>
									</div>
								</th>
								<th className="text-left font-medium p-1 w-1/6 cursor-pointer hover:bg-muted/50 transition-colors select-none" onClick={() => handleSort('health')}>
									<div className="flex items-center gap-1">
										Health
										<span className="text-xs opacity-60">{getSortIcon('health')}</span>
									</div>
								</th>
								<th className="text-left font-medium p-1 w-1/6 cursor-pointer hover:bg-muted/50 transition-colors select-none" onClick={() => handleSort('status')}>
									<div className="flex items-center gap-1">
										Status
										<span className="text-xs opacity-60">{getSortIcon('status')}</span>
									</div>
								</th>
								<th className="text-left font-medium p-1 w-1/6 cursor-pointer hover:bg-muted/50 transition-colors select-none" onClick={() => handleSort('uptime')}>
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
									<td className="p-1 w-1/6 font-mono text-xs text-muted-foreground" title={container.idShort}>{container.idShort}</td>
									<td className="p-1 w-1/4">
										<div className="flex items-center gap-1.5">
											<div className="w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: container.color }} />
											<span className="text-xs truncate">{container.name}</span>
										</div>
									</td>
									<td className="p-1 w-1/6">
										<span className="text-xs text-muted-foreground truncate" title={container.stack}>{container.stack}</span>
									</td>
									<td className="p-1 w-1/6">
										<Badge className={cn(
											"px-1.5 py-0.5 text-xs font-medium whitespace-nowrap border-0",
											{
												"bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400": container.healthValue === 3,
												"bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400": container.healthValue === 2,
												"bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400": container.healthValue === 1,
												"bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-400": container.healthValue === 0,
											}
										)}>{container.health}</Badge>
									</td>
									<td className="p-1 w-1/6">
										<Badge className={cn(
											"px-1.5 py-0.5 text-xs font-medium whitespace-nowrap border-0",
											{
												"bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400": container.status?.toLowerCase() === "running",
												"bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400": container.status?.toLowerCase() === "paused" || container.status?.toLowerCase() === "restarting",
												"bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400":
													container.status?.toLowerCase().includes("exited") ||
													container.status?.toLowerCase().includes("dead") ||
													container.status?.toLowerCase().includes("removing"),
												"bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-400": !container.status || container.status?.toLowerCase() === "created",
											}
										)} title={container.status}>{container.status}</Badge>
									</td>
									<td className="p-1 w-1/6 text-xs whitespace-nowrap">{container.uptime}</td>
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
						<button onClick={() => setCurrentPage(prev => Math.max(1, prev - 1))} disabled={currentPage === 1} className={cn(
							"px-2 py-1 text-xs rounded border transition-colors",
							"hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed",
							"border-border hover:border-border/60"
						)}>Previous</button>
						{Array.from({ length: totalPages }, (_, i) => i + 1).map(page => (
							<button key={page} onClick={() => setCurrentPage(page)} className={cn(
								"px-2 py-1 text-xs rounded border transition-colors min-w-[28px]",
								page === currentPage
									? "bg-primary text-primary-foreground border-primary"
									: "border-border hover:bg-muted hover:border-border/60"
							)}>{page}</button>
						))}
						<button onClick={() => setCurrentPage(prev => Math.min(totalPages, prev + 1))} disabled={currentPage === totalPages} className={cn(
							"px-2 py-1 text-xs rounded border transition-colors",
							"hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed",
							"border-border hover:border-border/60"
						)}>Next</button>
					</div>
				</div>
			)}
		</div>
	)
})
