import { lazy } from "react"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn } from "@/lib/utils"
import { useNetworkProbes } from "@/lib/use-network-probes"

const ContainersTable = lazy(() => import("../../containers-table/containers-table"))

export function LazyContainersTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <ContainersTable systemId={systemId} />}
		</div>
	)
}

const SmartTable = lazy(() => import("./smart-table"))

export function LazySmartTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SmartTable systemId={systemId} />}
		</div>
	)
}

const ZfsTable = lazy(() => import("./storage-pools-table"))

export function LazyZfsTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <ZfsTable systemId={systemId} />}
		</div>
	)
}

const SystemdTable = lazy(() => import("../../systemd-table/systemd-table"))

export function LazySystemdTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver()
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SystemdTable systemId={systemId} />}
		</div>
	)
}

const NetworkProbesTable = lazy(() => import("../../network-probes-table/network-probes-table"))

export function LazyNetworkProbesTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SystemNetworkProbesTable systemId={systemId} />}
		</div>
	)
}

function SystemNetworkProbesTable({ systemId }: { systemId: string }) {
	const probes = useNetworkProbes({ systemId })
	return <NetworkProbesTable systemId={systemId} probes={probes} />
}
