import { $systems, pb, $chartTime, $containerFilter, $userSettings } from '@/lib/stores'
import { ContainerStatsRecord, SystemRecord, SystemStatsRecord } from '@/types'
import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { useStore } from '@nanostores/react'
import Spinner from '../spinner'
import { ClockArrowUp, CpuIcon, GlobeIcon, LayoutGridIcon, MonitorIcon, XIcon } from 'lucide-react'
import ChartTimeSelect from '../charts/chart-time-select'
import { chartTimeData, cn, getPbTimestamp, useLocalStorage } from '@/lib/utils'
import { Separator } from '../ui/separator'
import { scaleTime } from 'd3-scale'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../ui/tooltip'
import { Button, buttonVariants } from '../ui/button'
import { Input } from '../ui/input'
import { Rows, TuxIcon } from '../ui/icons'
import { useIntersectionObserver } from '@/lib/use-intersection-observer'

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
	const netCardRef = useRef<HTMLDivElement>(null)
	const [dockerCpuChartData, setDockerCpuChartData] = useState<Record<string, number | string>[]>(
		[]
	)
	const [dockerMemChartData, setDockerMemChartData] = useState<Record<string, number | string>[]>(
		[]
	)
	const [dockerNetChartData, setDockerNetChartData] = useState<Record<string, number | number[]>[]>(
		[]
	)
	const hasDockerStats = dockerCpuChartData.length > 0

	useEffect(() => {
		document.title = `${name} / Beszel`
		return () => {
			resetCharts()
			$chartTime.set($userSettings.get().chartTime)
			$containerFilter.set('')
			// setHasDocker(false)
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
				if (interval > expectedInterval / 2 + expectedInterval) {
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

	// values for system info bar
	const systemInfo = useMemo(() => {
		if (!system.info) {
			return []
		}
		let uptime: number | string = system.info.u
		if (system.info.u < 172800) {
			const hours = Math.trunc(uptime / 3600)
			uptime = `${hours} hour${hours > 1 ? 's' : ''}`
		} else {
			uptime = `${Math.trunc(system.info?.u / 86400)} days`
		}
		return [
			{ value: system.host, Icon: GlobeIcon },
			{
				value: system.info.h,
				Icon: MonitorIcon,
				label: 'Hostname',
				// hide if hostname is same as host or name
				hide: system.info.h === system.host || system.info.h === system.name,
			},
			{ value: uptime, Icon: ClockArrowUp, label: 'Uptime' },
			{ value: system.info.k, Icon: TuxIcon, label: 'Kernel' },
			{
				value: `${system.info.m} (${system.info.c}c${system.info.t ? `/${system.info.t}t` : ''})`,
				Icon: CpuIcon,
				hide: !system.info.m,
			},
		] as {
			value: string | number | undefined
			label?: string
			Icon: any
			hide?: boolean
		}[]
	}, [system.info])

	/** Space for tooltip if more than 12 containers */
	const bottomSpacing = useMemo(() => {
		if (!netCardRef.current || !dockerNetChartData.length) {
			return 0
		}
		const tooltipHeight = (Object.keys(dockerNetChartData[0]).length - 11) * 17.8 - 40
		const wrapperEl = document.getElementById('chartwrap') as HTMLDivElement
		const wrapperRect = wrapperEl.getBoundingClientRect()
		const chartRect = netCardRef.current.getBoundingClientRect()
		const distanceToBottom = wrapperRect.bottom - chartRect.bottom
		return tooltipHeight - distanceToBottom
	}, [netCardRef.current, dockerNetChartData])

	if (!system.id) {
		return null
	}

	return (
		<>
			<div id="chartwrap" className="grid gap-4 mb-10">
				{/* system info */}
				<Card>
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
								{systemInfo.map(({ value, label, Icon, hide }, i) => {
									if (hide || !value) {
										return null
									}
									const content = (
										<div className="flex gap-1.5 items-center">
											<Icon className="h-4 w-4" /> {value}
										</div>
									)
									return (
										<div key={i} className="contents">
											<Separator orientation="vertical" className="h-4 bg-primary/30" />
											{label ? (
												<TooltipProvider>
													<Tooltip delayDuration={150}>
														<TooltipTrigger asChild>{content}</TooltipTrigger>
														<TooltipContent>{label}</TooltipContent>
													</Tooltip>
												</TooltipProvider>
											) : (
												content
											)}
										</div>
									)
								})}
							</div>
						</div>
						<div className="lg:ml-auto flex items-center gap-2 max-sm:-mb-1">
							<ChartTimeSelect className="w-full lg:w-40" />
							<TooltipProvider delayDuration={100}>
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											aria-label="Toggle grid"
											className={cn(
												buttonVariants({ variant: 'outline', size: 'icon' }),
												'hidden lg:flex p-0 text-primary'
											)}
											onClick={() => setGrid(!grid)}
										>
											{grid ? (
												<LayoutGridIcon className="h-[1.2rem] w-[1.2rem] opacity-85" />
											) : (
												<Rows className="h-[1.3rem] w-[1.3rem] opacity-85" />
											)}
										</Button>
									</TooltipTrigger>
									<TooltipContent>Toggle grid</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						</div>
					</div>
				</Card>

				{/* main charts */}
				<div className="grid lg:grid-cols-2 gap-4">
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
							isContainerChart={true}
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
							isContainerChart={true}
						>
							<ContainerMemChart chartData={dockerMemChartData} ticks={ticks} />
						</ChartCard>
					)}

					<ChartCard grid={grid} title="Disk Space" description="Usage of root partition">
						<DiskChart
							ticks={ticks}
							systemData={systemStats}
							dataKey="stats.du"
							diskSize={Math.round(systemStats.at(-1)?.stats.d ?? NaN)}
						/>
					</ChartCard>

					<ChartCard grid={grid} title="Disk I/O" description="Throughput of root filesystem">
						<DiskIoChart
							ticks={ticks}
							systemData={systemStats}
							dataKeys={['stats.dw', 'stats.dr']}
						/>
					</ChartCard>

					<ChartCard
						grid={grid}
						title="Bandwidth"
						description="Network traffic of public interfaces"
					>
						<BandwidthChart ticks={ticks} systemData={systemStats} />
					</ChartCard>

					{hasDockerStats && dockerNetChartData.length > 0 && (
						<div
							ref={netCardRef}
							className={cn({
								'col-span-full': !grid,
							})}
						>
							<ChartCard
								title="Docker Network I/O"
								description="Includes traffic between internal services"
								isContainerChart={true}
							>
								<ContainerNetChart chartData={dockerNetChartData} ticks={ticks} />
							</ChartCard>
						</div>
					)}

					{(systemStats.at(-1)?.stats.su ?? 0) > 0 && (
						<ChartCard grid={grid} title="Swap Usage" description="Swap space used by the system">
							<SwapChart ticks={ticks} systemData={systemStats} />
						</ChartCard>
					)}

					{systemStats.at(-1)?.stats.t && (
						<ChartCard grid={grid} title="Temperature" description="Temperatures of system sensors">
							<TemperatureChart ticks={ticks} systemData={systemStats} />
						</ChartCard>
					)}
				</div>

				{/* extra filesystem charts */}
				{Object.keys(systemStats.at(-1)?.stats.efs ?? {}).length > 0 && (
					<div className="grid lg:grid-cols-2 gap-4">
						{Object.keys(systemStats.at(-1)?.stats.efs ?? {}).map((extraFsName) => {
							return (
								<div key={extraFsName} className="contents">
									<ChartCard
										grid={grid}
										title={`${extraFsName} Usage`}
										description={`Disk usage of ${extraFsName}`}
									>
										<DiskChart
											ticks={ticks}
											systemData={systemStats}
											dataKey={`stats.efs.${extraFsName}.du`}
											diskSize={Math.round(systemStats.at(-1)?.stats.efs?.[extraFsName].d ?? NaN)}
										/>
									</ChartCard>
									<ChartCard
										grid={grid}
										title={`${extraFsName} I/O`}
										description={`Throughput of ${extraFsName}`}
									>
										<DiskIoChart
											ticks={ticks}
											systemData={systemStats}
											dataKeys={[`stats.efs.${extraFsName}.w`, `stats.efs.${extraFsName}.r`]}
										/>
									</ChartCard>
								</div>
							)
						})}
					</div>
				)}
			</div>

			{/* add space for tooltip if more than 12 containers */}
			{bottomSpacing > 0 && <span className="block" style={{ height: bottomSpacing }} />}
		</>
	)
}

function ContainerFilterBar() {
	const containerFilter = useStore($containerFilter)

	const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
		$containerFilter.set(e.target.value)
	}, []) // Use an empty dependency array to prevent re-creation

	return (
		<div className="relative py-1 block sm:w-44 sm:absolute sm:top-2.5 sm:right-3.5">
			<Input
				placeholder="Filter..."
				className="pl-4 pr-8"
				value={containerFilter}
				onChange={handleChange}
			/>
			{containerFilter && (
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label="Clear"
					className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
					onClick={() => $containerFilter.set('')}
				>
					<XIcon className="h-4 w-4" />
				</Button>
			)}
		</div>
	)
}

function ChartCard({
	title,
	description,
	children,
	grid,
	isContainerChart,
}: {
	title: string
	description: string
	children: React.ReactNode
	grid?: boolean
	isContainerChart?: boolean
}) {
	const { isIntersecting, ref } = useIntersectionObserver()

	return (
		<Card
			className={cn('pb-2 sm:pb-4 odd:last-of-type:col-span-full', { 'col-span-full': !grid })}
			ref={ref}
		>
			<CardHeader className="pb-5 pt-4 relative space-y-1 max-sm:py-3 max-sm:px-4">
				<CardTitle className="text-xl sm:text-2xl">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
				{isContainerChart && <ContainerFilterBar />}
			</CardHeader>
			<CardContent className="pl-0 w-[calc(100%-1.6em)] h-52 relative">
				{<Spinner />}
				{isIntersecting && <Suspense>{children}</Suspense>}
			</CardContent>
		</Card>
	)
}
