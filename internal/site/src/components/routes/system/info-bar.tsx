import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { FreeBsdIcon, TuxIcon, WebSocketIcon, WindowsIcon } from "@/components/ui/icons"
import { SystemStatus, ConnectionType, connectionTypeLabels, Os } from "@/lib/enums"
import { cn, formatBytes, getHostDisplayValue, secondsToString, toFixedFloat } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import {
	AppleIcon,
	ChevronRightSquareIcon,
	ClockArrowUp,
	CpuIcon,
	GlobeIcon,
	LayoutGridIcon,
	MonitorIcon,
	Rows,
	MemoryStickIcon,
} from "lucide-react"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import type { ChartData, SystemDetailsRecord, SystemRecord } from "@/types"
import { useEffect, useMemo, useState } from "react"
import { useLingui } from "@lingui/react/macro"
import { pb } from "@/lib/api"

export default function InfoBar({
	system,
	chartData,
	grid,
	setGrid,
	setIsPodman,
}: {
	system: SystemRecord
	chartData: ChartData
	grid: boolean
	setGrid: (grid: boolean) => void
	setIsPodman: (isPodman: boolean) => void
}) {
	const { t } = useLingui()
	const [details, setDetails] = useState<SystemDetailsRecord | null>(null)

	// Fetch system_details on mount / when system changes
	useEffect(() => {
		// skip fetching system details if agent is older version which includes details in Info struct
		if (!system.id || system.info?.m) {
			return setDetails(null)
		}
		pb.collection<SystemDetailsRecord>("system_details")
			.getOne(system.id, {
				fields: "hostname,kernel,cores,threads,cpu,os,os_name,memory,podman",
				headers: {
					"Cache-Control": "public, max-age=60",
				},
			})
			.then((details) => {
				setDetails(details)
				setIsPodman(details.podman)
			})
			.catch(() => setDetails(null))
	}, [system.id])

	// values for system info bar - use details with fallback to system.info
	const systemInfo = useMemo(() => {
		if (!system.info) {
			return []
		}

		// Use details if available, otherwise fall back to system.info
		const hostname = details?.hostname ?? system.info.h
		const kernel = details?.kernel ?? system.info.k
		const cores = details?.cores ?? system.info.c
		const threads = details?.threads ?? system.info.t
		const cpuModel = details?.cpu ?? system.info.m
		const os = details?.os ?? system.info.os ?? Os.Linux
		const osName = details?.os_name
		const memory = details?.memory

		const osInfo = {
			[Os.Linux]: {
				Icon: TuxIcon,
				// show kernel in tooltip if os name is available, otherwise show the kernel
				value: osName || kernel,
				label: osName ? kernel : undefined,
				// label: t({ comment: "Linux kernel", message: "Kernel" }),
			},
			[Os.Darwin]: {
				Icon: AppleIcon,
				value: osName || `macOS ${kernel}`,
			},
			[Os.Windows]: {
				Icon: WindowsIcon,
				value: osName || kernel,
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

		info.push({
			value: `${cpuModel} (${cores}c${threads ? `/${threads}t` : ""})`,
			Icon: CpuIcon,
			hide: !cpuModel,
		})

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

						{systemInfo.map(({ value, label, Icon, hide }) => {
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
