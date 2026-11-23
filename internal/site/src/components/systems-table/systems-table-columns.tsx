/** biome-ignore-all lint/correctness/useHookAtTopLevel: Hooks live inside memoized column definitions */
import { plural, t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import type { CellContext, ColumnDef } from "@tanstack/react-table"
import type { ClassValue } from "clsx"
import {
	ChevronRightSquareIcon,
	ClockArrowUp,
	CpuIcon,
	HardDriveIcon,
	MemoryStickIcon,
	PackageIcon,
	ServerIcon,
	TerminalSquareIcon,
	WifiIcon,
} from "lucide-react"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"
import { BatteryState, ConnectionType, connectionTypeLabels, MeterState, SystemStatus } from "@/lib/enums"
import { $userSettings } from "@/lib/stores"
import { sortableHeader } from "@/components/ui/data-table-settings"
import {
	cn,
	decimalString,
	formatBytes,
	formatTemperature,
	matchesTableFilter,
	parseSemVer,
	secondsToUptimeString,
} from "@/lib/utils"
import { batteryStateTranslations } from "@/lib/i18n"
import { connectedWiFi, primaryWiFi, primaryWiFiSignal, wifiNetworkName, wifiSignalState } from "@/lib/wifi"
import type { SystemRecord, WiFi } from "@/types"
import AlertButton from "../alerts/alert-button"
import { $router, Link } from "../router"
import { ActionsButton } from "../system-actions-button"
import {
	BatteryMediumIcon,
	EthernetIcon,
	GpuIcon,
	HourglassIcon,
	ThermometerIcon,
	WebSocketIcon,
	BatteryHighIcon,
	BatteryLowIcon,
	PlugChargingIcon,
	BatteryFullIcon,
} from "../ui/icons"

const STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

/** Rank of the updates dot color for sorting: 2 security (red), 1 regular (yellow), 0 up to date (green), -1 no data */
function getUpdatesRank(pu: SystemRecord["info"]["pu"]): number {
	if (!pu) {
		return -1
	}
	const [total, security = 0] = pu
	return security > 0 ? 2 : total > 0 ? 1 : 0
}

function getMeterStateByThresholds(value: number, warn = 65, crit = 90): MeterState {
	return value >= crit ? MeterState.Crit : value >= warn ? MeterState.Warn : MeterState.Good
}

/**
 * @param viewMode - "table" or "grid"
 * @returns - Column definitions for the systems table
 */
export function SystemsTableColumns(viewMode: "table" | "grid"): ColumnDef<SystemRecord>[] {
	return [
		{
			// size: 200,
			size: 100,
			minSize: 0,
			accessorKey: "name",
			id: "system",
			meta: { label: () => t`System`, Icon: ServerIcon },
			sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
			filterFn: (() => {
				const statusTranslations = {
					[SystemStatus.Up]: t`Up`,
					[SystemStatus.Down]: t`Down`,
					[SystemStatus.Paused]: t`Paused`,
				} as const

				// match filter value against name, host, agent version, or translated status
				return (row, _, filterValue) => {
					const sys = row.original
					return matchesTableFilter(
						filterValue,
						sys.name,
						sys.host,
						sys.info.v,
						statusTranslations[sys.status as keyof typeof statusTranslations]
					)
				}
			})(),
			enableHiding: false,
			invertSorting: false,
			cell: (info) => {
				const { name, id } = info.row.original
				const linkUrl = getPagePath($router, "system", { id })

				return (
					<>
						<span className="flex gap-2 items-center font-medium text-sm text-nowrap md:ps-1">
							<IndicatorDot system={info.row.original} />
							<Link
								href={linkUrl}
								tabIndex={-1}
								className="relative block max-w-48 truncate z-10"
								onMouseEnter={(e) => {
									// set title on hover if text is truncated to show full name
									const a = e.currentTarget
									if (a.scrollWidth > a.clientWidth) {
										a.title = name
									} else {
										a.removeAttribute("title")
									}
								}}
							>
								{name}
							</Link>
						</span>
						<Link href={linkUrl} className="inset-0 absolute size-full" aria-label={name}></Link>
					</>
				)
			},
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.cpu || undefined,
			id: "cpu",
			meta: { label: () => t`CPU`, Icon: CpuIcon },
			cell: TableCellWithMeter,
			header: sortableHeader,
		},
		{
			// accessorKey: "info.mp",
			accessorFn: ({ info }) => info.mp || undefined,
			id: "memory",
			meta: { label: () => t`Memory`, Icon: MemoryStickIcon },
			cell: TableCellWithMeter,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.dp || undefined,
			id: "disk",
			meta: { label: () => t`Disk`, Icon: HardDriveIcon },
			cell: (info: CellContext<SystemRecord, unknown>) =>
				info.row.original.info.efs ? DiskCellWithMultiple(info) : TableCellWithMeter(info),
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.g,
			id: "gpu",
			meta: { label: () => "GPU", Icon: GpuIcon },
			cell: (info) => {
				const val = info.getValue() as number | undefined
				if (val === undefined) {
					return null
				}
				return TableCellWithMeter(info)
			},
			header: sortableHeader,
		},
		{
			id: "loadAverage",
			accessorFn: ({ info }) => info.la?.reduce((acc, curr) => acc + curr, 0),
			meta: { label: () => t({ message: "Load Avg", comment: "Short label for load average" }), Icon: HourglassIcon },
			size: 0,
			header: sortableHeader,
			cell(info: CellContext<SystemRecord, unknown>) {
				const { info: sysInfo, status } = info.row.original
				const { major, minor } = parseSemVer(sysInfo.v)
				const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
				const loadAverages = sysInfo.la || []

				const max = Math.max(...loadAverages)
				if (max === 0 && (status === SystemStatus.Paused || (major < 1 && minor < 13))) {
					return null
				}

				const normalizedLoad = max / (sysInfo.t ?? 1)
				const threshold = getMeterStateByThresholds(normalizedLoad * 100, colorWarn, colorCrit)

				return (
					<div className="flex items-center gap-[.35em] w-full tabular-nums tracking-tight">
						<span
							className={cn("inline-block size-2 rounded-full me-0.5", {
								[STATUS_COLORS[SystemStatus.Up]]: threshold === MeterState.Good,
								[STATUS_COLORS[SystemStatus.Pending]]: threshold === MeterState.Warn,
								[STATUS_COLORS[SystemStatus.Down]]: threshold === MeterState.Crit,
								[STATUS_COLORS[SystemStatus.Paused]]: status !== SystemStatus.Up,
							})}
						/>
						{loadAverages?.map((la, i) => (
							<span key={i}>{decimalString(la, la >= 10 ? 1 : 2)}</span>
						))}
					</div>
				)
			},
		},
		{
			accessorFn: ({ info, status }) => (status !== SystemStatus.Up ? undefined : info.bb),
			id: "net",
			meta: { label: () => t`Net`, Icon: EthernetIcon },
			size: 0,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const val = info.getValue() as number | undefined
				if (val === undefined) {
					return null
				}
				const userSettings = useStore($userSettings, { keys: ["unitNet"] })
				const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
				return (
					<span className="tabular-nums whitespace-nowrap">
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.dt,
			id: "temp",
			meta: {
				label: () => t({ message: "Temp", comment: "Temperature label in systems table" }),
				Icon: ThermometerIcon,
				hideSortIcon: true,
			},
			size: 50,
			header: sortableHeader,
			cell(info) {
				const val = info.getValue() as number
				const userSettings = useStore($userSettings, { keys: ["unitTemp"] })
				if (!val) {
					return null
				}
				const { value, unit } = formatTemperature(val, userSettings.unitTemp)
				return (
					<span className={cn("tabular-nums whitespace-nowrap", viewMode === "table" && "ps-0.5")}>
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.bat?.[0],
			id: "battery",
			meta: {
				label: () => t({ message: "Bat", comment: "Battery label in systems table header" }),
				Icon: BatteryMediumIcon,
				hideSortIcon: true,
			},
			size: 70,
			header: sortableHeader,
			cell(info) {
				const [pct, state] = info.row.original.info.bat ?? []
				if (pct === undefined) {
					return null
				}

				let Icon = PlugChargingIcon
				let iconColor = "text-muted-foreground"

				if (state !== BatteryState.Charging) {
					if (pct < 25) {
						iconColor = pct < 11 ? "text-red-500" : "text-yellow-500"
						Icon = BatteryLowIcon
					} else if (pct < 75) {
						Icon = BatteryMediumIcon
					} else if (pct < 95) {
						Icon = BatteryHighIcon
					} else {
						Icon = BatteryFullIcon
					}
				}

				const stateLabel =
					state !== undefined ? (batteryStateTranslations[state as BatteryState]?.() ?? undefined) : undefined

				return (
					<Link
						tabIndex={-1}
						href={getPagePath($router, "system", { id: info.row.original.id })}
						className="flex items-center gap-1 tabular-nums tracking-tight relative z-10"
						title={stateLabel}
					>
						<Icon className={cn("size-3.5", iconColor)} />
						<span className="min-w-10">{pct}%</span>
					</Link>
				)
			},
		},
		{
			accessorFn: primaryWiFiSignal,
			id: "wifi",
			meta: { label: () => "Wi-Fi", Icon: WifiIcon, hideSortIcon: true },
			size: 80,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const connections = connectedWiFi(info.row.original)
				const primary = primaryWiFi(connections)
				if (!primary) {
					return null
				}
				const displayedConnections = viewMode === "table" ? [primary] : connections
				return (
					<Tooltip>
						<TooltipTrigger asChild>
							<Link
								href={getPagePath($router, "system", { id: info.row.original.id })}
								tabIndex={-1}
								className="flex flex-col gap-0.5 min-w-0 py-1 relative z-10"
							>
								{displayedConnections.map(([id, wifi]) => (
									<WiFiSignal key={id} wifi={wifi} />
								))}
								{viewMode === "table" && connections.length > 1 && (
									<span className="text-xs text-muted-foreground">+{connections.length - 1}</span>
								)}
							</Link>
						</TooltipTrigger>
						<TooltipContent side="right" className="max-w-xs pb-2">
							<div className="grid gap-1">
								{connections.map(([id, wifi]) => (
									<div key={id} className="grid gap-0.5">
										<div className="text-[0.65rem] max-w-40 text-muted-foreground uppercase tracking-wide truncate">
											{id}
										</div>
										<div className="flex gap-2 items-center text-xs">
											<WiFiSignal wifi={wifi} className="shrink-0" />
											{(wifi.s || wifi.p) && <span className="truncate max-w-40">{wifiNetworkName(wifi)}</span>}
											{wifi.a && (
												<span className="text-muted-foreground whitespace-nowrap">
													<Trans>AP · client avg</Trans>
												</span>
											)}
										</div>
									</div>
								))}
							</div>
						</TooltipContent>
					</Tooltip>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.sv?.[0],
			id: "services",
			meta: { label: () => t`Services`, Icon: TerminalSquareIcon, hideSortIcon: true },
			size: 50,
			header: sortableHeader,
			sortingFn: (a, b) => {
				// sort priorities: 1) has failed services (dot color), 2) total services
				const [totalCountA, numFailedA] = a.original.info.sv ?? [0, 0]
				const [totalCountB, numFailedB] = b.original.info.sv ?? [0, 0]
				const hasFailedA = numFailedA > 0 ? 1 : 0
				const hasFailedB = numFailedB > 0 ? 1 : 0
				if (hasFailedA !== hasFailedB) {
					return hasFailedA - hasFailedB
				}
				return totalCountA - totalCountB
			},
			cell(info) {
				const sys = info.row.original
				const [totalCount, numFailed] = sys.info.sv ?? [0, 0]
				if (sys.status !== SystemStatus.Up || totalCount === 0) {
					return null
				}
				const content = (
					<span className="tabular-nums whitespace-nowrap flex gap-1.5 items-center">
						<span
							className={cn("block size-2 rounded-full", {
								[STATUS_COLORS.pending]: numFailed > 0,
								[STATUS_COLORS.up]: numFailed === 0,
							})}
						/>
						{plural(totalCount, { one: "# service", other: "# services" })}
					</span>
				)
				if (numFailed === 0) {
					return content
				}
				return (
					<Tooltip>
						<TooltipTrigger asChild>
							<Link
								href={getPagePath($router, "system", { id: sys.id })}
								tabIndex={-1}
								className="relative z-10 w-fit block"
							>
								{content}
							</Link>
						</TooltipTrigger>
						<TooltipContent>
							{plural(numFailed, { one: "# failed service", other: "# failed services" })}
						</TooltipContent>
					</Tooltip>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.pu?.[0],
			id: "updates",
			meta: { label: () => t`Updates`, Icon: PackageIcon, hideSortIcon: true },
			size: 50,
			header: sortableHeader,
			sortingFn: (a, b) => {
				// sort priorities: 1) dot color (security > regular > up to date), 2) total updates
				const puA = a.original.info.pu
				const puB = b.original.info.pu
				const rankA = getUpdatesRank(puA)
				const rankB = getUpdatesRank(puB)
				if (rankA !== rankB) {
					return rankA - rankB
				}
				return (puA?.[0] ?? 0) - (puB?.[0] ?? 0)
			},
			cell(info) {
				const sys = info.row.original
				if (!sys.info.pu) {
					return null
				}
				const [total, security = 0] = sys.info.pu
				return (
					<span className="tabular-nums whitespace-nowrap flex gap-1.5 items-center">
						<span
							className={cn("block size-2 rounded-full", {
								[STATUS_COLORS[SystemStatus.Down]]: security > 0,
								[STATUS_COLORS[SystemStatus.Pending]]: security === 0 && total > 0,
								[STATUS_COLORS[SystemStatus.Up]]: total === 0,
								[STATUS_COLORS[SystemStatus.Paused]]: sys.status !== SystemStatus.Up,
							})}
						/>
						{total === 0 ? t`Up to date` : plural(total, { one: "# update", other: "# updates" })}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.u || undefined,
			id: "uptime",
			meta: { label: () => t`Uptime`, Icon: ClockArrowUp, hideSortIcon: true },
			size: 50,
			header: sortableHeader,
			cell(info) {
				const uptime = info.getValue() as number
				if (!uptime) {
					return null
				}
				return <span className="tabular-nums whitespace-nowrap">{secondsToUptimeString(uptime)}</span>
			},
		},
		{
			accessorFn: ({ info }) => info.v,
			id: "agent",
			meta: { label: () => t`Agent`, Icon: WifiIcon, hideSortIcon: true },
			size: 50,
			header: sortableHeader,
			cell(info) {
				const version = info.getValue() as string
				if (!version) {
					return null
				}
				const system = info.row.original
				let color = "text-red-500"
				if (system.status === SystemStatus.Up) {
					color = version === globalThis.BESZEL.HUB_VERSION ? "text-green-500" : "text-yellow-500"
				} else if (system.status === SystemStatus.Paused) {
					color = "text-primary/40"
				}
				return (
					<Link
						href={getPagePath($router, "system", { id: system.id })}
						className={cn(
							"flex gap-1.5 items-center md:pe-5 tabular-nums relative z-10",
							viewMode === "table" && "ps-0.5"
						)}
						tabIndex={-1}
						title={connectionTypeLabels[system.info.ct as ConnectionType]}
						role="none"
					>
						{system.info.ct === ConnectionType.WebSocket && (
							<WebSocketIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{system.info.ct === ConnectionType.SSH && (
							<ChevronRightSquareIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{!system.info.ct && <IndicatorDot system={system} className={cn(color, "bg-current mx-0.5")} />}
						<span className="truncate max-w-14">{info.getValue() as string}</span>
					</Link>
				)
			},
		},
		{
			id: "actions",
			meta: { label: () => t({ message: "Actions", comment: "Table column" }) },
			size: 50,
			cell: ({ row }) => (
				<div className="relative z-10 flex justify-end items-center gap-1 -ms-3">
					<AlertButton system={row.original} />
					<ActionsButton system={row.original} />
				</div>
			),
		},
	] as ColumnDef<SystemRecord>[]
}


function TableCellWithMeter(info: CellContext<SystemRecord, unknown>) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const val = Number(info.getValue()) || 0
	const threshold = getMeterStateByThresholds(val, colorWarn, colorCrit)
	const meterClass = cn(
		"h-full",
		(info.row.original.status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
	)
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight w-full min-w-0">
			<span className="min-w-8 shrink-0">{decimalString(val, val >= 10 ? 1 : 2)}%</span>
			<span className="flex-1 min-w-8 grid bg-muted h-[1em] rounded-sm overflow-hidden">
				<span className={meterClass} style={{ width: `${val}%` }}></span>
			</span>
		</div>
	)
}

function DiskCellWithMultiple(info: CellContext<SystemRecord, unknown>) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const { info: sysInfo, status, id } = info.row.original
	const extraFs = Object.entries(sysInfo.efs ?? {})
	const rootDiskPct = sysInfo.dp
	const rootDiskName = sysInfo.rdn

	extraFs.sort((a, b) => a[0].localeCompare(b[0]))

	function getIndicatorColor(pct: number) {
		const threshold = getMeterStateByThresholds(pct, colorWarn, colorCrit)
		return (
			(status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
		)
	}

	function getMeterClass(pct: number) {
		return cn("h-full", getIndicatorColor(pct))
	}

	// Extra disk indicators (max 3 dots - one per state if any disk exists in range)
	const stateColors = [STATUS_COLORS.up, STATUS_COLORS.pending, STATUS_COLORS.down]
	const extraDiskIndicators =
		status !== SystemStatus.Up
			? []
			: [...new Set(extraFs.map(([, pct]) => getMeterStateByThresholds(pct, colorWarn, colorCrit)))]
					.sort()
					.map((state) => stateColors[state])

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Link
					href={getPagePath($router, "system", { id })}
					tabIndex={-1}
					className="flex flex-col gap-0.5 w-full relative z-10"
				>
					<div className="flex gap-2 items-center tabular-nums tracking-tight">
						<span className="min-w-8 shrink-0">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
						<span className="flex-1 min-w-8 flex items-center gap-0.5 px-1 justify-end bg-muted h-[1em] rounded-sm overflow-hidden relative">
							{/* Root disk */}
							<span
								className={cn("absolute inset-0", getMeterClass(rootDiskPct))}
								style={{ width: `${rootDiskPct}%` }}
							></span>
							{/* Extra disk indicators */}
							{extraDiskIndicators.map((color) => (
								<span
									key={color}
									className={cn("size-1.5 rounded-full shrink-0 outline-[0.5px] outline-muted", color)}
								/>
							))}
						</span>
					</div>
				</Link>
			</TooltipTrigger>
			<TooltipContent side="right" className="max-w-xs pb-2">
				<div className="grid gap-1">
					<div className="grid gap-0.5">
						<div className="text-[0.65rem] max-w-40 text-muted-foreground uppercase tracking-wide truncate tabular-nums">
							{rootDiskName ?? <Trans context="Root disk label">Root</Trans>}
						</div>
						<div className="flex gap-2 items-center tabular-nums text-xs">
							<span className="min-w-7">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
							<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
								<span className={getMeterClass(rootDiskPct)} style={{ width: `${rootDiskPct}%` }}></span>
							</span>
						</div>
					</div>
					{extraFs.map(([name, pct]) => {
						return (
							<div key={name} className="grid gap-0.5">
								<div className="text-[0.65rem] max-w-40 text-muted-foreground uppercase tracking-wide truncate">
									{name}
								</div>
								<div className="flex gap-2 items-center tabular-nums text-xs">
									<span className="min-w-7">{decimalString(pct, pct >= 10 ? 1 : 2)}%</span>
									<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
										<span className={getMeterClass(pct)} style={{ width: `${pct}%` }}></span>
									</span>
								</div>
							</div>
						)
					})}
				</div>
			</TooltipContent>
		</Tooltip>
	)
}

function WiFiSignal({ wifi, className }: { wifi: WiFi; className?: ClassValue }) {
	const state = wifi.r === undefined ? undefined : wifiSignalState(wifi.r)
	return (
		<span className={cn("flex items-center gap-1.5 tabular-nums whitespace-nowrap", className)}>
			<span
				className={cn("block size-2 rounded-full shrink-0", {
					[STATUS_COLORS[SystemStatus.Up]]: state === MeterState.Good,
					[STATUS_COLORS[SystemStatus.Pending]]: state === MeterState.Warn,
					[STATUS_COLORS[SystemStatus.Down]]: state === MeterState.Crit,
					[STATUS_COLORS[SystemStatus.Paused]]: state === undefined,
				})}
			/>
			{wifi.r === undefined ? t`Unknown` : `${wifi.r} dBm`}
		</span>
	)
}

export function IndicatorDot({ system, className }: { system: SystemRecord; className?: ClassValue }) {
	className ||= STATUS_COLORS[system.status as keyof typeof STATUS_COLORS] || ""
	return (
		<span
			className={cn("shrink-0 size-2 rounded-full", className)}
			// style={{ marginBottom: "-1px" }}
		/>
	)
}
