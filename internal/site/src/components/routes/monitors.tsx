import { useLingui } from "@lingui/react/macro"
import { memo, useEffect } from "react"
import NetworkMonitorsTableNew from "@/components/network-monitors-table/network-monitors-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { useNetworkMonitors } from "@/lib/use-network-monitors"
import { $allSystemsById } from "@/lib/stores"
import { supportsNetworkMonitors } from "@/lib/utils"
import { useStore } from "@nanostores/react"

export default memo(() => {
	const { t } = useLingui()
	const { monitors, isLoading } = useNetworkMonitors({})
	const systems = useStore($allSystemsById)
	const visibleMonitors = monitors.filter((monitor) => {
		const system = systems[monitor.system]
		return !system || supportsNetworkMonitors(system)
	})

	useEffect(() => {
		document.title = `${t`Network Monitors`} / Beszel`
	}, [t])

	return (
		<>
			<div className="grid gap-4">
				<ActiveAlerts />
				<NetworkMonitorsTableNew monitors={visibleMonitors} isLoading={isLoading} />
			</div>
			<FooterRepoLink />
		</>
	)
})
