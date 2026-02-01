import { lazy } from "react"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn } from "@/lib/utils"
import type { ServicesTabProps } from "./types"

const SystemdTable = lazy(() => import("../../../systemd-table/systemd-table"))

function LazySystemdTable({ systemId }: { systemId: string }) {
	const { isIntersecting, ref } = useIntersectionObserver()
	return (
		<div ref={ref} className={cn(isIntersecting && "contents")}>
			{isIntersecting && <SystemdTable systemId={systemId} />}
		</div>
	)
}

export function ServicesTab({ systemId }: ServicesTabProps) {
	return <LazySystemdTable systemId={systemId} />
}
