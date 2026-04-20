import { useLingui } from "@lingui/react/macro"
import { memo, useEffect, useMemo } from "react"
import NetworkProbesTableNew from "@/components/network-probes-table/network-probes-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`Network Probes`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
				<div className="grid gap-4">
					<ActiveAlerts />
					<NetworkProbesTableNew />
				</div>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
