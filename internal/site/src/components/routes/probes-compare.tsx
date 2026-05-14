import { memo, useEffect, useMemo, useState } from "react"
import { useLingui } from "@lingui/react/macro"
import { Trans } from "@lingui/react/macro"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { NetworkIcon, HistoryIcon, GlobeIcon, ServerIcon, GitCompareArrowsIcon, RefreshCwIcon, EthernetPortIcon } from "lucide-react"
import { useNetworkProbes, fetchTargetComparison, type ComparisonResult } from "@/lib/use-network-probes"
import { ResponseChart, LossChart } from "@/components/routes/system/charts/probes-charts"
import { $direction } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { chartTimeData, parseSemVer } from "@/lib/utils"
import type { ChartData, ChartTimes, NetworkProbeRecord } from "@/types"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { Card } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"

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
	const direction = useStore($direction)
	const [chartTime, setChartTime] = useState<ChartTimes>("1h")

	useEffect(() => {
		document.title = `${t`Compare`} / Beszel`
	}, [t])

	const compareEntries = useMemo<CompareEntry[]>(() => {
		const map = new Map<string, { target: string; protocol: string; name: string; systems: Set<string> }>()
		for (const p of probes) {
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
			.filter(([, e]) => e.systems.size >= 2)
			.map(([key, e]) => ({ key, target: e.target, protocol: e.protocol, name: e.name || e.target }))
	}, [probes])

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

	if (compareEntries.length === 0 && probes.length > 0) {
		return (
			<div className="grid gap-4 py-4">
				<p className="text-sm text-muted-foreground text-center py-12">{t`No targets are monitored by more than one system.`}</p>
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

function ProtocolCompareView({ entries, chartData }: { entries: CompareEntry[]; chartData: ChartData }) {
	return (
		<>
			{entries.map((entry) => (
				<TargetCompareCharts key={entry.key} name={entry.name} target={entry.target} protocol={entry.protocol} chartData={chartData} />
			))}
		</>
	)
}

function TargetCompareCharts({
	name,
	target,
	protocol,
	chartData,
}: {
	name: string
	target: string
	protocol: string
	chartData: ChartData
}) {
	const { t } = useLingui()
	const [result, setResult] = useState<ComparisonResult>({ syntheticProbes: [], stats: [] })

	useEffect(() => {
		setResult({ syntheticProbes: [], stats: [] })
		fetchTargetComparison(target, protocol, chartData.chartTime)
			.then(setResult)
			.catch((e) => console.error("fetchTargetComparison failed", target, protocol, e))
	}, [target, protocol, chartData.chartTime])

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
		<>
			<Card>
				<div className="px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
					<h2 className="text-lg font-semibold mb-1.5">{name}</h2>
					<div className="flex items-center py-4 xl:p-0 -mt-3 xl:mt-1 gap-3 text-sm text-nowrap opacity-90 overflow-x-auto scrollbar-hide -mx-4 px-4 xl:mx-0">
						<div className="flex gap-1.5 items-center">
							<GlobeIcon className="h-4 w-4" />
							{target}
						</div>
						{result.interval > 0 && (
							<>
								<Separator orientation="vertical" className="h-4 bg-primary/30" />
								<div className="flex gap-1.5 items-center">
									<RefreshCwIcon className="h-4 w-4" />
									{result.interval}s
								</div>
							</>
						)}
						{protocol === "tcp" && result.port > 0 && (
							<>
								<Separator orientation="vertical" className="h-4 bg-primary/30" />
								<div className="flex gap-1.5 items-center">
									<EthernetPortIcon className="h-4 w-4" />
									{result.port}
								</div>
							</>
						)}
						<Separator orientation="vertical" className="h-4 bg-primary/30" />
						<div className="flex gap-1.5 items-center">
							<ServerIcon className="h-4 w-4" />
							{result.syntheticProbes.length}
						</div>
					</div>
				</div>
			</Card>
			{result.syntheticProbes.length === 0 && (
				<p className="text-sm text-muted-foreground py-4 text-center">{t`No data found for this target.`}</p>
			)}
			{syntheticProbes.length > 0 && (
				<div className="grid xl:grid-cols-2 gap-4">
					<ResponseChart
						probeStats={result.stats}
						probes={syntheticProbes}
						chartData={chartData}
						empty={empty}
						grid={true}
					/>
					<LossChart
						probeStats={result.stats}
						probes={syntheticProbes}
						chartData={chartData}
						empty={empty}
						grid={true}
					/>
				</div>
			)}
		</>
	)
}
