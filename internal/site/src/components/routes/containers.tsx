import { useLingui } from "@lingui/react/macro"
import { memo, useEffect, useMemo } from "react"
import ContainersTable from "@/components/containers-table/containers-table"
import PodsTable from "@/components/pods-table/pods-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`Containers & Pods`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
				<div className="grid gap-4">
					<ActiveAlerts />
					<ContainersTable />
					<PodsTable />
				</div>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
