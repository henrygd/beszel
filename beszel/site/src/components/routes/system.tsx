import { $systems, pb, $chartTime } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord, SystemStatsRecord } from '@/types'
import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'
import Spinner from '../spinner'
import { ClockArrowUp, CpuIcon, GlobeIcon, LayoutGridIcon } from 'lucide-react'
import ChartTimeSelect from '../charts/chart-time-select'
import {
	chartTimeData,
	cn,
	getPbTimestamp,
	useClampedIsInViewport,
	useLocalStorage,
} from '@/lib/utils'
import { Separator } from '../ui/separator'
import { scaleTime } from 'd3-scale'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../ui/tooltip'
import { Toggle } from '../ui/toggle'
import { buttonVariants } from '../ui/button'

const CpuChart = lazy(() => import('../charts/cpu-chart'))
const ContainerCpuChart = lazy(() => import('../charts/container-cpu-chart'))
const MemChart = lazy(() => import('../charts/mem-chart'))
const ContainerMemChart = lazy(() => import('../charts/container-mem-chart'))
const DiskChart = lazy(() => import('../charts/disk-chart'))
const DiskIoChart = lazy(() => import('../charts/disk-io-chart'))
const BandwidthChart = lazy(() => import('../charts/bandwidth-chart'))
const ContainerNetChart = lazy(() => import('../charts/container-net-chart'))
const SwapChart = lazy(() => import('../charts/swap-chart'))
const TemperatureChart = lazy(() => import('../charts/temperature-chart'))

