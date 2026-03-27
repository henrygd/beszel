import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import {
	AppleIcon,
	ChevronRightSquareIcon,
	ClockArrowUp,
	CpuIcon,
	GlobeIcon,
	MemoryStickIcon,
	MonitorIcon,
	Settings2Icon,
} from "lucide-react"
import { useMemo } from "react"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { FreeBsdIcon, TuxIcon, WebSocketIcon, WindowsIcon } from "@/components/ui/icons"
import { Separator } from "@/components/ui/separator"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { ConnectionType, connectionTypeLabels, Os, SystemStatus } from "@/lib/enums"
import { cn, formatBytes, getHostDisplayValue, secondsToUptimeString, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemDetailsRecord, SystemRecord } from "@/types"

export default function InfoBar({
	system,
	chartData,
	grid,
	setGrid,
	displayMode,
	setDisplayMode,
	details,
}: {
	system: SystemRecord
	chartData: ChartData
	grid: boolean
	setGrid: (grid: boolean) => void
	displayMode: "default" | "tabs"
	setDisplayMode: (mode: "default" | "tabs") => void
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

		const info = [
			{ value: getHostDisplayValue(system), Icon: GlobeIcon },
			{
				value: hostname,
				Icon: MonitorIcon,
				label: "Hostname",
				// hide if hostname is same as host or name
				hide: hostname === system.host || hostname === system.name,
			},
			{ value: secondsToUptimeString(system.info.u), Icon: ClockArrowUp, label: t`Uptime`, hide: !system.info.u },
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
			<div className="grid xl:flex xl:gap-4 px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
				<div className="min-w-0">
					<h1 className="text-2xl sm:text-[1.6rem] font-semibold mb-1.5">{system.name}</h1>
					<div className="flex xl:flex-wrap items-center py-4 xl:p-0 -mt-3 xl:mt-1 gap-3 text-sm text-nowrap opacity-90 overflow-x-auto scrollbar-hide -mx-4 px-4 xl:mx-0">
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
										<Tooltip delayDuration={100}>
											<TooltipTrigger asChild>{content}</TooltipTrigger>
											<TooltipContent>{label}</TooltipContent>
										</Tooltip>
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
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								aria-label={t`Settings`}
								variant="outline"
								size="icon"
								className="hidden xl:flex p-0 text-primary"
							>
								<Settings2Icon className="size-4 opacity-90" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" className="min-w-44">
							<DropdownMenuLabel className="px-3.5">
								<Trans context="Layout display options">Display</Trans>
							</DropdownMenuLabel>
							<DropdownMenuSeparator />
							<DropdownMenuRadioGroup
								className="px-1 pb-1"
								value={displayMode}
								onValueChange={(v) => setDisplayMode(v as "default" | "tabs")}
							>
								<DropdownMenuRadioItem value="default" onSelect={(e) => e.preventDefault()}>
									<Trans context="Default system layout option">Default</Trans>
								</DropdownMenuRadioItem>
								<DropdownMenuRadioItem value="tabs" onSelect={(e) => e.preventDefault()}>
									<Trans context="Tabs system layout option">Tabs</Trans>
								</DropdownMenuRadioItem>
							</DropdownMenuRadioGroup>
							<DropdownMenuSeparator />
							<DropdownMenuLabel className="px-3.5">
								<Trans>Chart width</Trans>
							</DropdownMenuLabel>
							<DropdownMenuSeparator />
							<DropdownMenuRadioGroup
								className="px-1 pb-1"
								value={grid ? "grid" : "full"}
								onValueChange={(v) => setGrid(v === "grid")}
							>
								<DropdownMenuRadioItem value="grid" onSelect={(e) => e.preventDefault()}>
									<Trans>Grid</Trans>
								</DropdownMenuRadioItem>
								<DropdownMenuRadioItem value="full" onSelect={(e) => e.preventDefault()}>
									<Trans>Full</Trans>
								</DropdownMenuRadioItem>
							</DropdownMenuRadioGroup>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			</div>
		</Card>
	)
}
