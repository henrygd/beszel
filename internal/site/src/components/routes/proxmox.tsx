import { useLingui } from "@lingui/react/macro"
import { memo, useEffect, useMemo } from "react"
import PveTable from "@/components/pve-table/pve-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`All Proxmox VMs`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
				<div className="grid gap-4">
					<ActiveAlerts />
					<PveTable />
				</div>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