export default function SystemDetail({ name }: { name: string }) {
	const systems = useStore($systems)
	const chartTime = useStore($chartTime)
	const [grid, setGrid] = useLocalStorage('grid', true)
	const [ticks, setTicks] = useState([] as number[])
	const [system, setSystem] = useState({} as SystemRecord)
	const [systemStats, setSystemStats] = useState([] as SystemStatsRecord[])
	const [hasDockerStats, setHasDocker] = useState(false)
	const [dockerCpuChartData, setDockerCpuChartData] = useState<Record<string, number | string>[]>(
		[]
	)
	const [dockerMemChartData, setDockerMemChartData] = useState<Record<string, number | string>[]>(
		[]
	)
	const [dockerNetChartData, setDockerNetChartData] = useState<Record<string, number | number[]>[]>(
		[]
	)

	useEffect(() => {
		document.title = `${name} / Beszel`
		return () => {
			resetCharts()
			$chartTime.set('1h')
			setHasDocker(false)
		}
	}, [name])

	function resetCharts() {
		setSystemStats([])
		setDockerCpuChartData([])
		setDockerMemChartData([])
		setDockerNetChartData([])
	}

	useEffect(resetCharts, [chartTime])

	useEffect(() => {
		if (system.id && system.name === name) {
			return
		}
		const matchingSystem = systems.find((s) => s.name === name) as SystemRecord
		if (matchingSystem) {
			setSystem(matchingSystem)
		}
	}, [name, system, systems])

	// update system when new data is available
	useEffect(() => {
		if (!system.id) {
			return
		}
		pb.collection<SystemRecord>('systems').subscribe(system.id, (e) => {
			setSystem(e.record)
		})
		return () => {
			pb.collection('systems').unsubscribe(system.id)
		}
	}, [system])

	async function getStats<T>(collection: string): Promise<T[]> {
		return await pb.collection<T>(collection).getFullList({
			filter: pb.filter('system={:id} && created > {:created} && type={:type}', {
				id: system.id,
				created: getPbTimestamp(chartTime),
				type: chartTimeData[chartTime].type,
			}),
			fields: 'created,stats',
			sort: 'created',
		})
	}

	// add empty values between records to make gaps if interval is too large
	function addEmptyValues<T extends SystemStatsRecord | ContainerStatsRecord>(
		records: T[],
		expectedInterval: number
	) {
		const modifiedRecords: T[] = []
		let prevTime = 0
		for (let i = 0; i < records.length; i++) {
			const record = records[i]
			record.created = new Date(record.created).getTime()
			if (prevTime) {
				const interval = record.created - prevTime
				// if interval is too large, add a null record
				if (interval - interval * 0.5 > expectedInterval) {
					// @ts-ignore
					modifiedRecords.push({ created: null, stats: null })
				}
			}
			prevTime = record.created
			modifiedRecords.push(record)
		}
		return modifiedRecords
	}

	// get stats
	useEffect(() => {
		if (!system.id || !chartTime) {
			return
		}
		Promise.allSettled([
			getStats<SystemStatsRecord>('system_stats'),
			getStats<ContainerStatsRecord>('container_stats'),
		]).then(([systemStats, containerStats]) => {
			const expectedInterval = chartTimeData[chartTime].expectedInterval
			if (containerStats.status === 'fulfilled' && containerStats.value.length) {
				makeContainerData(addEmptyValues(containerStats.value, expectedInterval))
				setHasDocker(true)
			}
			if (systemStats.status === 'fulfilled') {
				setSystemStats(addEmptyValues(systemStats.value, expectedInterval))
			}
		})
	}, [system, chartTime])

	useEffect(() => {
		if (!systemStats.length) {
			return
		}
		const now = new Date()
		const startTime = chartTimeData[chartTime].getOffset(now)
		const scale = scaleTime([startTime.getTime(), now], [0, systemStats.length])
		setTicks(scale.ticks(chartTimeData[chartTime].ticks).map((d) => d.getTime()))
	}, [chartTime, systemStats])

	// make container stats for charts
	const makeContainerData = useCallback((containers: ContainerStatsRecord[]) => {
		// console.log('containers', containers)
		const dockerCpuData = []
		const dockerMemData = []
		const dockerNetData = []
		for (let { created, stats } of containers) {
			if (!created) {
				let nullData = { time: null } as unknown
				dockerCpuData.push(nullData as Record<string, number | string>)
				dockerMemData.push(nullData as Record<string, number | string>)
				dockerNetData.push(nullData as Record<string, number | number[]>)
				continue
			}
			const time = new Date(created).getTime()
			let cpuData = { time } as Record<string, number | string>
			let memData = { time } as Record<string, number | string>
			let netData = { time } as Record<string, number | number[]>
			for (let container of stats) {
				cpuData[container.n] = container.c
				memData[container.n] = container.m
				netData[container.n] = [container.ns, container.nr, container.ns + container.nr] // sent, received, total
			}
			dockerCpuData.push(cpuData)
			dockerMemData.push(memData)
			dockerNetData.push(netData)
		}
		setDockerCpuChartData(dockerCpuData)
		setDockerMemChartData(dockerMemData)
		setDockerNetChartData(dockerNetData)
	}, [])

	const uptime = useMemo(() => {
		let uptime = system.info?.u || 0
		if (uptime < 172800) {
			return `${Math.trunc(uptime / 3600)} hours`
		}
		return `${Math.trunc(system.info?.u / 86400)} days`
	}, [system.info?.u])

	if (!system.id) {
		return null
	}

	return (
		<div className="grid lg:grid-cols-2 gap-4 mb-10">
			<Card className="col-span-full">
				<div className="grid lg:flex items-center gap-4 px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
					<div>
						<h1 className="text-[1.6rem] font-semibold mb-1.5">{system.name}</h1>
						<div className="flex flex-wrap items-center gap-3 gap-y-2 text-sm opacity-90">
							<div className="capitalize flex gap-2 items-center">
								<span className={cn('relative flex h-3 w-3')}>
									{system.status === 'up' && (
										<span
											className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"
											style={{ animationDuration: '1.5s' }}
										></span>
									)}
									<span
										className={cn('relative inline-flex rounded-full h-3 w-3', {
											'bg-green-500': system.status === 'up',
											'bg-red-500': system.status === 'down',
											'bg-primary/40': system.status === 'paused',
											'bg-yellow-500': system.status === 'pending',
										})}
									></span>
								</span>
								{system.status}
							</div>
							<Separator orientation="vertical" className="h-4 bg-primary/30" />
							<div className="flex gap-1.5">
								<GlobeIcon className="h-4 w-4 mt-[1px]" /> {system.host}
							</div>
							{system.info?.u && (
								<TooltipProvider>
									<Tooltip>
										<Separator orientation="vertical" className="h-4 bg-primary/30" />
										<TooltipTrigger asChild>
											<div className="flex gap-1.5">
												<ClockArrowUp className="h-4 w-4 mt-[1px]" /> {uptime}
											</div>
										</TooltipTrigger>
										<TooltipContent>Uptime</TooltipContent>
									</Tooltip>
								</TooltipProvider>
							)}
							{system.info?.m && (
								<>
									<Separator orientation="vertical" className="h-4 bg-primary/30" />
									<div className="flex gap-1.5">
										<CpuIcon className="h-4 w-4 mt-[1px]" />
										{system.info.m} ({system.info.c}c / {system.info.t}t)
									</div>
								</>
							)}
						</div>
					</div>
					<div className="lg:ml-auto flex items-center gap-2 max-sm:-mb-1">
						<ChartTimeSelect className="w-full lg:w-40" />
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild className="hidden lg:block opacity-85">
									<span>
										<Toggle
											aria-label="Toggle grid"
											className={cn(
												'p-0 border border-card',
												buttonVariants({ variant: 'ghost', size: 'icon' })
											)}
											pressed={grid}
											onPressedChange={setGrid}
										>
											<LayoutGridIcon className="h-[1.2rem] w-[1.2rem]" />
										</Toggle>
									</span>
								</TooltipTrigger>
								<TooltipContent>Toggle grid</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					</div>
				</div>
			</Card>

			<ChartCard
				grid={grid}
				title="Total CPU Usage"
				description="Average system-wide CPU utilization"
			>
				<CpuChart ticks={ticks} systemData={systemStats} />
			</ChartCard>

			{hasDockerStats && (
				<ChartCard
					grid={grid}
					title="Docker CPU Usage"
					description="CPU utilization of docker containers"
				>
					<ContainerCpuChart chartData={dockerCpuChartData} ticks={ticks} />
				</ChartCard>
			)}

			<ChartCard
				grid={grid}
				title="Total Memory Usage"
				description="Precise utilization at the recorded time"
			>
				<MemChart ticks={ticks} systemData={systemStats} />
			</ChartCard>

			{hasDockerStats && (
				<ChartCard
					grid={grid}
					title="Docker Memory Usage"
					description="Memory usage of docker containers"
				>
					<ContainerMemChart chartData={dockerMemChartData} ticks={ticks} />
				</ChartCard>
			)}

			{(systemStats.at(-1)?.stats.s ?? 0) > 0 && (
				<ChartCard grid={grid} title="Swap Usage" description="Swap space used by the system">
					<SwapChart ticks={ticks} systemData={systemStats} />
				</ChartCard>
			)}

			<ChartCard grid={grid} title="Disk Space" description="Usage of root partition">
				<DiskChart ticks={ticks} systemData={systemStats} />
			</ChartCard>

			{systemStats.at(-1)?.stats.t && (
				<ChartCard grid={grid} title="Temperature" description="Temperatures of system sensors">
					<TemperatureChart ticks={ticks} systemData={systemStats} />
				</ChartCard>
			)}

			<ChartCard grid={grid} title="Disk I/O" description="Throughput of root filesystem">
				<DiskIoChart ticks={ticks} systemData={systemStats} />
			</ChartCard>

			<ChartCard grid={grid} title="Bandwidth" description="Network traffic of public interfaces">
				<BandwidthChart ticks={ticks} systemData={systemStats} />
			</ChartCard>

			{hasDockerStats && dockerNetChartData.length > 0 && (
				<>
					<ChartCard
						grid={grid}
						title="Docker Network I/O"
						description="Includes traffic between internal services"
					>
						<ContainerNetChart chartData={dockerNetChartData} ticks={ticks} />
					</ChartCard>
					{/* add space for tooltip if more than 12 containers */}
					{Object.keys(dockerNetChartData[0]).length > 12 && (
						<span
							className="block"
							style={{
								height: (Object.keys(dockerNetChartData[0]).length - 13) * 18,
							}}
						/>
					)}
				</>
			)}
		</div>
	)
}

function ChartCard({
	title,
	description,
	children,
	grid,
}: {
	title: string
	description: string
	children: React.ReactNode
	grid: boolean
}) {
	const target = useRef<HTMLDivElement>(null)
	const [isInViewport, wrappedTargetRef] = useClampedIsInViewport({ target: target })
	return (
		<Card
			className={cn('pb-2 sm:pb-4 even:last-of-type:col-span-full', { 'col-span-full': !grid })}
			ref={wrappedTargetRef}
		>
			<CardHeader className="pb-5 pt-4 relative space-y-1 max-sm:py-3 max-sm:px-4">
				<CardTitle className="text-xl sm:text-2xl">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
			</CardHeader>
			<CardContent className="pl-0 w-[calc(100%-1.6em)] h-52 relative">
				{<Spinner />}
				{isInViewport && <Suspense>{children}</Suspense>}
			</CardContent>
		</Card>
	)
}
