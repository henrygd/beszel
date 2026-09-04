import { useLingui } from "@lingui/react/macro"
import { memo, Suspense, useEffect, useMemo } from "react"
import SystemsTable from "@/components/systems-table/systems-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { MonitorsSummary } from "@/components/monitors-summary"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`All Systems`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
			<div className="flex flex-col gap-4">
				<ActiveAlerts />
				<MonitorsSummary />
				<Suspense>
					<SystemsTable />
				</Suspense>
			</div>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
