import { useLingui } from "@lingui/react/macro"
import { memo, useEffect } from "react"
import NetworkMonitorsTableNew from "@/components/network-monitors-table/network-monitors-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { useNetworkMonitors } from "@/lib/use-network-monitors"

export default memo(() => {
	const { t } = useLingui()
	const monitors = useNetworkMonitors({})

	useEffect(() => {
		document.title = `${t`Network Monitors`} / Beszel`
	}, [t])

	return (
		<>
			<div className="grid gap-4">
				<ActiveAlerts />
				<NetworkMonitorsTableNew monitors={monitors} />
			</div>
			<FooterRepoLink />
		</>
	)
})
