import { useLingui } from "@lingui/react/macro"
import { memo, useEffect, useState } from "react"
import NetworkProbesTableNew from "@/components/network-probes-table/network-probes-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import type { NetworkProbeRecord } from "@/types"

export default memo(() => {
	const { t } = useLingui()
	const [probes, setProbes] = useState<NetworkProbeRecord[]>([])

	useEffect(() => {
		document.title = `${t`Network Probes`} / Beszel`
	}, [t])

	return (
		<>
			<div className="grid gap-4">
				<ActiveAlerts />
				<NetworkProbesTableNew probes={probes} setProbes={setProbes} />
			</div>
			<FooterRepoLink />
		</>
	)
})
