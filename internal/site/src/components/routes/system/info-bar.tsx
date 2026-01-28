import { plural } from "@lingui/core/macro"
import { useLingui } from "@lingui/react/macro"
import {
	AppleIcon,
	ChevronRightSquareIcon,
	ClockArrowUp,
	CpuIcon,
	GlobeIcon,
	LayoutGridIcon,
	MemoryStickIcon,
	MonitorIcon,
	Rows,
	TagIcon,
} from "lucide-react"
import { useMemo } from "react"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { FreeBsdIcon, TuxIcon, WebSocketIcon, WindowsIcon } from "@/components/ui/icons"
import { Separator } from "@/components/ui/separator"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import { ConnectionType, connectionTypeLabels, Os, SystemStatus } from "@/lib/enums"
import { cn, formatBytes, getHostDisplayValue, secondsToString, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemDetailsRecord, SystemRecord, TagRecord } from "@/types"
import { getTagColorClasses } from "@/components/tags-columns"

export default function InfoBar({
	system,
	chartData,
	grid,
	setGrid,
	details,
}: {
	system: SystemRecord
	chartData: ChartData
	grid: boolean
	setGrid: (grid: boolean) => void
	details: SystemDetailsRecord | null
}) {
	const { t } = useLingui()

	// values for system info bar - use details with fallback to system.info
	const systemInfo = useMemo(() => {
		if (!system.info) {
			return []
		}

		// Use details if available, otherwise fall back to system.info
		const hostname = details?.hostname ?? system.info.h
		const kernel = details?.kernel ?? system.info.k
		const cores = details?.cores ?? system.info.c
		const threads = details?.threads ?? system.info.t ?? 0
		const cpuModel = details?.cpu ?? system.info.m
		const os = details?.os ?? system.info.os ?? Os.Linux
		const osName = details?.os_name
		const arch = details?.arch
		const memory = details?.memory

		const osInfo = {
			[Os.Linux]: {
				Icon: TuxIcon,
				// show kernel in tooltip if os name is available, otherwise show the kernel
				value: osName || kernel,
				label: osName ? kernel : undefined,
			},
			[Os.Darwin]: {
				Icon: AppleIcon,
				value: osName || `macOS ${kernel}`,
			},
			[Os.Windows]: {
				Icon: WindowsIcon,
				value: osName || kernel,
				label: osName ? kernel : undefined,
			},
			[Os.FreeBSD]: {
				Icon: FreeBsdIcon,
				value: osName || kernel,
				label: osName ? kernel : undefined,
			},
		}

		let uptime: string
		if (system.info.u < 3600) {
			uptime = secondsToString(system.info.u, "minute")
		} else if (system.info.u < 360000) {
			uptime = secondsToString(system.info.u, "hour")
		} else {
			uptime = secondsToString(system.info.u, "day")
		}
		const info = [
			{ value: getHostDisplayValue(system), Icon: GlobeIcon },
			{
				value: hostname,
				Icon: MonitorIcon,
				label: "Hostname",
				// hide if hostname is same as host or name
				hide: hostname === system.host || hostname === system.name,
			},
			{ value: uptime, Icon: ClockArrowUp, label: t`Uptime`, hide: !system.info.u },
			osInfo[os],
			{
				value: cpuModel,
				Icon: CpuIcon,
				hide: !cpuModel,
				label: `${plural(cores, { one: "# core", other: "# cores" })} / ${plural(threads, { one: "# thread", other: "# threads" })}${arch ? ` / ${arch}` : ""}`,
			},
		] as {
			value: string | number | undefined
			label?: string
			Icon: React.ElementType
			hide?: boolean
		}[]

		if (memory) {
			const memValue = formatBytes(memory, false, undefined, false)
			info.push({
				value: `${toFixedFloat(memValue.value, memValue.value >= 10 ? 1 : 2)} ${memValue.unit}`,
				Icon: MemoryStickIcon,
				hide: !memory,
				label: t`Memory`,
			})
		}

		return info
	}, [system, details, t])

	let translatedStatus: string = system.status
	if (system.status === SystemStatus.Up) {
		translatedStatus = t({ message: "Up", comment: "Context: System is up" })
	} else if (system.status === SystemStatus.Down) {
		translatedStatus = t({ message: "Down", comment: "Context: System is down" })
	}

	return (
		<Card>
			<div className="grid xl:flex gap-4 px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
				<div>
					<h1 className="text-[1.6rem] font-semibold mb-1.5">{system.name}</h1>
					<div className="flex flex-wrap items-center gap-3 gap-y-2 text-sm opacity-90">
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<div className="capitalize flex gap-2 items-center">
										<span className={cn("relative flex h-3 w-3")}>
											{system.status === SystemStatus.Up && (
												<span
													className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"
													style={{ animationDuration: "1.5s" }}
												></span>
											)}
											<span
												className={cn("relative inline-flex rounded-full h-3 w-3", {
													"bg-green-500": system.status === SystemStatus.Up,
													"bg-red-500": system.status === SystemStatus.Down,
													"bg-primary/40": system.status === SystemStatus.Paused,
													"bg-yellow-500": system.status === SystemStatus.Pending,
												})}
											></span>
										</span>
										{translatedStatus}
									</div>
								</TooltipTrigger>
								{system.info.ct && (
									<TooltipContent>
										<div className="flex gap-1 items-center">
											{system.info.ct === ConnectionType.WebSocket ? (
												<WebSocketIcon className="size-4" />
											) : (
												<ChevronRightSquareIcon className="size-4" strokeWidth={2} />
											)}
											{connectionTypeLabels[system.info.ct as ConnectionType]}
										</div>
									</TooltipContent>
								)}
							</Tooltip>
						</TooltipProvider>

						{systemInfo.map(({ value, label, Icon, hide }, index) => {
							if (hide || !value) {
								return null
							}
							const content = (
								<div className="flex gap-1.5 items-center">
									<Icon className="h-4 w-4" /> {value}
								</div>
							)
							return (
								<div key={value} className="contents">
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
									{/* Render tags after host/IP (index 0) */}
									{index === 1 && system.expand?.tags && system.expand.tags.length > 0 && (
										<>
											<Separator orientation="vertical" className="h-4 bg-primary/30" />
											<TooltipProvider>
												<Tooltip delayDuration={150}>
													<TooltipTrigger asChild>
														<div className="flex gap-1.5 items-center cursor-default">
															<TagIcon className="h-4 w-4" />
															<span>{system.expand.tags.length}</span>
														</div>
													</TooltipTrigger>
													<TooltipContent>
														<div className="flex flex-wrap gap-1.5 max-w-64">
															{system.expand.tags.map((tag: TagRecord) => (
																<Badge
																	key={tag.id}
																	className={`text-xs pointer-events-none ${getTagColorClasses(tag.color)}`}
																>
																	{tag.name}
																</Badge>
															))}
														</div>
													</TooltipContent>
												</Tooltip>
											</TooltipProvider>
										</>
									)}
								</div>
							)
						})}
					</div>
				</div>
				<div className="xl:ms-auto flex items-center gap-2 max-sm:-mb-1">
					<ChartTimeSelect className="w-full xl:w-40" agentVersion={chartData.agentVersion} />
					<TooltipProvider delayDuration={100}>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									aria-label={t`Toggle grid`}
									variant="outline"
									size="icon"
									className="hidden xl:flex p-0 text-primary"
									onClick={() => setGrid(!grid)}
								>
									{grid ? (
										<LayoutGridIcon className="h-[1.2rem] w-[1.2rem] opacity-75" />
									) : (
										<Rows className="h-[1.3rem] w-[1.3rem] opacity-75" />
									)}
								</Button>
							</TooltipTrigger>
							<TooltipContent>{t`Toggle grid`}</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</div>
			</div>
		</Card>
	)
}
