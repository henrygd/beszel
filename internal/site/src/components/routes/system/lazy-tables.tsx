import { lazy } from "react"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn } from "@/lib/utils"
import { useNetworkMonitors } from "@/lib/use-network-monitors"

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

const NutTable = lazy(() => import("./nut-table"))

export function LazyNutTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <NutTable systemId={systemId} />}
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

const NetworkMonitorsTable = lazy(() => import("../../network-monitors-table/network-monitors-table"))

export function LazyNetworkMonitorsTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver({ rootMargin: "90px" })
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SystemNetworkMonitorsTable systemId={systemId} />}
		</div>
	)
}

function SystemNetworkMonitorsTable({ systemId }: { systemId: string }) {
	const { monitors, isLoading } = useNetworkMonitors({ systemId })
	return <NetworkMonitorsTable systemId={systemId} monitors={monitors} isLoading={isLoading} />
}
