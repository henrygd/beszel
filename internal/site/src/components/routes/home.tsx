import { useLingui } from "@lingui/react/macro"
import { memo, Suspense, useEffect } from "react"
import SystemsTable from "@/components/systems-table/systems-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { FleetOverview } from "@/components/fleet-overview"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`All Systems`} / Beszel`
	}, [t])

	return (
		<>
			<div className="flex flex-col gap-4">
				<FleetOverview />
				<ActiveAlerts />
				<Suspense>
					<SystemsTable />
				</Suspense>
			</div>
			<FooterRepoLink />
		</>
	)
})
