import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { atom } from "nanostores"
import { memo, useCallback, useMemo, useState } from "react"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { ResponseLossChart } from "@/components/routes/system/charts/monitors-charts"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { getMonitorIdentityKey } from "@/lib/monitor-compare"
import {
	getMonitorLabels,
	type MonitorProtocol as Protocol,
	PROTOCOL_LABELS,
	PROTOCOLS,
	withMonitorsGaps,
} from "@/lib/network-monitor-utils"
import { $allSystemsById, $direction, getUserChartTime } from "@/lib/stores"
import { useProtocolMonitorStats } from "@/lib/use-network-monitors"
import { chartTimeData, parseSemVer } from "@/lib/utils"
import type { ChartOptions, ChartTimes, NetworkMonitorRecord, NetworkMonitorStatsRecord } from "@/types"
import { SystemMultiSelect } from "./monitor-dialog"

const TAB_STORAGE_KEY = "besz-monitor-charts-tab"

function loadTab(): Protocol | null {
	try {
		return localStorage.getItem(TAB_STORAGE_KEY) as Protocol | null
	} catch {
		return null
	}
}

/** Groups monitors by what they probe, so each target gets a chart with a line per system. */
function groupTargets(monitors: NetworkMonitorRecord[]) {
	const byTarget = new Map<string, NetworkMonitorRecord[]>()
	for (const monitor of monitors) {
		const key = getMonitorIdentityKey(monitor)
		const group = byTarget.get(key) ?? []
		group.push(monitor)
		byTarget.set(key, group)
	}
	const groups = [...byTarget.entries()].map(([key, group]) => ({ key, monitors: group }))
	// Titles tell apart targets that differ only by DNS server or URL scheme.
	const titles = getMonitorLabels(
		groups.map(({ monitors }) => monitors[0]),
		(_, target) => target
	)
	return groups
		.map((group) => ({ ...group, title: titles.get(group.monitors[0].id) ?? group.monitors[0].target }))
		.sort((a, b) => a.title.localeCompare(b.title, undefined, { numeric: true }))
}

/**
 * Dashboard of response and loss charts for every target, with a tab per protocol like the system page.
 * `monitors` arrive already filtered by the table's filters.
 */
export default function NetworkMonitorsCharts({ monitors }: { monitors: NetworkMonitorRecord[] }) {
	// Realtime isn't offered (see useProtocolMonitorStats), so fall back to 1h.
	const [chartTimeStore] = useState(() => {
		const defaultTime = getUserChartTime()
		return atom<ChartTimes>(defaultTime === "1m" ? "1h" : defaultTime)
	})
	const chartTime = useStore(chartTimeStore)
	const direction = useStore($direction)
	const [selectedSystemIds, setSelectedSystemIds] = useState<Set<string>>(() => new Set())
	const [storedTab, setStoredTab] = useState(loadTab)

	const chartData = useMemo<ChartOptions>(
		() => ({ agentVersion: parseSemVer(), orientation: direction === "rtl" ? "right" : "left", chartTime }),
		[direction, chartTime]
	)

	const systemIdsWithMonitors = useMemo(() => [...new Set(monitors.map((m) => m.system))], [monitors])

	const targetsByProtocol = useMemo(() => {
		const byProtocol = new Map<Protocol, NetworkMonitorRecord[]>()
		for (const monitor of monitors) {
			if (selectedSystemIds.size && !selectedSystemIds.has(monitor.system)) continue
			const group = byProtocol.get(monitor.protocol) ?? []
			group.push(monitor)
			byProtocol.set(monitor.protocol, group)
		}
		return new Map([...byProtocol].map(([protocol, group]) => [protocol, groupTargets(group)]))
	}, [monitors, selectedSystemIds])

	const protocols = PROTOCOLS.filter((protocol) => targetsByProtocol.has(protocol))
	// Fall back to the first tab when the remembered protocol has no monitors (e.g. filtered out).
	const activeTab = storedTab && protocols.includes(storedTab) ? storedTab : protocols[0]

	const handleTabChange = useCallback((value: string) => {
		setStoredTab(value as Protocol)
		try {
			localStorage.setItem(TAB_STORAGE_KEY, value)
		} catch {
			// the tab just isn't remembered
		}
	}, [])

	return (
		<Tabs value={activeTab} onValueChange={handleTabChange} className="contents">
			<div className="flex flex-wrap items-center gap-2">
				<TabsList className="h-11 p-1.5 shadow-xs overflow-auto justify-start flex-1 basis-full md:basis-0 min-w-0">
					{protocols.map((protocol) => (
						<TabsTrigger key={protocol} value={protocol} className="w-full flex items-center gap-1.5">
							{PROTOCOL_LABELS[protocol]}
						</TabsTrigger>
					))}
				</TabsList>
				<SystemMultiSelect
					id="monitor-charts-systems"
					className="h-11 flex-1 min-w-0 basis-full sm:basis-0 md:max-w-64 bg-card"
					systemIds={systemIdsWithMonitors}
					selectedSystemIds={selectedSystemIds}
					onChange={setSelectedSystemIds}
					disabled={systemIdsWithMonitors.length < 2}
				/>
				<ChartTimeSelect
					className="h-11 bg-card flex-1 min-w-0 basis-full sm:basis-0 md:max-w-48"
					agentVersion={chartData.agentVersion}
					chartTimeStore={chartTimeStore}
					allowRealtime={false}
				/>
			</div>
			{activeTab ? (
				// Only the active tab is mounted, so only its protocol fetches; switching back reuses the cache.
				<ProtocolCharts
					key={activeTab}
					protocol={activeTab}
					targets={targetsByProtocol.get(activeTab) ?? []}
					chartData={chartData}
				/>
			) : (
				<p className="py-12 text-center text-sm text-muted-foreground">
					<Trans>No results.</Trans>
				</p>
			)}
		</Tabs>
	)
}

const ProtocolCharts = memo(function ProtocolCharts({
	protocol,
	targets,
	chartData,
}: {
	protocol: Protocol
	targets: ReturnType<typeof groupTargets>
	chartData: ChartOptions
}) {
	// Stats cover the whole protocol, so filtering monitors doesn't refetch.
	const monitorStats = useProtocolMonitorStats(protocol, chartData.chartTime)

	return (
		<div className="grid xl:grid-cols-2 gap-4">
			{targets.map(({ key, title, monitors }) => (
				<TargetChart key={key} title={title} monitors={monitors} monitorStats={monitorStats} chartData={chartData} />
			))}
		</div>
	)
})

/** Response and loss for one target, with lines for each system that probes it. */
const TargetChart = memo(function TargetChart({
	title,
	monitors,
	monitorStats,
	chartData,
}: {
	title: string
	monitors: NetworkMonitorRecord[]
	monitorStats: NetworkMonitorStatsRecord[]
	chartData: ChartOptions
}) {
	const systems = useStore($allSystemsById)
	const getLabel = useCallback((m: NetworkMonitorRecord) => systems[m.system]?.name ?? m.system, [systems])

	// Only this target's records, with breaks at failed probes and missing data (see withMonitorsGaps).
	const stats = useMemo(
		() => withMonitorsGaps(monitorStats, monitors, chartTimeData[chartData.chartTime].expectedInterval),
		[monitorStats, monitors, chartData.chartTime]
	)
	return (
		<ResponseLossChart
			monitorStats={stats}
			monitors={monitors}
			chartData={chartData}
			empty={stats.length === 0}
			title={title}
			getLabel={getLabel}
		/>
	)
})
