import { memo, useEffect, useMemo, useState } from "react"
import { useLingui } from "@lingui/react/macro"
import { Trans } from "@lingui/react/macro"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { NetworkIcon, HistoryIcon, GlobeIcon, ServerIcon, GitCompareArrowsIcon } from "lucide-react"
import { useNetworkProbes, fetchTargetComparison, type ComparisonResult } from "@/lib/use-network-probes"
import { ResponseLossChart } from "@/components/routes/system/charts/probes-charts"
import { $direction, $systems } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { chartTimeData, cn, parseSemVer } from "@/lib/utils"
import type { ChartData, ChartTimes, NetworkProbeRecord } from "@/types"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { Card } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

const CHART_TIMES: ChartTimes[] = ["1h", "12h", "24h", "1w", "30d"]
const ZERO_VERSION = parseSemVer("0.0.0")

interface CompareEntry {
	key: string
	target: string
	protocol: string
	name: string
}

export default memo(function ProbesCompare({ protocol }: { protocol?: string }) {
	const { t } = useLingui()
	const probes = useNetworkProbes({})
	const allSystems = useStore($systems)
	const direction = useStore($direction)
	const [chartTime, setChartTime] = useState<ChartTimes>("1h")
	const [selectedSystemIds, setSelectedSystemIds] = useState<Set<string>>(new Set())

	useEffect(() => {
		document.title = `${t`Compare`} / Beszel`
	}, [t])

	const toggleSelectedSystem = (sysId: string) => {
		setSelectedSystemIds((prev) => {
			const next = new Set(prev)
			if (next.has(sysId)) {
				next.delete(sysId)
			} else {
				next.add(sysId)
			}
			return next
		})
	}

	const selectedSystemIdsList = useMemo(
		() => (selectedSystemIds.size > 0 ? [...selectedSystemIds].sort() : undefined),
		[selectedSystemIds]
	)

	// with 2+ systems selected, restrict comparison strictly to those systems; with a single
	// system selected there's nothing to compare it against on its own, so use it as a pivot
	// and keep all probes in scope, then only keep targets that pivot system actually monitors
	const relevantProbes = useMemo(
		() => (selectedSystemIdsList && selectedSystemIdsList.length >= 2
			? probes.filter((p) => selectedSystemIdsList.includes(p.system))
			: probes),
		[probes, selectedSystemIdsList]
	)

	const pivotSystemId = selectedSystemIdsList?.length === 1 ? selectedSystemIdsList[0] : undefined

	const compareEntries = useMemo<CompareEntry[]>(() => {
		const map = new Map<string, { target: string; protocol: string; name: string; systems: Set<string> }>()
		for (const p of relevantProbes) {
			const key = `${p.target}\x00${p.protocol}`
			const existing = map.get(key)
			if (existing) {
				existing.systems.add(p.system)
				if (!existing.name && p.name) existing.name = p.name
			} else {
				map.set(key, { target: p.target, protocol: p.protocol, name: p.name || "", systems: new Set([p.system]) })
			}
		}
		return [...map.entries()]
			.filter(([, e]) => e.systems.size >= 2 && (!pivotSystemId || e.systems.has(pivotSystemId)))
			.map(([key, e]) => ({ key, target: e.target, protocol: e.protocol, name: e.name || e.target }))
	}, [relevantProbes, pivotSystemId])

	const compareProtocols = useMemo(() => {
		const set = new Set(compareEntries.map((e) => e.protocol))
		return (["icmp", "tcp", "http"] as const).filter((p) => set.has(p))
	}, [compareEntries])

	const [activeProtocol, setActiveProtocol] = useState(
		() => (protocol && compareProtocols.includes(protocol as never) ? protocol : compareProtocols[0]) ?? "icmp"
	)

	useEffect(() => {
		if (!compareProtocols.includes(activeProtocol as never) && compareProtocols.length > 0) {
			setActiveProtocol(protocol && compareProtocols.includes(protocol as never) ? protocol : compareProtocols[0])
		}
	}, [compareProtocols])

	const chartData = useMemo<ChartData>(
		() => ({
			agentVersion: ZERO_VERSION,
			chartTime,
			orientation: direction === "rtl" ? "right" : "left",
		}),
		[chartTime, direction]
	)

	const totalTargets = useMemo(() => new Set(compareEntries.map((e) => e.target)).size, [compareEntries])
	const totalSystems = useMemo(() => {
		const systems = new Set<string>()
		for (const p of probes) {
			systems.add(p.system)
		}
		return systems.size
	}, [probes])

	let systemSelectorLabel = t`All systems`
	if (selectedSystemIds.size === 1) {
		const selectedSystem = allSystems.find((s) => selectedSystemIds.has(s.id))
		systemSelectorLabel = selectedSystem?.name ?? t`1 system selected`
	} else if (selectedSystemIds.size > 1) {
		systemSelectorLabel = t`${selectedSystemIds.size} systems selected`
	}

	const systemSelector = (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" className="relative ps-9 justify-start font-normal w-full xl:w-48">
					<ServerIcon className="size-3.5 absolute start-3 top-1/2 -translate-y-1/2 opacity-85" />
					{systemSelectorLabel}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent className="w-56">
				{allSystems.map((sys) => (
					<DropdownMenuCheckboxItem
						key={sys.id}
						checked={selectedSystemIds.has(sys.id)}
						onCheckedChange={() => toggleSelectedSystem(sys.id)}
					>
						{sys.name}
					</DropdownMenuCheckboxItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	)

	if (compareEntries.length === 0 && probes.length > 0) {
		return (
			<div className="grid gap-4 py-4">
				<div className="max-w-48">{systemSelector}</div>
				<p className="text-sm text-muted-foreground text-center py-12">
					{selectedSystemIds.size > 0
						? t`No shared targets among the selected systems.`
						: t`No targets are monitored by more than one system.`}
				</p>
				<FooterRepoLink />
			</div>
		)
	}

	return (
		<>
			<div className="grid gap-4 py-4 pb-8">
				<Card>
					<div className="grid xl:flex xl:gap-4 px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
						<div className="min-w-0">
							<h1 className="text-2xl sm:text-[1.6rem] font-semibold mb-1.5 flex items-center gap-2.5">
								<GitCompareArrowsIcon className="size-6 opacity-80" />
								<Trans>Network Probes Compare</Trans>
							</h1>
							<div className="flex items-center py-4 xl:p-0 -mt-3 xl:mt-1 gap-3 text-sm text-nowrap opacity-90 overflow-x-auto scrollbar-hide -mx-4 px-4 xl:mx-0">
								<div className="flex gap-1.5 items-center">
									<GlobeIcon className="h-4 w-4" />
									{t`${totalTargets} targets`}
								</div>
								<Separator orientation="vertical" className="h-4 bg-primary/30" />
								<div className="flex gap-1.5 items-center">
									<ServerIcon className="h-4 w-4" />
									{t`${totalSystems} systems`}
								</div>
								<Separator orientation="vertical" className="h-4 bg-primary/30" />
								<div className="flex gap-1.5 items-center">
									<NetworkIcon className="h-4 w-4" />
									{compareProtocols.map((p) => p.toUpperCase()).join(" · ")}
								</div>
							</div>
						</div>
						<div className="xl:ms-auto flex items-center gap-2 max-sm:-mb-1">
							{systemSelector}
							<Select value={chartTime} onValueChange={(v) => setChartTime(v as ChartTimes)}>
								<SelectTrigger className="w-full xl:w-40 ps-9 relative">
									<HistoryIcon className="h-4 w-4 absolute start-3 top-1/2 -translate-y-1/2 opacity-70" />
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{CHART_TIMES.map((ct) => (
										<SelectItem key={ct} value={ct}>
											{chartTimeData[ct].label()}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					</div>
				</Card>

				<Tabs value={activeProtocol} onValueChange={setActiveProtocol} className="contents">
					<TabsList className="h-11 p-1.5 w-full shadow-xs overflow-auto justify-start">
						{compareProtocols.map((p) => (
							<TabsTrigger key={p} value={p} className="w-full flex items-center gap-1.5 uppercase">
								<NetworkIcon className="size-3.5" />
								{p}
							</TabsTrigger>
						))}
					</TabsList>

					{compareProtocols.map((p) => (
						<TabsContent key={p} value={p} forceMount className={activeProtocol === p ? "contents" : "hidden"}>
							{activeProtocol === p && (
								<ProtocolCompareView
									entries={compareEntries.filter((e) => e.protocol === p)}
									chartData={chartData}
									systemIds={selectedSystemIdsList && selectedSystemIdsList.length >= 2 ? selectedSystemIdsList : undefined}
								/>
							)}
						</TabsContent>
					))}
				</Tabs>
			</div>
			<FooterRepoLink />
		</>
	)
})

function ProtocolCompareView({
	entries,
	chartData,
	systemIds,
}: {
	entries: CompareEntry[]
	chartData: ChartData
	systemIds?: string[]
}) {
	return (
		<div className={cn("grid gap-4 items-start", entries.length > 1 && "xl:grid-cols-2")}>
			{entries.map((entry) => (
				<TargetCompareCharts
					key={entry.key}
					name={entry.name}
					target={entry.target}
					protocol={entry.protocol}
					chartData={chartData}
					systemIds={systemIds}
				/>
			))}
		</div>
	)
}

function TargetCompareCharts({
	name,
	target,
	protocol,
	chartData,
	systemIds,
}: {
	name: string
	target: string
	protocol: string
	chartData: ChartData
	systemIds?: string[]
}) {
	const { t } = useLingui()
	const [result, setResult] = useState<ComparisonResult>({ syntheticProbes: [], stats: [], interval: 0, port: 0 })

	useEffect(() => {
		setResult({ syntheticProbes: [], stats: [], interval: 0, port: 0 })
		fetchTargetComparison(target, protocol, chartData.chartTime, systemIds)
			.then(setResult)
			.catch((e) => console.error("fetchTargetComparison failed", target, protocol, e))
	}, [target, protocol, chartData.chartTime, systemIds])

	const syntheticProbes = useMemo(
		() =>
			result.syntheticProbes.map(
				(p) =>
					({
						id: p.id,
						name: p.name,
						target: p.name,
						resAvg1h: 0,
					}) as unknown as NetworkProbeRecord
			),
		[result.syntheticProbes]
	)

	const empty = result.stats.length < 2

	return (
		<div className="grid gap-4">
			{result.syntheticProbes.length === 0 && (
				<p className="text-sm text-muted-foreground py-4 text-center">{t`No data found for this target.`}</p>
			)}
			{syntheticProbes.length > 0 && (
				<ResponseLossChart
					probeStats={result.stats}
					probes={syntheticProbes}
					chartData={chartData}
					empty={empty}
					grid={false}
					titlePrefix={name}
				/>
			)}
		</div>
	)
}
